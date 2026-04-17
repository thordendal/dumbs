package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thordendal/dumbs/internal/chaos"
	"github.com/thordendal/dumbs/internal/config"
	"github.com/thordendal/dumbs/internal/database"
	"github.com/thordendal/dumbs/internal/health"
	"github.com/thordendal/dumbs/internal/logger"
	"github.com/thordendal/dumbs/internal/metrics"
)

// Server wraps an Echo instance and its dependencies.
type Server struct {
	e   *echo.Echo
	cfg *config.Loader
	db  *database.DB
	h   *health.Health
	m   *metrics.Metrics
	cm  *chaos.Manager
}

// New creates and wires a Server. Call Start to begin accepting connections.
func New(cfg *config.Loader, db *database.DB, h *health.Health, m *metrics.Metrics, cm *chaos.Manager) *Server {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Custom error handler: logs 5xx through zerolog instead of Echo's internal logger.
	e.HTTPErrorHandler = httpErrorHandler

	// ---- Middleware stack (order matters) ----

	// 1. Recover from panics before anything else can touch the response.
	e.Use(middleware.Recover())

	// 2. Assign / propagate X-Request-ID so every subsequent log line can carry it.
	e.Use(middleware.RequestID())

	// 3. Access log — one structured line per completed request.
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:       true,
		LogURI:          true,
		LogStatus:       true,
		LogLatency:      true,
		LogRemoteIP:     true,
		LogRequestID:    true,
		LogError:        true,
		LogResponseSize: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ev := logger.Get().Info()
			if v.Status >= 500 {
				ev = logger.Get().Error()
			} else if v.Status >= 400 {
				ev = logger.Get().Warn()
			}
			ev = ev.
				Str("remote_ip", v.RemoteIP).
				Str("method", v.Method).
				Str("uri", v.URI).
				Int("status", v.Status).
				Dur("latency", v.Latency).
				Int64("bytes_out", v.ResponseSize).
				Str("request_id", v.RequestID)
			if v.Error != nil {
				ev = ev.Err(v.Error)
			}
			ev.Msg("access")
			return nil
		},
	}))

	// 4. Reject oversized request bodies before handlers read them.
	e.Use(middleware.BodyLimit("1MB"))

	// 5. Per-request deadline — prevents slow clients from holding goroutines forever.
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 30 * time.Second,
	}))

	// 6. Prometheus request metrics into our private registry.
	e.Use(echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
		Subsystem:  "dumbs",
		Registerer: m.Registry,
	}))

	s := &Server{e: e, cfg: cfg, db: db, h: h, m: m, cm: cm}
	s.routes()
	return s
}

// httpErrorHandler is Echo's global error handler. It logs 5xx errors via
// zerolog and writes a plain JSON body so the client always gets a response.
func httpErrorHandler(err error, c echo.Context) {
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		he = &echo.HTTPError{Code: http.StatusInternalServerError, Message: err.Error()}
	}

	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	if he.Code >= 500 {
		logger.Get().Error().
			Err(err).
			Str("request_id", rid).
			Str("method", c.Request().Method).
			Str("uri", c.Request().RequestURI).
			Int("status", he.Code).
			Msg("unhandled error")
	}

	if !c.Response().Committed {
		msg := he.Message
		if s, ok := msg.(string); ok {
			_ = c.JSON(he.Code, map[string]string{"error": s})
		} else {
			_ = c.JSON(he.Code, map[string]any{"error": msg})
		}
	}
}

func (s *Server) routes() {
	s.e.GET("/", s.handleUIIndex)
	s.e.GET("/workers/:kind", s.handleUIWorker)
	s.e.POST("/workers/:kind/stop", s.handleUIWorkerStop)
	s.e.POST("/workers/:kind/start", s.handleUIWorkerStart)
	s.e.POST("/workers/:kind/reset-db", s.handleUIWorkerResetDB)

	s.e.GET("/health/live", s.handleLive)
	s.e.GET("/health/ready", s.handleReady)
	s.e.GET("/metrics", s.handleMetrics)
	s.e.POST("/config/reload", s.handleConfigReload)

	s.e.GET("/chaos", s.handleChaosList)
	s.e.DELETE("/chaos", s.handleChaosDeleteAll)
	s.e.GET("/chaos/:kind", s.handleChaosGet)
	s.e.PUT("/chaos/:kind", s.handleChaosPut)
	s.e.PATCH("/chaos/:kind", s.handleChaosPatch)
	s.e.POST("/chaos/:kind", s.handleChaosPost)
	s.e.DELETE("/chaos/:kind", s.handleChaosDel)

	s.e.POST("/database/reset", s.handleDatabaseReset)
}

// Start begins listening on addr. Blocks until the server stops.
func (s *Server) Start(addr string) error {
	logger.Get().Info().Str("addr", addr).Msg("server: listening")
	return s.e.Start(addr)
}

// Shutdown gracefully drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.e.Shutdown(ctx)
}

// handleMetrics serves all Prometheus metrics from the private registry.
func (s *Server) handleMetrics(c echo.Context) error {
	promhttp.HandlerFor(s.m.Registry, promhttp.HandlerOpts{}).ServeHTTP(c.Response(), c.Request())
	return nil
}
