package tsdns

import "time"

// Metrics defines the interface for core system metrics reporting.
// Core components use this to report status without depending on specific
// monitoring implementations like Prometheus.
type Metrics interface {
	// RecordQuery records a DNS query and its status (hit, miss, error).
	RecordQuery(status string)
	// RecordCacheRefresh records a cache refresh result (success, error).
	RecordCacheRefresh(status string)
	// RecordRepositoryOp records the duration and status of a repository operation.
	RecordRepositoryOp(op string, status string, duration time.Duration)
}

// nopMetrics is a no-op implementation of the Metrics interface.
type nopMetrics struct{}

func (nopMetrics) RecordQuery(string)                               {}
func (nopMetrics) RecordCacheRefresh(string)                        {}
func (nopMetrics) RecordRepositoryOp(string, string, time.Duration) {}

// NewNopMetrics returns a Metrics implementation that does nothing.
func NewNopMetrics() Metrics {
	return nopMetrics{}
}
