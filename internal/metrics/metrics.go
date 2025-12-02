package metrics

import (
	"encoding/json"
	"math"
	"sync/atomic"
	"time"
)

const (
	MaxLatencyMicros = 100000 // Track up to 100ms with 1us precision
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
	
	// Histogram for accurate percentiles (Lock-free)
	// Index i stores count of requests taking i microseconds.
	// Last index stores all requests >= MaxLatencyMicros
	LatencyHistogram [MaxLatencyMicros + 1]atomic.Int64
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

// AddLatency adds to the total latency and updates the histogram.
func (m *Metrics) AddLatency(microseconds int64) {
	m.TotalLatency.Add(microseconds)
	
	// Update Histogram
	idx := microseconds
	if idx > MaxLatencyMicros {
		idx = MaxLatencyMicros
	}
	m.LatencyHistogram[idx].Add(1)
}

// calculatePercentile returns the latency value (in ms) below which the given percentile falls.
func (m *Metrics) calculatePercentile(p float64, totalCount int64) float64 {
	if totalCount == 0 {
		return 0
	}
	targetCount := int64(math.Ceil(float64(totalCount) * p))
	var currentCount int64 = 0
	
	for i := 0; i <= MaxLatencyMicros; i++ {
		count := m.LatencyHistogram[i].Load()
		currentCount += count
		if currentCount >= targetCount {
			// Convert micros to millis
			return float64(i) / 1000.0
		}
	}
	return float64(MaxLatencyMicros) / 1000.0
}

// MarshalJSON implements the json.Marshaler interface for Metrics.
func (m *Metrics) MarshalJSON() ([]byte, error) {
	totalOrders := m.OrdersReceived.Load()
	
	avgLatency := float64(0)
	if totalOrders > 0 {
		avgLatency = float64(m.TotalLatency.Load()) / float64(totalOrders) / 1000.0 // to ms
	}

	uptimeSeconds := time.Since(m.StartTime).Seconds()
	throughput := float64(0)
	if uptimeSeconds > 0 {
		throughput = float64(totalOrders) / uptimeSeconds
	}

	// Calculate accurate percentiles
	p50 := m.calculatePercentile(0.50, totalOrders)
	p99 := m.calculatePercentile(0.99, totalOrders)
	p999 := m.calculatePercentile(0.999, totalOrders)

	return json.Marshal(map[string]interface{}{
		"orders_received":           totalOrders,
		"orders_matched":            m.OrdersMatched.Load(),
		"orders_cancelled":          m.OrdersCancelled.Load(),
		"orders_in_book":            m.OrdersInBook.Load(),
		"trades_executed":           m.TradesExecuted.Load(),
		"latency_avg_ms":            avgLatency,
		"latency_p50_ms":            p50,
		"latency_p99_ms":            p99,
		"latency_p999_ms":           p999,
		"throughput_orders_per_sec": throughput,
	})
}
