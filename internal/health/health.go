package health

import (
	"context"
	"time"

	"github.com/thordendal/dumbs/internal/database"
)

// Health evaluates liveness and readiness probes.
type Health struct {
	db *database.DB
}

// New returns a Health checker. db may be nil — readiness will always fail.
func New(db *database.DB) *Health {
	return &Health{db: db}
}

// IsLive always returns true: if this code is running, the process is alive.
func (h *Health) IsLive() bool {
	return true
}

// IsReady returns (true, nil) when the database can be pinged successfully.
// Returns (false, err) with the underlying error so callers can log it.
// Returns (false, nil) when no database was configured.
func (h *Health) IsReady() (bool, error) {
	if h.db == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		return false, err
	}
	return true, nil
}
