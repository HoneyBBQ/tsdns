// Package metrics provides a Prometheus-backed implementation of system metrics reporting.
package metrics

import (
	"time"

	"github.com/honeybbq/tsdns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type promMetrics struct {
	queryTotal          *prometheus.CounterVec
	cacheRefreshTotal   *prometheus.CounterVec
	repositoryOpLatency *prometheus.HistogramVec
}

// NewPrometheusMetrics returns a Metrics implementation that reports to Prometheus.
func NewPrometheusMetrics() tsdns.Metrics {
	return &promMetrics{
		queryTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tsdns_queries_total",
			Help: "Total number of DNS queries received.",
		}, []string{"status"}),

		cacheRefreshTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tsdns_cache_refresh_total",
			Help: "Total number of cache refreshes.",
		}, []string{"status"}),

		repositoryOpLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "tsdns_repository_op_latency_seconds",
			Help:    "Latency of repository operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"op", "status"}),
	}
}

// RecordQuery records a query event with its result status.
func (p *promMetrics) RecordQuery(status string) {
	p.queryTotal.WithLabelValues(status).Inc()
}

// RecordCacheRefresh records a cache refresh event with its result status.
func (p *promMetrics) RecordCacheRefresh(status string) {
	p.cacheRefreshTotal.WithLabelValues(status).Inc()
}

// RecordRepositoryOp records the duration and status of a repository operation.
func (p *promMetrics) RecordRepositoryOp(op string, status string, duration time.Duration) {
	p.repositoryOpLatency.WithLabelValues(op, status).Observe(duration.Seconds())
}
