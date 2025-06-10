package worker

import (
        "context"
        "log/slog"
        "slices"
        "time"

        "github.com/floriansw/go-hll-rcon/rconv2"
        "github.com/floriansw/go-hll-rcon/rconv2/api"
        "github.com/floriansw/hll-geofences/data"
        "github.com/floriansw/hll-geofences/sync"
)

type Worker struct {
        pool               *rconv2.ConnectionPool
        l                  *slog.Logger
        c                  data.Server
        axisFences         []data.Fence
        alliesFences       []data.Fence
        punishAfterSeconds time.Duration
        sessionTicker      *time.Ticker
        playerTicker       *time.Ticker
        punishTicker       *time.Ticker
        inactivityTicker   *time.Ticker
        lastMapChange      time.Time
        current            *api.GetSessionResponse
        outsidePlayers     sync.Map[string, outsidePlayer]
        trackedPlayers     sync.Map[string, struct{}] // Added: Track players who have entered an allowed fence
        restartCh          chan struct{}
}

type outsidePlayer struct {
        Name         string
        LastGrid     api.Grid
        FirstOutside time.Time
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

func (w *Worker) Host() string {
        return w.c.Host
}

func NewWorker(l *slog.Logger, pool *rconv2.ConnectionPool, c data.Server) *Worker {
        punishAfterSeconds := 10
        if c.PunishAfterSeconds != nil {
                punishAfterSeconds = *c.PunishAfterSeconds
        }
        return &Worker{
                l:                  l,
                pool:               pool,
                punishAfterSeconds: time.Duration(punishAfterSeconds) * time.Second,
                c:                  c,
                sessionTicker:      time.NewTicker(1 * time.Second),
                playerTicker:       time.NewTicker(500 * time.Millisecond),
                punishTicker:       time.NewTicker(time.Second),
                inactivityTicker:   time.NewTicker(2 * time.Hour),
                lastMapChange:      time.Now(),
                outsidePlayers:     sync.Map[string, outsidePlayer]{},
                trackedPlayers:     sync.Map[string, struct{}]{}, // Initialize tracked players map
                restartCh:          make(chan struct{}),
        }
}

func (w *Worker) RestartSignal() <-chan struct{} {
        return w.restartCh
}

func (w *Worker) Run(ctx context.Context) {
        if err := w.populateSession(ctx); err != nil {
                w.l.Error("fetch-session", "error", err)
                return
        }

        go w.pollSession(ctx)
        go w.pollPlayers(ctx)
        go w.punishPlayers(ctx)
        go w.checkInactivity(ctx)
}

func (w *Worker) populateSession(ctx context.Context) error {
        return w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
                si, err := c.SessionInfo(ctx)
                if err != nil {
                        return err
                }
                if w.current != nil && w.current.MapName != si.MapName {
                        w.l.Info("map-changed", "old_map", w.current.MapName, "new_map", si.MapName)
                        w.lastMapChange = time.Now()
                        select {
                        case w.restartCh <- struct{}{}:
                                w.l.Info("signaled-restart-on-map-change")
                        default:
                                w.l.Warn("restart-channel-full")
                        }
                }
                w.current = si
                w.axisFences = w.applicableFences(w.c.AxisFence)
                w.alliesFences = w.applicableFences(w.c.AlliesFence)
                return nil
        })
}

func (w *Worker) checkInactivity(ctx context.Context) {
        for {
                select {
                case <-ctx.Done():
                        w.inactivityTicker.Stop()
                        return
                case <-w.inactivityTicker.C:
                        if time.Since(w.lastMapChange) >= 2*time.Hour {
                                err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
                                        players, err := c.Players(ctx)
                                        if err != nil {
                                                return err
                                        }
                                        if len(players.Players) == 0 {
                                                w.l.Info("no-players-inactivity-restart")
                                                select {
                                                case w.restartCh <- struct{}{}:
                                                        w.l.Info("signaled-restart-on-inactivity")
                                                        w.lastMapChange = time.Now()
                                                default:
                                                        w.l.Warn("restart-channel-full")
                                                }
                                        }
                                        return nil
                                })
                                if err != nil {
                                        w.l.Error("check-inactivity", "error", err)
                                }
                        }
                }
        }
}

func (w *Worker) punishPlayers(ctx context.Context) {
        for {
                select {
                case <-ctx.Done():
                        w.punishTicker.Stop()
                        return
                case <-w.punishTicker.C:
                        w.outsidePlayers.Range(func(id string, o outsidePlayer) bool {
                                if time.Since(o.FirstOutside) > w.punishAfterSeconds && time.Since(o.FirstOutside) < w.punishAfterSeconds+5*time.Second {
                                        go w.punishPlayer(ctx, id, o)
                                }
                                return true
                        })
                }
        }
}

func (w *Worker) punishPlayer(ctx context.Context, id string, o outsidePlayer) {
        message := w.c.PunishMessage()
        w.l.Debug("punish-message-final", "message", message)

        err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
                return c.PunishPlayer(ctx, id, message)
        })
        if err != nil {
                w.l.Error("punish-player", "player_id", id, "error", err)
                return
        }
        w.l.Info("punish-player", "player", o.Name, "grid", o.LastGrid.String())

        time.Sleep(5 * time.Second)
        w.outsidePlayers.Delete(id)
}

func (w *Worker) pollSession(ctx context.Context) {
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

func (w *Worker) pollPlayers(ctx context.Context) {
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
                                w.trackedPlayers.Range(func(id string, _ struct{}) bool {
                                        found := false
                                        for _, player := range players.Players {
                                                if player.Id == id {
                                                        found = true
                                                        break
                                                }
                                        }
                                        if !found {
                                                w.trackedPlayers.Delete(id)
                                                w.outsidePlayers.Delete(id)
                                        }
                                        return true
                                })
                                return nil
                        })
                        if err != nil {
                                w.l.Error("poll-players", "error", err)
                        }
                }
        }
}

func (w *Worker) checkPlayer(ctx context.Context, p api.GetPlayerResponse) {
        // Skip whitelisted players
        if slices.Contains(w.c.GetWhitelist(), p.Id) {
                w.outsidePlayers.Delete(p.Id)
                w.trackedPlayers.Delete(p.Id)
                return
        }

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

        // Start tracking player only after they enter an allowed fence
        if insideFence {
                w.trackedPlayers.Store(p.Id, struct{}{})
                w.outsidePlayers.Delete(p.Id)
                return
        }

        // Only process players who are already tracked (i.e., have entered an allowed fence before)
        if _, ok := w.trackedPlayers.Load(p.Id); !ok {
                return
        }

        // Player is outside allowed fences and is tracked
        if o, ok := w.outsidePlayers.Load(p.Id); ok {
                o.LastGrid = g
                w.outsidePlayers.Store(p.Id, o)
                return
        }

        w.outsidePlayers.Store(p.Id, outsidePlayer{FirstOutside: time.Now(), Name: p.Name, LastGrid: g})
        w.l.Info("player-outside-fence", "player", p.Name, "grid", g)

        message := w.c.WarningMessage()
        w.l.Debug("warning-message-final", "message", message)

        err := w.pool.WithConnection(ctx, func(c *rconv2.Connection) error {
                return c.MessagePlayer(ctx, p.Name, message)
        })
        if err != nil {
                w.l.Error("message-player-outside-fence", "player", p.Name, "grid", g, "error", err)
        }
}

func (w *Worker) applicableFences(f []data.Fence) (v []data.Fence) {
        for _, fence := range f {
                if fence.Matches(w.current) {
                        v = append(v, fence)
                }
        }
        return
}
