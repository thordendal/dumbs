// Package chaos implements the ChaosManager and all chaos worker goroutines.
//
// Each worker is started and stopped via context cancellation. The Manager
// holds a *worker value per chaos type; nil means the worker is not running.
// All public methods on Manager are safe for concurrent use.
package chaos

import (
	"context"
	"errors"
	"sync"

	"github.com/thor/dumbs/internal/config"
	"github.com/thor/dumbs/internal/database"
	"github.com/thor/dumbs/internal/metrics"
)

// errNoDB is returned when database chaos is requested but no DB is configured.
var errNoDB = errors.New("database not configured; set app.database.dsn in config")

// worker tracks the lifecycle of a single chaos goroutine.
type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Status is returned by Manager.Status and serialised to JSON for GET /chaos/status.
type Status struct {
	Logs     bool `json:"logs"`
	Datadir  bool `json:"datadir"`
	Database bool `json:"database"`
	Memory   bool `json:"memory"`
}

// Manager owns all chaos workers and the resources they share.
type Manager struct {
	cfg     *config.Loader
	db      *database.DB
	metrics *metrics.Metrics

	// leak holds the memory accumulated by the memory-leak worker so that it
	// is reachable (and therefore not collected) while the worker runs.
	mu   sync.Mutex
	leak [][]byte

	wLogs   *worker
	wData   *worker
	wDB     *worker
	wMemory *worker
}

// NewManager constructs a Manager. db may be nil; calling StartDatabase will
// return an error in that case.
func NewManager(cfg *config.Loader, db *database.DB, m *metrics.Metrics) *Manager {
	return &Manager{cfg: cfg, db: db, metrics: m}
}

// startWorker launches fn in a new goroutine under a fresh cancellable context.
// Returns false without starting if the worker is already running.
func (m *Manager) startWorker(slot **worker, fn func(context.Context)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if *slot != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	*slot = &worker{cancel: cancel, done: done}
	go func() {
		defer close(done)
		fn(ctx)
	}()
	return true
}

// stopWorker cancels the worker in slot and blocks until it exits.
// Returns false if the worker was not running. Clears the slot.
func (m *Manager) stopWorker(slot **worker) bool {
	m.mu.Lock()
	w := *slot
	*slot = nil
	m.mu.Unlock()

	if w == nil {
		return false
	}
	w.cancel()
	<-w.done
	return true
}

// --- Logs ---

func (m *Manager) StartLogs() bool {
	ok := m.startWorker(&m.wLogs, m.runLogs)
	if ok {
		m.metrics.ChaosLogsActive.Set(1)
	}
	return ok
}

func (m *Manager) StopLogs() bool {
	ok := m.stopWorker(&m.wLogs)
	if ok {
		m.metrics.ChaosLogsActive.Set(0)
	}
	return ok
}

// --- Data dir ---

func (m *Manager) StartDatadir() bool {
	ok := m.startWorker(&m.wData, m.runDatadir)
	if ok {
		m.metrics.ChaosDatdirActive.Set(1)
	}
	return ok
}

func (m *Manager) StopDatadir() bool {
	ok := m.stopWorker(&m.wData)
	if ok {
		m.metrics.ChaosDatdirActive.Set(0)
	}
	return ok
}

// --- Database ---

// StartDatabase starts DB chaos. Returns (false, nil) if already running,
// (false, err) if DB is not configured.
func (m *Manager) StartDatabase() (bool, error) {
	if m.db == nil {
		return false, errNoDB
	}
	ok := m.startWorker(&m.wDB, m.runDatabase)
	if ok {
		m.metrics.ChaosDatabaseActive.Set(1)
	}
	return ok, nil
}

func (m *Manager) StopDatabase() bool {
	ok := m.stopWorker(&m.wDB)
	if ok {
		m.metrics.ChaosDatabaseActive.Set(0)
	}
	return ok
}

// --- Memory ---

func (m *Manager) StartMemory() bool {
	ok := m.startWorker(&m.wMemory, m.runMemory)
	if ok {
		m.metrics.ChaosMemoryActive.Set(1)
	}
	return ok
}

func (m *Manager) StopMemory() bool {
	ok := m.stopWorker(&m.wMemory)
	if ok {
		m.metrics.ChaosMemoryActive.Set(0)
		// Release the leak slice so the GC can eventually reclaim it.
		m.mu.Lock()
		m.leak = nil
		m.mu.Unlock()
	}
	return ok
}

// --- Global ---

// StopAll stops every active chaos worker and blocks until all have exited.
func (m *Manager) StopAll() {
	m.StopLogs()
	m.StopDatadir()
	m.StopDatabase()
	m.StopMemory()
}

// Status returns a point-in-time snapshot of which workers are running.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		Logs:     m.wLogs != nil,
		Datadir:  m.wData != nil,
		Database: m.wDB != nil,
		Memory:   m.wMemory != nil,
	}
}
