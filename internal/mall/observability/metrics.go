// Package observability owns a registry per application, avoiding global registration
// and keeping buyer IDs, order IDs and arbitrary URLs out of metric labels.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry            *prometheus.Registry
	Requests            *prometheus.CounterVec
	Duration            *prometheus.HistogramVec
	Maintenance         *prometheus.CounterVec
	MaintenanceDuration prometheus.Histogram
	MaintenanceSuccess  prometheus.Gauge
}

func New() *Metrics {
	m := &Metrics{
		Registry:            prometheus.NewRegistry(),
		Requests:            prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mall_http_requests_total", Help: "Completed requests including outer admission rejections."}, []string{"operation", "status"}),
		Duration:            prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "mall_http_duration_seconds", Help: "End-to-end HTTP duration including admission.", Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}}, []string{"operation", "status"}),
		Maintenance:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mall_maintenance_runs_total", Help: "Reconciliation batches by outcome."}, []string{"result"}),
		MaintenanceDuration: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "mall_maintenance_duration_seconds", Help: "Duration of a complete reconciliation batch."}),
		MaintenanceSuccess:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "mall_maintenance_last_success_timestamp_seconds", Help: "Unix timestamp of last successful reconciliation batch; zero before first success."}),
	}
	m.Registry.MustRegister(m.Requests, m.Duration, m.Maintenance, m.MaintenanceDuration, m.MaintenanceSuccess, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	for _, result := range []string{"success", "error"} {
		m.Maintenance.WithLabelValues(result)
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{MaxRequestsInFlight: 1})
}

// Filter surrounds the admission filter so a fast 429 cannot hide incoming load.
func (m *Metrics) Filter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		operation := "other"
		if r.Method == http.MethodPost && r.URL.Path == "/api/mall/v1/orders" {
			operation = "purchase"
		}
		started := time.Now()
		response := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			status := strconv.Itoa(response.status)
			m.Requests.WithLabelValues(operation, status).Inc()
			m.Duration.WithLabelValues(operation, status).Observe(time.Since(started).Seconds())
		}()
		next.ServeHTTP(response, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	if status >= 200 {
		w.status = status
		w.written = true
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
