package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/thordendal/dumbs/internal/chaos"
	"github.com/thordendal/dumbs/internal/config"
	"github.com/thordendal/dumbs/internal/database"
	"github.com/thordendal/dumbs/internal/health"
	"github.com/thordendal/dumbs/internal/logger"
	"github.com/thordendal/dumbs/internal/metrics"
	"github.com/thordendal/dumbs/internal/server"
)

func main() {
	cfgPath := flag.String("config", "", "path to config YAML file (required)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "dumbs: -config flag is required")
		os.Exit(1)
	}

	// --- Config ---
	loader, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dumbs: load config: %v\n", err)
		os.Exit(1)
	}

	// --- Logger ---
	if err := logger.Init(loader.Get()); err != nil {
		fmt.Fprintf(os.Stderr, "dumbs: init logger: %v\n", err)
		os.Exit(1)
	}
	log := logger.Get()
	log.Info().
		Str("config", *cfgPath).
		Int("pid", os.Getpid()).
		Str("go", runtime.Version()).
		Msg("dumbs starting")

	// --- Database ---
	cfg := loader.Get()
	var db *database.DB
	if cfg.App.Database.DSN != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		db, err = database.Connect(ctx, cfg.App.Database.DSN)
		cancel()
		if err != nil {
			log.Warn().Err(err).Msg("database unavailable at startup; readiness probe will report 503")
		} else {
			schCtx, schCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := db.InitSchema(schCtx); err != nil {
				log.Warn().Err(err).Msg("schema init failed; database chaos may not work")
			}
			schCancel()
			log.Info().Msg("database connected")
		}
	} else {
		log.Warn().Msg("no database DSN configured; readiness probe will report 503")
	}

	// --- Subsystems ---
	m := metrics.New()
	h := health.New(db)
	cm := chaos.NewManager(loader, db, m)
	srv := server.New(loader, db, h, m, cm)

	// --- Signal handling ---
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		for sig := range sigs {
			switch sig {
			case syscall.SIGHUP:
				if err := loader.Reload(); err != nil {
					logger.Get().Error().Err(err).Msg("SIGHUP: config reload failed")
					continue
				}
				if err := logger.Apply(loader.Get()); err != nil {
					logger.Get().Error().Err(err).Msg("SIGHUP: logger apply failed")
					continue
				}
				m.ConfigReloads.Inc()
				logger.Get().Info().Msg("SIGHUP: config reloaded")

			case syscall.SIGUSR1:
				logger.Get().Info().Msg("SIGUSR1: stopping all chaos workers")
				cm.StopAll()
				logger.Get().Info().Msg("SIGUSR1: all chaos workers stopped")

			case syscall.SIGQUIT:
				logger.Get().Info().Msg("SIGQUIT: graceful shutdown initiated")
				cm.StopAll()
				shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := srv.Shutdown(shutCtx); err != nil {
					logger.Get().Error().Err(err).Msg("SIGQUIT: server shutdown error")
				}
				shutCancel()
				db.Close()
				logger.Get().Info().Msg("SIGQUIT: shutdown complete")
				os.Exit(0)

			case syscall.SIGTERM, syscall.SIGINT:
				logger.Get().Error().
					Str("signal", sig.String()).
					Msg("hard shutdown: not cleaning up — DB connections left dangling")
				os.Exit(1)
			}
		}
	}()

	// --- Start HTTP server (blocks) ---
	if err := srv.Start(loader.Get().App.Listen); err != nil {
		// Echo returns http.ErrServerClosed after graceful shutdown — that is not an error.
		logger.Get().Info().Err(err).Msg("server stopped")
	}
}
