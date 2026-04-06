package chaos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	"github.com/thor/dumbs/internal/logger"
)

// runLogs floods the log output with ~100 MB/min of structured entries.
//
// Pre-generated hex chunks are rotated to avoid hammering crypto/rand on every
// line. Each log line is ~1 100 bytes (two 1 024-char hex fields + JSON overhead)
// which at one line per 660 µs gives ~1.67 MB/s ≈ 100 MB/min.
func (m *Manager) runLogs(ctx context.Context) {
	const chunkCount = 64
	const rawSize = 512 // 512 bytes → 1024-char hex string

	chunks := make([]string, chunkCount)
	for i := range chunks {
		raw := make([]byte, rawSize)
		_, _ = io.ReadFull(rand.Reader, raw)
		chunks[i] = hex.EncodeToString(raw)
	}

	ticker := time.NewTicker(660 * time.Microsecond)
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
