package server

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/thordendal/dumbs/internal/chaos"
	"github.com/thordendal/dumbs/internal/logger"
)

//go:embed templates
var templateFS embed.FS

var (
	tmplIndex  = template.Must(template.ParseFS(templateFS, "templates/index.html"))
	tmplWorker = template.Must(template.ParseFS(templateFS, "templates/worker.html"))
)

// workerSummary is the data for one row on the index page.
type workerSummary struct {
	Kind   string
	Active bool
}

// workerPageData holds all fields needed to render the worker detail page.
// Only the fields relevant to the given Kind are populated.
type workerPageData struct {
	Kind   string
	Active bool
	Flash  string
	// logs
	RatePerSec   int
	PayloadBytes int
	// datadir
	FileSizeBytes int
	// database
	InsertBatchSize      int
	BadQueryIntervalMs   int
	HangingTxnIntervalMs int
	// memory
	ChunkSizeBytes int
	IntervalMs     int
}

// handleUIIndex renders the index page listing all chaos workers.
func (s *Server) handleUIIndex(c echo.Context) error {
	logs := s.cm.GetLogs()
	datadir := s.cm.GetDatadir()
	db := s.cm.GetDatabase()
	mem := s.cm.GetMemory()
	workers := []workerSummary{
		{Kind: logs.Kind, Active: logs.Active},
		{Kind: datadir.Kind, Active: datadir.Active},
		{Kind: db.Kind, Active: db.Active},
		{Kind: mem.Kind, Active: mem.Active},
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmplIndex.Execute(c.Response(), workers)
}

// handleUIWorker renders the detail page for a single chaos worker.
func (s *Server) handleUIWorker(c echo.Context) error {
	return s.renderWorkerPage(c, "")
}

// renderWorkerPage builds the worker detail page data and executes the template.
// flash is an optional error message shown at the top of the page.
func (s *Server) renderWorkerPage(c echo.Context, flash string) error {
	kind := c.Param("kind")
	data := workerPageData{Flash: flash}
	switch kind {
	case "logs":
		cfg := s.cm.GetLogs()
		data.Kind = cfg.Kind
		data.Active = cfg.Active
		data.RatePerSec = cfg.RatePerSec
		data.PayloadBytes = cfg.PayloadBytes
	case "datadir":
		cfg := s.cm.GetDatadir()
		data.Kind = cfg.Kind
		data.Active = cfg.Active
		data.FileSizeBytes = cfg.FileSizeBytes
		data.IntervalMs = cfg.IntervalMs
	case "database":
		cfg := s.cm.GetDatabase()
		data.Kind = cfg.Kind
		data.Active = cfg.Active
		data.InsertBatchSize = cfg.InsertBatchSize
		data.BadQueryIntervalMs = cfg.BadQueryIntervalMs
		data.HangingTxnIntervalMs = cfg.HangingTxnIntervalMs
	case "memory":
		cfg := s.cm.GetMemory()
		data.Kind = cfg.Kind
		data.Active = cfg.Active
		data.ChunkSizeBytes = cfg.ChunkSizeBytes
		data.IntervalMs = cfg.IntervalMs
	default:
		return echo.ErrNotFound
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmplWorker.Execute(c.Response(), data)
}

// handleUIWorkerStop stops the named chaos worker and redirects back to its page.
func (s *Server) handleUIWorkerStop(c echo.Context) error {
	kind := c.Param("kind")
	switch kind {
	case "logs":
		s.cm.ResetLogs()
	case "datadir":
		s.cm.ResetDatadir()
	case "database":
		s.cm.ResetDatabase()
	case "memory":
		s.cm.ResetMemory()
	default:
		return echo.ErrNotFound
	}
	logger.Get().Info().Str("chaos", kind).Msg("chaos worker stopped via UI")
	return c.Redirect(http.StatusSeeOther, "/workers/"+kind)
}

// handleUIWorkerStart parses the start form and applies the new config.
// On error the worker page is re-rendered with a flash message.
func (s *Server) handleUIWorkerStart(c echo.Context) error {
	kind := c.Param("kind")
	var applyErr error
	switch kind {
	case "logs":
		cfg := chaos.LogsConfig{
			Kind:         "logs",
			Active:       true,
			RatePerSec:   formInt(c, "rate_per_sec", 0),
			PayloadBytes: formInt(c, "payload_bytes", 0),
		}
		_, applyErr = s.cm.ApplyLogs(cfg)
	case "datadir":
		cfg := chaos.DatadirConfig{
			Kind:          "datadir",
			Active:        true,
			FileSizeBytes: formInt(c, "file_size_bytes", 0),
			IntervalMs:    formInt(c, "interval_ms", 0),
		}
		_, applyErr = s.cm.ApplyDatadir(cfg)
	case "database":
		cfg := chaos.DatabaseChaosConfig{
			Kind:                 "database",
			Active:               true,
			InsertBatchSize:      formInt(c, "insert_batch_size", 0),
			BadQueryIntervalMs:   formInt(c, "bad_query_interval_ms", 0),
			HangingTxnIntervalMs: formInt(c, "hanging_txn_interval_ms", 0),
		}
		_, applyErr = s.cm.ApplyDatabase(cfg)
	case "memory":
		cfg := chaos.MemoryConfig{
			Kind:           "memory",
			Active:         true,
			ChunkSizeBytes: formInt(c, "chunk_size_bytes", 0),
			IntervalMs:     formInt(c, "interval_ms", 0),
		}
		_, applyErr = s.cm.ApplyMemory(cfg)
	default:
		return echo.ErrNotFound
	}
	if applyErr != nil {
		return s.renderWorkerPage(c, applyErr.Error())
	}
	logger.Get().Warn().Str("chaos", kind).Msg("chaos worker started via UI")
	return c.Redirect(http.StatusSeeOther, "/workers/"+kind)
}

// handleUIWorkerResetDB drops and recreates the events table via the UI.
// Only valid for kind=database; others return 404.
func (s *Server) handleUIWorkerResetDB(c echo.Context) error {
	if c.Param("kind") != "database" {
		return echo.ErrNotFound
	}
	if s.db == nil {
		return s.renderWorkerPage(c, "database not configured")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()
	if err := s.db.ResetSchema(ctx); err != nil {
		logger.Get().Error().Err(err).Msg("database reset failed via UI")
		return s.renderWorkerPage(c, err.Error())
	}
	logger.Get().Info().Msg("database schema reset via UI")
	return c.Redirect(http.StatusSeeOther, "/workers/database")
}

// formInt reads a named form value and converts it to int.
// Returns fallback if the value is absent or cannot be parsed.
func formInt(c echo.Context, name string, fallback int) int {
	v := c.FormValue(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
