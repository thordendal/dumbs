package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// LogConfig holds logging-related configuration.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // json | plain
	Output string `yaml:"output"` // "stdout" or file path
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	DataDir  string         `yaml:"data_dir"`
	Database DatabaseConfig `yaml:"database"`
	Listen   string         `yaml:"listen"`
}

// Config is the top-level config structure matching the YAML layout.
type Config struct {
	Log LogConfig `yaml:"log"`
	App AppConfig `yaml:"app"`
}

func defaults() Config {
	return Config{
		Log: LogConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		App: AppConfig{
			DataDir: "/var/lib/dumbs",
			Listen:  ":8080",
		},
	}
}

// Loader holds the current config and the path it was loaded from.
// All exported methods are safe for concurrent use.
type Loader struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// Load reads the config file at path and returns a Loader.
func Load(path string) (*Loader, error) {
	l := &Loader{path: path, cfg: defaults()}
	if err := l.reload(); err != nil {
		return nil, err
	}
	return l, nil
}

// Reload re-reads the config file and updates the in-memory config.
func (l *Loader) Reload() error {
	return l.reload()
}

func (l *Loader) reload() error {
	f, err := os.Open(l.path)
	if err != nil {
		return fmt.Errorf("open config %q: %w", l.path, err)
	}
	defer f.Close()

	cfg := defaults()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	l.mu.Lock()
	l.cfg = cfg
	l.mu.Unlock()
	return nil
}

// Get returns a snapshot of the current config.
func (l *Loader) Get() Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cfg
}
