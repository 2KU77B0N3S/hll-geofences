package worker

import (
	"context"
	"fmt"
	"github.com/floriansw/go-hll-rcon/rconv2"
	"github.com/floriansw/go-hll-rcon/rconv2/api"
	"github.com/floriansw/hll-geofences/data"
	"github.com/floriansw/hll-geofences/sync"
	"log/slog"
	"slices"
	"sync"
	"time"
)

type worker struct {
	pool			*rconv2.ConnectionPool
	l			*slog.Logger
	c			data.Server
	axisFences		[]data.Fence
	alliesFences		[]data.Fence
	punishAfterSeconds	time.Duration

	sessionTicker		*time.Ticker
	playerTicker		*time.Ticker
	punishTicker		*time.Ticker

	current			*api.GetSessionResponse
	outsidePlayers		[]outsidePlayer
	outsidePlayersMu	sync.RWMutex
	trackedPlayers		[]string
	trackedPlayersMu	sync.RWMutex
}

type outsidePlayer struct {
	Name		string
	LastGrid	api.Grid
	FirstOutside	time.Time
}

var alliedTeams = []api.PlayerTeam{
	api.PlayerTeamB8a,
	api.PlayerTeamDak,
	api.PlayerTeamGb,
	api.PlayerTeamRus,
	api.PlayerTeamUs,
}

var axisTeams = []api.PlayerTeam{
	api.PlayerTeamGer,
}

func NewWorker(l *slog.Logger, pool *rconv2.ConnectionPool, c data.Server) *worker {
	punishAfterSeconds := 10
	if c.PunishAfterSeconds != nil {
		punishAfterSeconds = *c.PunishAfterSeconds
	}
	return &worker{
		l:			l,
		pool:			pool,
		punishAfterSeconds:	time.Duration(punishAfterSeconds) * time.Second,
		c:			c,

		sessionTicker:		time.NewTicker(1 * time.Second),
		playerTicker:		time.NewTicker(500 * time.Millisecond),
		punishTicker:		time.NewTicker(time.Second),
	}
}

func (w *worker) Run(ctx context.Context) {
	if err := w.populateSession(ctx); err != nil {
		w.l.Error("fetch-session", "error", err)
		return
	}

	go w.pollSession(ctx)
	go w.pollPlayers(ctx)
	go w.punishPlayers(ctx)
}

func (w *worker) populateSession(ctx context.Context) error {
	return w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
		si, err := c.SessionInfo(ctx)
		if err != nil {
			return err
		}
		w.current = si
		w.axisFences = w.applicableFences(w.c.AxisFence)
		w.alliesFences = w.applicableFences(w.c.AlliesFence)
		return nil
	})
}

func (w *worker) punishPlayers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.punishTicker.Stop()
			return
		case <-w.punishTicker.C:
			w.outsidePlayersMu.RLock()
			for _, o := range w.outsidePlayers {
				if time.Since(o.FirstOutside) > w.punishAfterSeconds && time.Since(o.FirstOutside) < w.punishAfterSeconds+5*time.Second {
					go w.punishPlayer(ctx, o.Name, o)
				}
			}
			w.outsidePlayersMu.RUnlock()
		}
	}
}

func (w *worker) punishPlayer(ctx context.Context, id string, o outsidePlayer) {
	err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
		return c.PunishPlayer(ctx, id, fmt.Sprintf(w.c.PunishMessage(), w.punishAfterSeconds.String()))
	})
	if err != nil {
		w.l.Error("punish-player", "player_id", id, "error", err)
		return
	}
	w.l.Info("punish-player", "player", o.Name, "grid", o.LastGrid.String())

	time.Sleep(5 * time.Second)
	w.outsidePlayersMu.Lock()
	w.outsidePlayers = removePlayer(w.outsidePlayers, id)
	w.outsidePlayersMu.Unlock()
}

func (w *worker) pollSession(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.sessionTicker.Stop()
			return
		case <-w.sessionTicker.C:
			if err := w.populateSession(ctx); err != nil {
				w.l.Error("poll-session", "error", err)
			}
		}
	}
}

