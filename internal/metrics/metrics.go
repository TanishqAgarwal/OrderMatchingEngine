package metrics

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

// Metrics holds thread-safe counters for the application.
type Metrics struct {
	StartTime       time.Time
	OrdersReceived  atomic.Int64
	OrdersMatched   atomic.Int64
	OrdersCancelled atomic.Int64
	OrdersInBook    atomic.Int64
	TradesExecuted  atomic.Int64
	TotalLatency    atomic.Int64 // in microseconds
}

// NewMetrics creates a new Metrics struct.
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime: time.Now(),
	}
}

// IncOrdersReceived increments the total orders counter.
func (m *Metrics) IncOrdersReceived() {
	m.OrdersReceived.Add(1)
}

// IncOrdersMatched increments the total orders matched counter.
func (m *Metrics) IncOrdersMatched(count int64) {
	m.OrdersMatched.Add(count)
}

// IncOrdersCancelled increments the total orders cancelled counter.
func (m *Metrics) IncOrdersCancelled() {
	m.OrdersCancelled.Add(1)
}

// IncOrdersInBook increments the orders in book counter.
func (m *Metrics) IncOrdersInBook() {
	m.OrdersInBook.Add(1)
}

// DecOrdersInBook decrements the orders in book counter.
func (m *Metrics) DecOrdersInBook() {
	m.OrdersInBook.Add(-1)
}

// IncTradesExecuted increments the total trades counter.
func (m *Metrics) IncTradesExecuted(count int64) {
	m.TradesExecuted.Add(count)
}

// AddLatency adds to the total latency.
func (m *Metrics) AddLatency(microseconds int64) {
	m.TotalLatency.Add(microseconds)
}

// MarshalJSON implements the json.Marshaler interface for Metrics.
func (m *Metrics) MarshalJSON() ([]byte, error) {
	avgLatency := float64(0)
	totalOrders := m.OrdersReceived.Load()
	if totalOrders > 0 {
		avgLatency = float64(m.TotalLatency.Load()) / float64(totalOrders) / 1000.0 // to ms
	}

	uptimeSeconds := time.Since(m.StartTime).Seconds()
	throughput := float64(0)
	if uptimeSeconds > 0 {
		throughput = float64(totalOrders) / uptimeSeconds
	}

	return json.Marshal(map[string]interface{}{
		"orders_received":           totalOrders,
		"orders_matched":            m.OrdersMatched.Load(),
		"orders_cancelled":          m.OrdersCancelled.Load(),
		"orders_in_book":            m.OrdersInBook.Load(),
		"trades_executed":           m.TradesExecuted.Load(),
		"latency_p50_ms":            avgLatency, // Placeholder: using average as p50 for now
		"latency_p99_ms":            avgLatency, // Placeholder
		"latency_p999_ms":           avgLatency, // Placeholder
		"throughput_orders_per_sec": throughput,
	})
}
