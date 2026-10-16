package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// storageCollector samples dependencies on scrape, with one shared deadline.
// Failed database reads omit backlog samples instead of reporting a false zero.
type storageCollector struct {
	data        *Data
	sql         *sql.DB
	descriptors map[string]*prometheus.Desc
}

func newStorageCollector(data *Data, database *sql.DB) *storageCollector {
	descriptions := map[string]string{
		"mall_database_up":                 "Whether the bounded backlog query succeeded.",
		"mall_redis_up":                    "Whether Redis PING succeeded.",
		"mall_database_connections_in_use": "Connections currently in use.",
		"mall_database_connections_idle":   "Idle database connections.",
		"mall_database_connections_max":    "Maximum open database connections.",
		"mall_database_wait_total":         "Total waits for a database connection.",
		"mall_database_wait_seconds_total": "Cumulative time waiting for a database connection.",
		"mall_overdue_orders":              "Unpaid orders past their expiry deadline.",
		"mall_oldest_overdue_seconds":      "Age past deadline of the oldest unpaid order.",
		"mall_processing_payments":         "Payments awaiting reconciliation.",
		"mall_pending_refunds":             "Refunds awaiting reconciliation.",
	}
	collector := &storageCollector{data: data, sql: database, descriptors: make(map[string]*prometheus.Desc)}
	for name, help := range descriptions {
		collector.descriptors[name] = prometheus.NewDesc(name, help, nil, nil)
	}
	return collector
}

func (c *storageCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, descriptor := range c.descriptors {
		ch <- descriptor
	}
}

func (c *storageCollector) Collect(ch chan<- prometheus.Metric) {
	emit := func(name string, kind prometheus.ValueType, value float64) {
		ch <- prometheus.MustNewConstMetric(c.descriptors[name], kind, value)
	}
	stats := c.sql.Stats()
	emit("mall_database_connections_in_use", prometheus.GaugeValue, float64(stats.InUse))
	emit("mall_database_connections_idle", prometheus.GaugeValue, float64(stats.Idle))
	emit("mall_database_connections_max", prometheus.GaugeValue, float64(stats.MaxOpenConnections))
	emit("mall_database_wait_total", prometheus.CounterValue, float64(stats.WaitCount))
	emit("mall_database_wait_seconds_total", prometheus.CounterValue, stats.WaitDuration.Seconds())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	redisUp := 0.0
	if c.data.redis.Ping(ctx).Err() == nil {
		redisUp = 1
	}
	emit("mall_redis_up", prometheus.GaugeValue, redisUp)
	var snapshot struct {
		Overdue  float64
		Oldest   float64
		Payments float64
		Refunds  float64
	}
	// Only unfinished orders are interesting. Do not group by order or buyer IDs.
	err := c.data.db.WithContext(ctx).Raw(`
 SELECT
  count(*) FILTER (WHERE state='pending' AND expires_at < now()) AS overdue,
  COALESCE(max(EXTRACT(EPOCH FROM now()-expires_at))
   FILTER (WHERE state='pending' AND expires_at < now()), 0) AS oldest,
  count(*) FILTER (WHERE payment_state='processing') AS payments,
  count(*) FILTER (WHERE payment_state='refund_pending') AS refunds
 FROM orders
 WHERE state='pending' OR payment_state IN ('processing', 'refund_pending')
 `).Scan(&snapshot).Error
	databaseUp := 0.0
	if err == nil {
		databaseUp = 1
		emit("mall_overdue_orders", prometheus.GaugeValue, snapshot.Overdue)
		emit("mall_oldest_overdue_seconds", prometheus.GaugeValue, snapshot.Oldest)
		emit("mall_processing_payments", prometheus.GaugeValue, snapshot.Payments)
		emit("mall_pending_refunds", prometheus.GaugeValue, snapshot.Refunds)
	}
	emit("mall_database_up", prometheus.GaugeValue, databaseUp)
}
