package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/floriansw/go-hll-rcon/rconv2"
	"github.com/floriansw/hll-geofences/data"
	"github.com/floriansw/hll-geofences/worker"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).Warn("load-env", "error", err)
	}

	// Set up logger
	level := slog.LevelInfo
	if _, ok := os.LookupEnv("DEBUG"); ok {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Load configuration
	configPath := "./config.yml"
	if path, ok := os.LookupEnv("CONFIG_PATH"); ok {
		configPath = path
	}
	c, err := data.NewConfig(configPath, logger)
	if err != nil {
		logger.Error("config", "error", err)
		return
	}

	// Save config on exit
	defer func() {
		if err := c.Save(); err != nil {
			logger.Error("save-config", "error", err)
		}
	}()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize workers
	workers := make([]*worker.Worker, 0, len(c.Servers))
	for _, server := range c.Servers {
		pool, err := rconv2.NewConnectionPool(rconv2.ConnectionPoolOptions{
			Logger:   logger,
			Hostname: server.Host,
			Port:     server.Port,
			Password: server.Password,
		})
		if err != nil {
			logger.Error("create-connection-pool", "server", server.Host, "error", err)
			continue
		}
		w := worker.NewWorker(logger, pool, server)
		workers = append(workers, w)
		go w.Run(ctx)
	}

	// Channel to aggregate restart signals from all workers
	restartCh := make(chan struct{}, len(workers))
	for _, w := range workers {
		go func(w *worker.Worker) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-w.RestartSignal():
					select {
					case restartCh <- struct{}{}:
						logger.Info("worker-requested-restart", "server", w.Host()) // Use w.Host()
					default:
						logger.Warn("restart-channel-full", "server", w.Host()) // Use w.Host()
					}
				}
			}
		}(w)
	}

	// Listen for OS signals or restart signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		logger.Info("received-shutdown-signal")
	case <-restartCh:
		logger.Info("received-restart-signal")
	}

	// Graceful shutdown
	logger.Info("initiating-graceful-shutdown")
	cancel()

	// Wait briefly for workers to clean up
	time.Sleep(500 * time.Millisecond)

	// Restart the application
	logger.Info("restarting-application")
	if err := restartApplication(); err != nil {
		logger.Error("restart-application", "error", err)
		os.Exit(1)
	}
}

// restartApplication restarts the current executable with the same arguments
func restartApplication() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	// Start the new process
	if err := cmd.Start(); err != nil {
		return err
	}

	// Exit the current process
	os.Exit(0)
	return nil
}
