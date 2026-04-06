package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds the Prometheus registry and every custom dumbs metric.
type Metrics struct {
	Registry            *prometheus.Registry
	ChaosLogsActive     prometheus.Gauge
	ChaosDatdirActive   prometheus.Gauge
	ChaosDatabaseActive prometheus.Gauge
	ChaosMemoryActive   prometheus.Gauge
	ConfigReloads       prometheus.Counter
}

// New creates the Prometheus registry and registers all custom metrics plus the
// standard Go runtime and process collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		ChaosLogsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dumbs_chaos_logs_active",
			Help: "1 while log-flooding chaos is running, 0 otherwise.",
		}),
		ChaosDatdirActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dumbs_chaos_datadir_active",
			Help: "1 while data-dir flooding chaos is running, 0 otherwise.",
		}),
		ChaosDatabaseActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dumbs_chaos_database_active",
			Help: "1 while database chaos is running, 0 otherwise.",
		}),
		ChaosMemoryActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dumbs_chaos_memory_active",
			Help: "1 while memory-leak chaos is running, 0 otherwise.",
		}),
		ConfigReloads: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dumbs_config_reloads_total",
			Help: "Total number of successful config reloads.",
		}),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.ChaosLogsActive,
		m.ChaosDatdirActive,
		m.ChaosDatabaseActive,
		m.ChaosMemoryActive,
		m.ConfigReloads,
	)
	return m
}
