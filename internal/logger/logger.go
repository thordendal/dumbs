// Package logger provides a global zerolog logger with hot-reload support.
// Callers should call Get() at the point of use (not store the result long-term)
// so that level and output changes from config reloads take effect immediately.
package logger

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/thordendal/dumbs/internal/config"
)

var (
	mu      sync.Mutex
	current zerolog.Logger
	closer  io.Closer // non-nil when output is a file; closed on next Apply
)

// Init initialises the global logger from cfg. Must be called before Get.
func Init(cfg config.Config) error {
	mu.Lock()
	defer mu.Unlock()
	return apply(cfg)
}

// Apply hot-swaps the global logger's level and output from cfg.
// Safe to call from any goroutine; previous file handle is closed.
func Apply(cfg config.Config) error {
	mu.Lock()
	defer mu.Unlock()
	return apply(cfg)
}

func apply(cfg config.Config) error {
	// Close previous file handle if any.
	if closer != nil {
		_ = closer.Close()
		closer = nil
	}

	var base io.Writer
	if cfg.Log.Output == "" || cfg.Log.Output == "stdout" {
		base = os.Stdout
	} else {
		f, err := os.OpenFile(cfg.Log.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return err
		}
		closer = f
		base = f
	}

	var w io.Writer
	if strings.ToLower(cfg.Log.Format) == "plain" {
		w = zerolog.ConsoleWriter{Out: base, TimeFormat: time.RFC3339}
	} else {
		w = base
	}

	current = zerolog.New(w).With().Timestamp().Logger().Level(parseLevel(cfg.Log.Level))
	return nil
}

// Get returns a pointer to a copy of the current global logger.
// zerolog v1.33+ defines Warn/Error/Info/etc. on *Logger (pointer receivers),
// so returning a pointer avoids "cannot call pointer method" errors on the
// non-addressable return value of a function.
// Each call produces a fresh copy so callers can't accidentally mutate the
// global logger's context.
func Get() *zerolog.Logger {
	mu.Lock()
	l := current
	mu.Unlock()
	return &l
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