func (w *worker) pollPlayers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.playerTicker.Stop()
			return
		case <-w.playerTicker.C:
			if len(w.alliesFences) == 0 && len(w.axisFences) == 0 {
				continue
			}

			err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
				players, err := c.Players(ctx)
				if err != nil {
					return err
				}
				for _, player := range players.Players {
					go w.checkPlayer(ctx, player)
				}
				w.trackedPlayersMu.Lock()
				var newTracked []string
				for _, id := range w.trackedPlayers {
					for _, player := range players.Players {
						if player.Id == id {
							newTracked = append(newTracked, id)
							break
						}
					}
				}
				w.trackedPlayers = newTracked
				w.trackedPlayersMu.Unlock()

				w.outsidePlayersMu.Lock()
				var newOutside []outsidePlayer
				for _, o := range w.outsidePlayers {
					for _, player := range players.Players {
						if player.Id == o.Name {
							newOutside = append(newOutside, o)
							break
						}
					}
				}
				w.outsidePlayers = newOutside
				w.outsidePlayersMu.Unlock()

				return nil
			})
			if err != nil {
				w.l.Error("poll-players", "error", err)
			}
		}
	}
}

func (w *worker) checkPlayer(ctx context.Context, p api.GetPlayerResponse) {
	if !p.Position.IsSpawned() {
		return
	}

	var fences []data.Fence
	if slices.Contains(alliedTeams, p.Team) {
		fences = w.alliesFences
	} else if slices.Contains(axisTeams, p.Team) {
		fences = w.axisFences
	}
	if len(fences) == 0 {
		return
	}

	g := p.Position.Grid(w.current)
	insideFence := false
	for _, f := range fences {
		if f.Includes(g) {
			insideFence = true
			break
		}
	}

	if insideFence {
		w.trackedPlayersMu.Lock()
		if !containsPlayer(w.trackedPlayers, p.Id) {
			w.trackedPlayers = append(w.trackedPlayers, p.Id)
		}
		w.trackedPlayersMu.Unlock()
		w.outsidePlayersMu.Lock()
		w.outsidePlayers = removePlayer(w.outsidePlayers, p.Id)
		w.outsidePlayersMu.Unlock()
		return
	}

	w.trackedPlayersMu.RLock()
	hasPlayer := containsPlayer(w.trackedPlayers, p.Id)
	w.trackedPlayersMu.RUnlock()
	if !hasPlayer {
		return
	}

	w.outsidePlayersMu.RLock()
	o, found := findOutsidePlayer(w.outsidePlayers, p.Id)
	w.outsidePlayersMu.RUnlock()
	if found {
		o.LastGrid = g
		w.outsidePlayersMu.Lock()
		for i, player := range w.outsidePlayers {
			if player.Name == p.Id {
				w.outsidePlayers[i] = o
				break
			}
		}
		w.outsidePlayersMu.Unlock()
		return
	}

	newOutside := outsidePlayer{FirstOutside: time.Now(), Name: p.Name, LastGrid: g}
	w.outsidePlayersMu.Lock()
	w.outsidePlayers = append(w.outsidePlayers, newOutside)
	w.outsidePlayersMu.Unlock()
	w.l.Info("player-outside-fence", "player", p.Name, "grid", g)
	err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
		return c.MessagePlayer(ctx, p.Name, fmt.Sprintf(w.c.WarningMessage(), w.punishAfterSeconds.String()))
	})
	if err != nil {
		w.l.Error("message-player-outside-fence", "player", p.Name, "grid", g, "error", err)
	}
}

func (w *worker) applicableFences(f []data.Fence) (v []data.Fence) {
	for _, fence := range f {
		if fence.Matches(w.current) {
			v = append(v, fence)
		}
	}
	return
}

// Helper functions for slice operations
func containsPlayer(players []string, id string) bool {
	for _, p := range players {
		if p == id {
			return true
		}
	}
	return false
}

func removePlayer(players []outsidePlayer, id string) []outsidePlayer {
	var result []outsidePlayer
	for _, p := range players {
		if p.Name != id {
			result = append(result, p)
		}
	}
	return result
}

func findOutsidePlayer(players []outsidePlayer, id string) (outsidePlayer, bool) {
	for _, p := range players {
		if p.Name == id {
			return p, true
		}
	}
	return outsidePlayer{}, false
}
