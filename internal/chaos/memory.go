package chaos

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/thor/dumbs/internal/logger"
)

const (
	// leakChunkSize is the number of bytes appended per tick (~870 KiB).
	// 870 KiB/s × 60 s ≈ 50 MiB/min.
	leakChunkSize = 870 * 1024
	leakInterval  = time.Second
)

// runMemory appends ~870 KiB of random bytes to the Manager's leak slice every
// second (~50 MiB/min). The slice is held on the Manager so it is reachable and
// the GC cannot collect it while the worker runs.
//
// On StopMemory the slice is nilled; resident memory will drop on the next GC
// cycle — not instantly. The lesson: RSS in top doesn't always reflect allocator
// state; use `go tool pprof` or watch /proc/<pid>/status.
func (m *Manager) runMemory(ctx context.Context) {
	ticker := time.NewTicker(leakInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			chunk := make([]byte, leakChunkSize)
			if _, err := rand.Read(chunk); err != nil {
				logger.Get().Error().Err(err).Msg("chaos/memory: rand.Read failed")
				return
			}

			m.mu.Lock()
			m.leak = append(m.leak, chunk)
			total := int64(len(m.leak)) * leakChunkSize
			m.mu.Unlock()

			logger.Get().Warn().
				Int64("total_leaked_bytes", total).
				Msg("chaos/memory: leak tick")
		}
	}
}
