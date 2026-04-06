// Package chaos implements the ChaosManager and all chaos worker goroutines.
//
// Each worker is a REST resource at /chaos/:kind. The resource document holds
// both configuration and active state. Workers are started and stopped via
// context cancellation; the Manager holds a *worker per chaos type (nil = not
// running). All exported methods on Manager are safe for concurrent use.
package chaos

import (
	"context"
	"errors"
	"sync"

	"github.com/thor/dumbs/internal/config"
	"github.com/thor/dumbs/internal/database"
	"github.com/thor/dumbs/internal/metrics"
)

// Sentinel errors returned by Manager methods.
var (
	// ErrNoDB is returned when database chaos is requested but no DB is configured.
	ErrNoDB = errors.New("database not configured; set app.database.dsn in config")
	// ErrAlreadyActive is returned by Activate* when the worker is already running.
	ErrAlreadyActive = errors.New("worker is already active")
)

// worker tracks the lifecycle of a single chaos goroutine.
type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager owns all chaos workers and the resources they share.
type Manager struct {
	cfg     *config.Loader
	db      *database.DB
	metrics *metrics.Metrics

	mu          sync.Mutex
	leak        [][]byte // memory leak accumulator; held here to prevent GC
	cfgLogs     LogsConfig
	cfgDatadir  DatadirConfig
	cfgDatabase DatabaseChaosConfig
	cfgMemory   MemoryConfig

	wLogs   *worker
	wData   *worker
	wDB     *worker
	wMemory *worker
}

// NewManager constructs a Manager with all workers initialised to their default
// configs and not running. db may be nil; ApplyDatabase/ActivateDatabase will
// return ErrNoDB in that case.
func NewManager(cfg *config.Loader, db *database.DB, m *metrics.Metrics) *Manager {
	return &Manager{
		cfg:         cfg,
		db:          db,
		metrics:     m,
		cfgLogs:     defaultLogsConfig(),
		cfgDatadir:  defaultDatadirConfig(),
		cfgDatabase: defaultDatabaseChaosConfig(),
		cfgMemory:   defaultMemoryConfig(),
	}
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

// ---- Logs ----------------------------------------------------------------

// GetLogs returns the current logs config with the live active state.
func (m *Manager) GetLogs() LogsConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfgLogs
	cfg.Active = m.wLogs != nil
	return cfg
}

// ApplyLogs replaces the stored logs config and starts/stops/restarts the
// worker according to cfg.Active. Config change while running = stop+restart.
func (m *Manager) ApplyLogs(cfg LogsConfig) (LogsConfig, error) {
	cfg.Kind = "logs"
	if err := validateLogsConfig(cfg); err != nil {
		return m.GetLogs(), err
	}
	if stopped := m.stopWorker(&m.wLogs); stopped {
		m.metrics.ChaosLogsActive.Set(0)
	}
	m.mu.Lock()
	m.cfgLogs = cfg
	m.mu.Unlock()
	if cfg.Active {
		m.startWorker(&m.wLogs, m.runLogs)
		m.metrics.ChaosLogsActive.Set(1)
	}
	return m.GetLogs(), nil
}

// PatchLogs applies a partial update to the logs config.
func (m *Manager) PatchLogs(patch LogsConfigPatch) (LogsConfig, error) {
	m.mu.Lock()
	current := m.cfgLogs
	current.Active = m.wLogs != nil
	merged := patch.apply(current)
	m.mu.Unlock()
	return m.ApplyLogs(merged)
}

// ActivateLogs starts the logs worker with the current config.
// Returns ErrAlreadyActive if the worker is already running.
func (m *Manager) ActivateLogs() (LogsConfig, error) {
	if !m.startWorker(&m.wLogs, m.runLogs) {
		return m.GetLogs(), ErrAlreadyActive
	}
	m.metrics.ChaosLogsActive.Set(1)
	return m.GetLogs(), nil
}

// ResetLogs stops the logs worker and resets config to defaults.
func (m *Manager) ResetLogs() LogsConfig {
	if m.stopWorker(&m.wLogs) {
		m.metrics.ChaosLogsActive.Set(0)
	}
	m.mu.Lock()
	m.cfgLogs = defaultLogsConfig()
	m.mu.Unlock()
	return m.GetLogs()
}

