package chaos

import "fmt"

// LogsConfig is the resource document for the log-flood chaos worker.
type LogsConfig struct {
	Kind         string `json:"kind"`
	Active       bool   `json:"active"`
	RatePerSec   int    `json:"rate_per_sec"`  // log lines/sec (default 1520 ≈ 100 MB/min)
	PayloadBytes int    `json:"payload_bytes"` // raw bytes per hex chunk (default 512)
}

// LogsConfigPatch is the PATCH request body; nil fields are left unchanged.
type LogsConfigPatch struct {
	Active       *bool `json:"active"`
	RatePerSec   *int  `json:"rate_per_sec"`
	PayloadBytes *int  `json:"payload_bytes"`
}

func defaultLogsConfig() LogsConfig {
	return LogsConfig{Kind: "logs", RatePerSec: 1520, PayloadBytes: 512}
}

func (p LogsConfigPatch) apply(c LogsConfig) LogsConfig {
	if p.Active != nil {
		c.Active = *p.Active
	}
	if p.RatePerSec != nil {
		c.RatePerSec = *p.RatePerSec
	}
	if p.PayloadBytes != nil {
		c.PayloadBytes = *p.PayloadBytes
	}
	return c
}

func validateLogsConfig(c LogsConfig) error {
	if c.RatePerSec <= 0 {
		return fmt.Errorf("rate_per_sec must be positive")
	}
	if c.PayloadBytes <= 0 {
		return fmt.Errorf("payload_bytes must be positive")
	}
	return nil
}

// DatadirConfig is the resource document for the data-dir flood chaos worker.
type DatadirConfig struct {
	Kind          string `json:"kind"`
	Active        bool   `json:"active"`
	FileSizeBytes int    `json:"file_size_bytes"` // bytes per file (default 10 MiB)
}

// DatadirConfigPatch is the PATCH request body; nil fields are left unchanged.
type DatadirConfigPatch struct {
	Active        *bool `json:"active"`
	FileSizeBytes *int  `json:"file_size_bytes"`
}

func defaultDatadirConfig() DatadirConfig {
	return DatadirConfig{Kind: "datadir", FileSizeBytes: 10 * 1024 * 1024}
}

func (p DatadirConfigPatch) apply(c DatadirConfig) DatadirConfig {
	if p.Active != nil {
		c.Active = *p.Active
	}
	if p.FileSizeBytes != nil {
		c.FileSizeBytes = *p.FileSizeBytes
	}
	return c
}

func validateDatadirConfig(c DatadirConfig) error {
	if c.FileSizeBytes <= 0 {
		return fmt.Errorf("file_size_bytes must be positive")
	}
	return nil
}

// DatabaseChaosConfig is the resource document for the database chaos worker.
type DatabaseChaosConfig struct {
	Kind                 string `json:"kind"`
	Active               bool   `json:"active"`
	InsertBatchSize      int    `json:"insert_batch_size"`       // rows per CopyFrom batch (default 1000)
	BadQueryIntervalMs   int    `json:"bad_query_interval_ms"`   // ms between bad queries (default 200)
	HangingTxnIntervalMs int    `json:"hanging_txn_interval_ms"` // ms between new hanging txns (default 2000)
}

// DatabaseChaosConfigPatch is the PATCH request body; nil fields are left unchanged.
type DatabaseChaosConfigPatch struct {
	Active               *bool `json:"active"`
	InsertBatchSize      *int  `json:"insert_batch_size"`
	BadQueryIntervalMs   *int  `json:"bad_query_interval_ms"`
	HangingTxnIntervalMs *int  `json:"hanging_txn_interval_ms"`
}

func defaultDatabaseChaosConfig() DatabaseChaosConfig {
	return DatabaseChaosConfig{
		Kind:                 "database",
		InsertBatchSize:      1000,
		BadQueryIntervalMs:   200,
		HangingTxnIntervalMs: 2000,
	}
}

func (p DatabaseChaosConfigPatch) apply(c DatabaseChaosConfig) DatabaseChaosConfig {
	if p.Active != nil {
		c.Active = *p.Active
	}
	if p.InsertBatchSize != nil {
		c.InsertBatchSize = *p.InsertBatchSize
	}
	if p.BadQueryIntervalMs != nil {
		c.BadQueryIntervalMs = *p.BadQueryIntervalMs
	}
	if p.HangingTxnIntervalMs != nil {
		c.HangingTxnIntervalMs = *p.HangingTxnIntervalMs
	}
	return c
}

func validateDatabaseChaosConfig(c DatabaseChaosConfig) error {
	if c.InsertBatchSize <= 0 {
		return fmt.Errorf("insert_batch_size must be positive")
	}
	if c.BadQueryIntervalMs <= 0 {
		return fmt.Errorf("bad_query_interval_ms must be positive")
	}
	if c.HangingTxnIntervalMs <= 0 {
		return fmt.Errorf("hanging_txn_interval_ms must be positive")
	}
	return nil
}

// MemoryConfig is the resource document for the memory-leak chaos worker.
type MemoryConfig struct {
	Kind           string `json:"kind"`
	Active         bool   `json:"active"`
	ChunkSizeBytes int    `json:"chunk_size_bytes"` // bytes appended per tick (default 870 KiB)
	IntervalMs     int    `json:"interval_ms"`      // ms between ticks (default 1000)
}

// MemoryConfigPatch is the PATCH request body; nil fields are left unchanged.
type MemoryConfigPatch struct {
	Active         *bool `json:"active"`
	ChunkSizeBytes *int  `json:"chunk_size_bytes"`
	IntervalMs     *int  `json:"interval_ms"`
}

func defaultMemoryConfig() MemoryConfig {
	return MemoryConfig{Kind: "memory", ChunkSizeBytes: 870 * 1024, IntervalMs: 1000}
}

func (p MemoryConfigPatch) apply(c MemoryConfig) MemoryConfig {
	if p.Active != nil {
		c.Active = *p.Active
	}
	if p.ChunkSizeBytes != nil {
		c.ChunkSizeBytes = *p.ChunkSizeBytes
	}
	if p.IntervalMs != nil {
		c.IntervalMs = *p.IntervalMs
	}
	return c
}

func validateMemoryConfig(c MemoryConfig) error {
	if c.ChunkSizeBytes <= 0 {
		return fmt.Errorf("chunk_size_bytes must be positive")
	}
	if c.IntervalMs <= 0 {
		return fmt.Errorf("interval_ms must be positive")
	}
	return nil
}
