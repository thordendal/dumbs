package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/thor/dumbs/internal/chaos"
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

// handleChaosList returns all four chaos resource documents.
func (s *Server) handleChaosList(c echo.Context) error {
	return c.JSON(http.StatusOK, s.cm.GetAll())
}

// handleChaosDeleteAll stops all chaos workers and resets all configs to defaults.
func (s *Server) handleChaosDeleteAll(c echo.Context) error {
	s.cm.ResetAll()
	logger.Get().Info().Msg("all chaos workers stopped and reset via API")
	return c.JSON(http.StatusOK, s.cm.GetAll())
}

// handleChaosGet returns the resource document for a single chaos worker.
func (s *Server) handleChaosGet(c echo.Context) error {
	switch c.Param("kind") {
	case "logs":
		return c.JSON(http.StatusOK, s.cm.GetLogs())
	case "datadir":
		return c.JSON(http.StatusOK, s.cm.GetDatadir())
	case "database":
		return c.JSON(http.StatusOK, s.cm.GetDatabase())
	case "memory":
		return c.JSON(http.StatusOK, s.cm.GetMemory())
	default:
		return echo.ErrNotFound
	}
}

// handleChaosPut replaces the full resource document for a chaos worker.
// All fields must be provided. active=true starts the worker; active=false stops it.
func (s *Server) handleChaosPut(c echo.Context) error {
	switch c.Param("kind") {
	case "logs":
		var cfg chaos.LogsConfig
		if err := c.Bind(&cfg); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.ApplyLogs(cfg)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "logs").Bool("active", result.Active).Msg("chaos config replaced via PUT")
		return c.JSON(http.StatusOK, result)
	case "datadir":
		var cfg chaos.DatadirConfig
		if err := c.Bind(&cfg); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.ApplyDatadir(cfg)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "datadir").Bool("active", result.Active).Msg("chaos config replaced via PUT")
		return c.JSON(http.StatusOK, result)
	case "database":
		var cfg chaos.DatabaseChaosConfig
		if err := c.Bind(&cfg); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.ApplyDatabase(cfg)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "database").Bool("active", result.Active).Msg("chaos config replaced via PUT")
		return c.JSON(http.StatusOK, result)
	case "memory":
		var cfg chaos.MemoryConfig
		if err := c.Bind(&cfg); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.ApplyMemory(cfg)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "memory").Bool("active", result.Active).Msg("chaos config replaced via PUT")
		return c.JSON(http.StatusOK, result)
	default:
		return echo.ErrNotFound
	}
}

// handleChaosPatch applies a partial update to a chaos resource document.
// Omitted JSON fields are left unchanged.
func (s *Server) handleChaosPatch(c echo.Context) error {
	switch c.Param("kind") {
	case "logs":
		var patch chaos.LogsConfigPatch
		if err := c.Bind(&patch); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.PatchLogs(patch)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "logs").Bool("active", result.Active).Msg("chaos config patched")
		return c.JSON(http.StatusOK, result)
	case "datadir":
		var patch chaos.DatadirConfigPatch
		if err := c.Bind(&patch); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.PatchDatadir(patch)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "datadir").Bool("active", result.Active).Msg("chaos config patched")
		return c.JSON(http.StatusOK, result)
	case "database":
		var patch chaos.DatabaseChaosConfigPatch
		if err := c.Bind(&patch); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.PatchDatabase(patch)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "database").Bool("active", result.Active).Msg("chaos config patched")
		return c.JSON(http.StatusOK, result)
	case "memory":
		var patch chaos.MemoryConfigPatch
		if err := c.Bind(&patch); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		result, err := s.cm.PatchMemory(patch)
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "memory").Bool("active", result.Active).Msg("chaos config patched")
		return c.JSON(http.StatusOK, result)
	default:
		return echo.ErrNotFound
	}
}

// handleChaosPost activates a chaos worker with its current config.
// Returns 409 Conflict if the worker is already active.
func (s *Server) handleChaosPost(c echo.Context) error {
	switch c.Param("kind") {
	case "logs":
		result, err := s.cm.ActivateLogs()
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "logs").Msg("chaos worker activated via POST")
		return c.JSON(http.StatusOK, result)
	case "datadir":
		result, err := s.cm.ActivateDatadir()
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "datadir").Msg("chaos worker activated via POST")
		return c.JSON(http.StatusOK, result)
	case "database":
		result, err := s.cm.ActivateDatabase()
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "database").Msg("chaos worker activated via POST")
		return c.JSON(http.StatusOK, result)
	case "memory":
		result, err := s.cm.ActivateMemory()
		if err != nil {
			return chaosHTTPError(err)
		}
		logger.Get().Warn().Str("chaos", "memory").Msg("chaos worker activated via POST")
		return c.JSON(http.StatusOK, result)
	default:
		return echo.ErrNotFound
	}
}

// handleChaosDel stops a chaos worker and resets its config to defaults.
func (s *Server) handleChaosDel(c echo.Context) error {
	switch c.Param("kind") {
	case "logs":
		result := s.cm.ResetLogs()
		logger.Get().Info().Str("chaos", "logs").Msg("chaos worker stopped and reset via DELETE")
		return c.JSON(http.StatusOK, result)
	case "datadir":
		result := s.cm.ResetDatadir()
		logger.Get().Info().Str("chaos", "datadir").Msg("chaos worker stopped and reset via DELETE")
		return c.JSON(http.StatusOK, result)
	case "database":
		result := s.cm.ResetDatabase()
		logger.Get().Info().Str("chaos", "database").Msg("chaos worker stopped and reset via DELETE")
		return c.JSON(http.StatusOK, result)
	case "memory":
		result := s.cm.ResetMemory()
		logger.Get().Info().Str("chaos", "memory").Msg("chaos worker stopped and reset via DELETE")
		return c.JSON(http.StatusOK, result)
	default:
		return echo.ErrNotFound
	}
}

// chaosHTTPError maps known chaos sentinel errors to appropriate HTTP statuses.
func chaosHTTPError(err error) error {
	switch {
	case errors.Is(err, chaos.ErrAlreadyActive):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, chaos.ErrNoDB):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	default:
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}
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