// ---- Datadir -------------------------------------------------------------

// GetDatadir returns the current datadir config with the live active state.
func (m *Manager) GetDatadir() DatadirConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfgDatadir
	cfg.Active = m.wData != nil
	return cfg
}

// ApplyDatadir replaces the stored datadir config and starts/stops/restarts
// the worker according to cfg.Active.
func (m *Manager) ApplyDatadir(cfg DatadirConfig) (DatadirConfig, error) {
	cfg.Kind = "datadir"
	if err := validateDatadirConfig(cfg); err != nil {
		return m.GetDatadir(), err
	}
	if stopped := m.stopWorker(&m.wData); stopped {
		m.metrics.ChaosDatdirActive.Set(0)
	}
	m.mu.Lock()
	m.cfgDatadir = cfg
	m.mu.Unlock()
	if cfg.Active {
		m.startWorker(&m.wData, m.runDatadir)
		m.metrics.ChaosDatdirActive.Set(1)
	}
	return m.GetDatadir(), nil
}

// PatchDatadir applies a partial update to the datadir config.
func (m *Manager) PatchDatadir(patch DatadirConfigPatch) (DatadirConfig, error) {
	m.mu.Lock()
	current := m.cfgDatadir
	current.Active = m.wData != nil
	merged := patch.apply(current)
	m.mu.Unlock()
	return m.ApplyDatadir(merged)
}

// ActivateDatadir starts the datadir worker with the current config.
// Returns ErrAlreadyActive if the worker is already running.
func (m *Manager) ActivateDatadir() (DatadirConfig, error) {
	if !m.startWorker(&m.wData, m.runDatadir) {
		return m.GetDatadir(), ErrAlreadyActive
	}
	m.metrics.ChaosDatdirActive.Set(1)
	return m.GetDatadir(), nil
}

// ResetDatadir stops the datadir worker and resets config to defaults.
func (m *Manager) ResetDatadir() DatadirConfig {
	if m.stopWorker(&m.wData) {
		m.metrics.ChaosDatdirActive.Set(0)
	}
	m.mu.Lock()
	m.cfgDatadir = defaultDatadirConfig()
	m.mu.Unlock()
	return m.GetDatadir()
}

// ---- Database ------------------------------------------------------------

// GetDatabase returns the current database chaos config with the live active state.
func (m *Manager) GetDatabase() DatabaseChaosConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfgDatabase
	cfg.Active = m.wDB != nil
	return cfg
}

// ApplyDatabase replaces the stored database chaos config and starts/stops/restarts
// the worker according to cfg.Active. Returns ErrNoDB if active=true and no DB.
func (m *Manager) ApplyDatabase(cfg DatabaseChaosConfig) (DatabaseChaosConfig, error) {
	cfg.Kind = "database"
	if cfg.Active && m.db == nil {
		return m.GetDatabase(), ErrNoDB
	}
	if err := validateDatabaseChaosConfig(cfg); err != nil {
		return m.GetDatabase(), err
	}
	if stopped := m.stopWorker(&m.wDB); stopped {
		m.metrics.ChaosDatabaseActive.Set(0)
	}
	m.mu.Lock()
	m.cfgDatabase = cfg
	m.mu.Unlock()
	if cfg.Active {
		m.startWorker(&m.wDB, m.runDatabase)
		m.metrics.ChaosDatabaseActive.Set(1)
	}
	return m.GetDatabase(), nil
}

// PatchDatabase applies a partial update to the database chaos config.
func (m *Manager) PatchDatabase(patch DatabaseChaosConfigPatch) (DatabaseChaosConfig, error) {
	m.mu.Lock()
	current := m.cfgDatabase
	current.Active = m.wDB != nil
	merged := patch.apply(current)
	m.mu.Unlock()
	return m.ApplyDatabase(merged)
}

// ActivateDatabase starts the database chaos worker with the current config.
// Returns ErrNoDB if no database is configured, ErrAlreadyActive if running.
func (m *Manager) ActivateDatabase() (DatabaseChaosConfig, error) {
	if m.db == nil {
		return m.GetDatabase(), ErrNoDB
	}
	if !m.startWorker(&m.wDB, m.runDatabase) {
		return m.GetDatabase(), ErrAlreadyActive
	}
	m.metrics.ChaosDatabaseActive.Set(1)
	return m.GetDatabase(), nil
}

