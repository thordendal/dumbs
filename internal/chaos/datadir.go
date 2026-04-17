package chaos

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thordendal/dumbs/internal/logger"
)

const chaosSubdir = "chaos"

// runDatadir creates sequential binary files filled with random data inside
// <data_dir>/chaos/ and never deletes them.
// File size and inter-file delay are read from cfgDatadir at goroutine start;
// use ApplyDatadir/PatchDatadir to change them (which stop+restarts the worker).
// Set interval_ms=0 to write as fast as possible.
// The lesson: disks fill up. Monitor with `df -h`.
func (m *Manager) runDatadir(ctx context.Context) {
	m.mu.Lock()
	cfg := m.cfgDatadir
	m.mu.Unlock()

	dir := filepath.Join(m.cfg.Get().App.DataDir, chaosSubdir)
	if err := os.MkdirAll(dir, 0750); err != nil {
		logger.Get().Error().Err(err).Str("dir", dir).Msg("chaos/datadir: mkdir failed")
		return
	}

	buf := make([]byte, cfg.FileSizeBytes)
	seq := 0
	for {
		if ctx.Err() != nil {
			return
		}

		if _, err := rand.Read(buf); err != nil {
			logger.Get().Error().Err(err).Msg("chaos/datadir: rand.Read failed")
			return
		}

		name := filepath.Join(dir, fmt.Sprintf("chaos-%06d.bin", seq))
		f, err := os.Create(name)
		if err != nil {
			logger.Get().Error().Err(err).Str("file", name).Msg("chaos/datadir: create failed")
			return
		}
		if _, err := f.Write(buf); err != nil {
			logger.Get().Error().Err(err).Str("file", name).Msg("chaos/datadir: write failed")
			f.Close()
			return
		}
		f.Close()
		logger.Get().Warn().Str("file", name).Int("bytes", cfg.FileSizeBytes).Msg("chaos/datadir: file written")
		seq++

		if cfg.IntervalMs > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(cfg.IntervalMs) * time.Millisecond):
			}
		}
	}
}
