package chaos

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/thordendal/dumbs/internal/logger"
)

// runMemory appends random-byte chunks to the Manager's leak slice on each tick,
// accumulating memory at ~ChunkSizeBytes/IntervalMs. The slice is held on the
// Manager so the GC cannot collect it while the worker runs.
//
// Config is snapshotted at goroutine start; use ApplyMemory/PatchMemory to
// change it (which stop+restarts the worker).
//
// On stop the slice is nilled; RSS will drop on the next GC cycle — not
// instantly. The lesson: RSS in top doesn't always reflect allocator state.
func (m *Manager) runMemory(ctx context.Context) {
	m.mu.Lock()
	cfg := m.cfgMemory
	m.mu.Unlock()

	ticker := time.NewTicker(time.Duration(cfg.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			chunk := make([]byte, cfg.ChunkSizeBytes)
			if _, err := rand.Read(chunk); err != nil {
				logger.Get().Error().Err(err).Msg("chaos/memory: rand.Read failed")
				return
			}

			m.mu.Lock()
			m.leak = append(m.leak, chunk)
			total := int64(len(m.leak)) * int64(cfg.ChunkSizeBytes)
			m.mu.Unlock()

			logger.Get().Warn().
				Int64("total_leaked_bytes", total).
				Msg("chaos/memory: leak tick")
		}
	}
}