// ResetDatabase stops the database chaos worker and resets config to defaults.
func (m *Manager) ResetDatabase() DatabaseChaosConfig {
	if m.stopWorker(&m.wDB) {
		m.metrics.ChaosDatabaseActive.Set(0)
	}
	m.mu.Lock()
	m.cfgDatabase = defaultDatabaseChaosConfig()
	m.mu.Unlock()
	return m.GetDatabase()
}

// ---- Memory --------------------------------------------------------------

// GetMemory returns the current memory config with the live active state.
func (m *Manager) GetMemory() MemoryConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfgMemory
	cfg.Active = m.wMemory != nil
	return cfg
}

// ApplyMemory replaces the stored memory config and starts/stops/restarts the
// worker according to cfg.Active. Stopping also nils the leak slice so the GC
// can eventually reclaim the allocated memory.
func (m *Manager) ApplyMemory(cfg MemoryConfig) (MemoryConfig, error) {
	cfg.Kind = "memory"
	if err := validateMemoryConfig(cfg); err != nil {
		return m.GetMemory(), err
	}
	if stopped := m.stopWorker(&m.wMemory); stopped {
		m.metrics.ChaosMemoryActive.Set(0)
		m.mu.Lock()
		m.leak = nil
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.cfgMemory = cfg
	m.mu.Unlock()
	if cfg.Active {
		m.startWorker(&m.wMemory, m.runMemory)
		m.metrics.ChaosMemoryActive.Set(1)
	}
	return m.GetMemory(), nil
}

// PatchMemory applies a partial update to the memory config.
func (m *Manager) PatchMemory(patch MemoryConfigPatch) (MemoryConfig, error) {
	m.mu.Lock()
	current := m.cfgMemory
	current.Active = m.wMemory != nil
	merged := patch.apply(current)
	m.mu.Unlock()
	return m.ApplyMemory(merged)
}

// ActivateMemory starts the memory leak worker with the current config.
// Returns ErrAlreadyActive if the worker is already running.
func (m *Manager) ActivateMemory() (MemoryConfig, error) {
	if !m.startWorker(&m.wMemory, m.runMemory) {
		return m.GetMemory(), ErrAlreadyActive
	}
	m.metrics.ChaosMemoryActive.Set(1)
	return m.GetMemory(), nil
}

// ResetMemory stops the memory leak worker and resets config to defaults.
func (m *Manager) ResetMemory() MemoryConfig {
	if m.stopWorker(&m.wMemory) {
		m.metrics.ChaosMemoryActive.Set(0)
		m.mu.Lock()
		m.leak = nil
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.cfgMemory = defaultMemoryConfig()
	m.mu.Unlock()
	return m.GetMemory()
}

// ---- Collection-level ----------------------------------------------------

// GetAll returns all four worker resource documents as a slice.
func (m *Manager) GetAll() []any {
	return []any{
		m.GetLogs(),
		m.GetDatadir(),
		m.GetDatabase(),
		m.GetMemory(),
	}
}

// ResetAll stops all workers and resets all configs to defaults.
// Used by DELETE /chaos.
func (m *Manager) ResetAll() {
	m.ResetLogs()
	m.ResetDatadir()
	m.ResetDatabase()
	m.ResetMemory()
}

// StopAll stops every active chaos worker without resetting config.
// Used by signal handling (SIGUSR1 and SIGQUIT).
func (m *Manager) StopAll() {
	if m.stopWorker(&m.wLogs) {
		m.metrics.ChaosLogsActive.Set(0)
	}
	if m.stopWorker(&m.wData) {
		m.metrics.ChaosDatdirActive.Set(0)
	}
	if m.stopWorker(&m.wDB) {
		m.metrics.ChaosDatabaseActive.Set(0)
	}
	if m.stopWorker(&m.wMemory) {
		m.metrics.ChaosMemoryActive.Set(0)
		m.mu.Lock()
		m.leak = nil
		m.mu.Unlock()
	}
}
