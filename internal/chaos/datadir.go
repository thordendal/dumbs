package chaos

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thor/dumbs/internal/logger"
)

const (
	chaosSubdir = "chaos"
	fileSize    = 10 * 1024 * 1024 // 10 MiB per file
)

// runDatadir creates sequential 10 MiB binary files filled with random data
// inside <data_dir>/chaos/ as fast as possible, and never deletes them.
// The lesson: disks fill up. Monitor with `df -h`.
func (m *Manager) runDatadir(ctx context.Context) {
	dir := filepath.Join(m.cfg.Get().App.DataDir, chaosSubdir)
	if err := os.MkdirAll(dir, 0750); err != nil {
		logger.Get().Error().Err(err).Str("dir", dir).Msg("chaos/datadir: mkdir failed")
		return
	}

	buf := make([]byte, fileSize)
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
		logger.Get().Warn().Str("file", name).Int("bytes", fileSize).Msg("chaos/datadir: file written")
		seq++
	}
}
