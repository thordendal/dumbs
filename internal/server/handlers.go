package server

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/thor/dumbs/internal/logger"
)

// handleRoot is the trivial liveness-visible endpoint: GET / → 200 "foobar".
func handleRoot(c echo.Context) error {
	return c.String(http.StatusOK, "foobar\n")
}

// handleLive is the liveness probe. Always 200 while the process is running.
// (Liveness cannot fail by design — if the handler runs, the process is alive.)
func (s *Server) handleLive(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is the readiness probe. Returns 503 and logs when the database
// is unreachable so the failure shows up in the log stream, not just in the
// monitoring system.
func (s *Server) handleReady(c echo.Context) error {
	ok, err := s.h.IsReady()
	if ok {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
	if err != nil {
		logger.Get().Error().Err(err).Msg("readiness probe: database ping failed")
	} else {
		logger.Get().Error().Msg("readiness probe: database not configured")
	}
	return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "database unreachable"})
}

// handleConfigReload re-reads the config file and applies the new log settings.
func (s *Server) handleConfigReload(c echo.Context) error {
	if err := s.cfg.Reload(); err != nil {
		logger.Get().Error().Err(err).Msg("config reload failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	cfg := s.cfg.Get()
	if err := logger.Apply(cfg); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	s.m.ConfigReloads.Inc()
	logger.Get().Info().Msg("config reloaded via API")
	return c.JSON(http.StatusOK, map[string]string{"status": "reloaded"})
}

// handleChaosStart returns a handler that starts the named chaos worker.
func (s *Server) handleChaosStart(kind string) echo.HandlerFunc {
	return func(c echo.Context) error {
		switch kind {
		case "logs":
			if !s.cm.StartLogs() {
				return c.JSON(http.StatusConflict, alreadyRunning(kind))
			}
		case "datadir":
			if !s.cm.StartDatadir() {
				return c.JSON(http.StatusConflict, alreadyRunning(kind))
			}
		case "database":
			ok, err := s.cm.StartDatabase()
			if err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			}
			if !ok {
				return c.JSON(http.StatusConflict, alreadyRunning(kind))
			}
		case "memory":
			if !s.cm.StartMemory() {
				return c.JSON(http.StatusConflict, alreadyRunning(kind))
			}
		}
		logger.Get().Warn().Str("chaos", kind).Msg("chaos worker started via API")
		return c.JSON(http.StatusOK, map[string]string{"status": "started", "chaos": kind})
	}
}

// handleChaosStop returns a handler that stops the named chaos worker.
func (s *Server) handleChaosStop(kind string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var stopped bool
		switch kind {
		case "logs":
			stopped = s.cm.StopLogs()
		case "datadir":
			stopped = s.cm.StopDatadir()
		case "database":
			stopped = s.cm.StopDatabase()
		case "memory":
			stopped = s.cm.StopMemory()
		}
		if !stopped {
			return c.JSON(http.StatusConflict, map[string]string{"error": kind + " chaos was not running"})
		}
		logger.Get().Info().Str("chaos", kind).Msg("chaos worker stopped via API")
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "chaos": kind})
	}
}

// handleChaosStatus returns the running state of all chaos workers.
func (s *Server) handleChaosStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, s.cm.Status())
}

func alreadyRunning(kind string) map[string]string {
	return map[string]string{"error": kind + " chaos is already running"}
}

// handleDatabaseReset drops and recreates the events table.
// Useful for cleaning up after database chaos without restarting the app.
func (s *Server) handleDatabaseReset(c echo.Context) error {
	if s.db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database not configured"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()
	if err := s.db.ResetSchema(ctx); err != nil {
		logger.Get().Error().Err(err).Msg("database reset failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	logger.Get().Info().Msg("database schema reset via API")
	return c.JSON(http.StatusOK, map[string]string{"status": "reset"})
}
