package matching

import (
	"fmt"
	"repello/internal/metrics"
	"repello/internal/models"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessOrder_SimpleMatch(t *testing.T) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)

	sellOrder := models.NewOrder("seller1", "BTCUSD", models.Sell, models.Limit, 100, 10)
	engine.ProcessOrder(sellOrder)

	buyOrder := models.NewOrder("buyer1", "BTCUSD", models.Buy, models.Limit, 100, 10)
	result, err := engine.ProcessOrder(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
	assert.Equal(t, int64(10), result.Trades[0].Quantity)
	assert.Equal(t, int64(100), result.Trades[0].Price)
	assert.Equal(t, int64(0), buyOrder.RemainingQuantity)

	// Check if the book is empty
	ob := engine.getOrderBook("BTCUSD")
	assert.True(t, ob.Bids.Empty())
	assert.True(t, ob.Asks.Empty())
}

func TestProcessOrder_PartialFill(t *testing.T) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)

	sellOrder := models.NewOrder("seller1", "BTCUSD", models.Sell, models.Limit, 100, 5)
	engine.ProcessOrder(sellOrder)

	buyOrder := models.NewOrder("buyer1", "BTCUSD", models.Buy, models.Limit, 100, 10)
	result, err := engine.ProcessOrder(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
	assert.Equal(t, int64(5), result.Trades[0].Quantity)
	assert.Equal(t, int64(5), buyOrder.RemainingQuantity) // Remaining quantity

	// Check if the buy order is now in the book
	ob := engine.getOrderBook("BTCUSD")
	assert.False(t, ob.Bids.Empty())
	assert.True(t, ob.Asks.Empty())
	bestBid := ob.GetBestBid()
	assert.Equal(t, "buyer1", bestBid.ID)
}

func TestProcessOrder_MultiLevelMatch(t *testing.T) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)

	sellOrder1 := models.NewOrder("seller1", "BTCUSD", models.Sell, models.Limit, 100, 5)
	sellOrder2 := models.NewOrder("seller2", "BTCUSD", models.Sell, models.Limit, 101, 5)
	engine.ProcessOrder(sellOrder1)
	engine.ProcessOrder(sellOrder2)

	buyOrder := models.NewOrder("buyer1", "BTCUSD", models.Buy, models.Limit, 101, 8)
	result, err := engine.ProcessOrder(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Trades))
	assert.Equal(t, int64(0), buyOrder.RemainingQuantity)

	// Check trade details
	// Trade 1 with seller1 at 100
	assert.Equal(t, int64(5), result.Trades[0].Quantity)
	assert.Equal(t, int64(100), result.Trades[0].Price)
	// Trade 2 with seller2 at 101
	assert.Equal(t, int64(3), result.Trades[1].Quantity)
	assert.Equal(t, int64(101), result.Trades[1].Price)

	// Check remaining order in the book
	ob := engine.getOrderBook("BTCUSD")
	bestAsk := ob.GetBestAsk()
	assert.Equal(t, "seller2", bestAsk.ID)
	assert.Equal(t, int64(2), bestAsk.RemainingQuantity)
}

func TestProcessOrder_MarketOrderRejection(t *testing.T) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)

	// Sell order: 5 shares @ 100
	sellOrder := models.NewOrder("seller1", "BTCUSD", models.Sell, models.Limit, 100, 5)
	engine.ProcessOrder(sellOrder)

	// Market Buy order: 10 shares
	buyOrder := models.NewOrder("buyer1", "BTCUSD", models.Buy, models.Market, 0, 10)
	result, err := engine.ProcessOrder(buyOrder)

	// EXPECTATION: Should FAIL with "insufficient liquidity"
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient liquidity")
	assert.Contains(t, err.Error(), "only 5 shares available")
	assert.Nil(t, result)
	
	// Book should still contain the sell order
	ob := engine.getOrderBook("BTCUSD")
	assert.False(t, ob.Asks.Empty()) // The sell order should remain untouched
	
	bestAsk := ob.GetBestAsk()
	assert.Equal(t, int64(5), bestAsk.RemainingQuantity) // Nothing matched
}

func TestEngineConcurrency(t *testing.T) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)
	numGoroutines := 100
	ordersPerGoroutine := 100
	symbol := "BTCUSD"

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ordersPerGoroutine; j++ {
				side := models.Buy
				if (id+j)%2 == 0 {
					side = models.Sell
				}
				order := models.NewOrder(
					fmt.Sprintf("order-%d-%d", id, j),
					symbol,
					side,
					models.Limit,
					100, // Same price to create contention
					1,
				)
				_, err := engine.ProcessOrder(order)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

// BenchmarkPlaceOrder measures the throughput of placing orders into a pre-filled book.
// Helps verify that the engine meets the high-performance requirement (e.g., 30k+ TPS).
func BenchmarkPlaceOrder(b *testing.B) {
	m := metrics.NewMetrics()
	engine := NewEngine(m)
	symbol := "BTCUSD"

	// Pre-fill the book
	for i := 0; i < 1000; i++ {
		sellOrder := models.NewOrder(fmt.Sprintf("sell-%d", i), symbol, models.Sell, models.Limit, int64(1000+i), 1)
		engine.ProcessOrder(sellOrder)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		order := models.NewOrder(
			fmt.Sprintf("bench-%d", i),
			symbol,
			models.Buy,
			models.Limit,
			1000,
			1,
		)
		engine.ProcessOrder(order)
	}
}
