package chaos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	"github.com/thor/dumbs/internal/logger"
)

// runLogs floods the log output with structured entries at the rate and payload
// size configured in cfgLogs. Config is snapshotted at goroutine start; use
// ApplyLogs/PatchLogs to change it (which stop+restarts the worker).
func (m *Manager) runLogs(ctx context.Context) {
	m.mu.Lock()
	cfg := m.cfgLogs
	m.mu.Unlock()

	const chunkCount = 64
	rawSize := cfg.PayloadBytes

	chunks := make([]string, chunkCount)
	for i := range chunks {
		raw := make([]byte, rawSize)
		_, _ = io.ReadFull(rand.Reader, raw)
		chunks[i] = hex.EncodeToString(raw)
	}

	interval := time.Second / time.Duration(cfg.RatePerSec)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Get().Warn().
				Str("chaos", "logs").
				Str("payload_a", chunks[i%chunkCount]).
				Str("payload_b", chunks[(i+1)%chunkCount]).
				Str("trace_id", randomHex(8)).
				Str("request_id", randomHex(8)).
				Msg("chaos: log flood")
			i++
		}
	}
}

// randomHex returns 2*n hex characters from crypto/rand.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
