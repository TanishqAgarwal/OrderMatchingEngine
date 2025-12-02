package matching

import (
	"fmt"
	"repello/internal/metrics"
	"repello/internal/models"
	"sync"
	"time"

	"github.com/emirpasic/gods/trees/redblacktree"
	"github.com/emirpasic/gods/utils"
	"github.com/google/uuid"
)

// OrderBookDepth represents the aggregated depth of the order book.
type OrderBookDepth struct {
	Symbol    string           `json:"symbol"`
	Timestamp int64            `json:"timestamp"`
	Bids      []PriceLevelData `json:"bids"`
	Asks      []PriceLevelData `json:"asks"`
}

type PriceLevelData struct {
	Price    int64 `json:"price"`
	Quantity int64 `json:"quantity"`
}

// PriceLevel represents a collection of orders at a specific price.
type PriceLevel []*models.Order

// OrderBook represents the order book for a single financial instrument.
type OrderBook struct {
	Symbol string
	Bids   *redblacktree.Tree // Price (int64) -> PriceLevel ([]*Order)
	Asks   *redblacktree.Tree // Price (int64) -> PriceLevel ([]*Order)
	Orders map[string]*models.Order
	mu     sync.RWMutex
}

// NewOrderBook creates and returns a new OrderBook.
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		// Bids are sorted in descending order (highest price first)
		Bids: redblacktree.NewWith(func(a, b interface{}) int {
			return utils.Int64Comparator(b, a) // Reverse comparison
		}),
		// Asks are sorted in ascending order (lowest price first)
		Asks:   redblacktree.NewWith(utils.Int64Comparator),
		Orders: make(map[string]*models.Order),
	}
}

// AddOrder adds an order to the order book.
func (ob *OrderBook) AddOrder(order *models.Order) {
	if _, exists := ob.Orders[order.ID]; exists {
		return // Order already exists
	}
	ob.Orders[order.ID] = order

	var tree *redblacktree.Tree
	if order.Side == models.Buy {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	price := order.Price
	level, found := tree.Get(price)

	if !found {
		newLevel := make(PriceLevel, 0, 1)
		newLevel = append(newLevel, order)
		tree.Put(price, newLevel)
	} else {
		existingLevel := level.(PriceLevel)
		existingLevel = append(existingLevel, order)
		tree.Put(price, existingLevel)
	}
}

// RemoveOrder removes an order from the order book by its ID.
func (ob *OrderBook) RemoveOrder(orderID string) *models.Order {
	order, exists := ob.Orders[orderID]
	if !exists {
		return nil
	}

	delete(ob.Orders, orderID)

	var tree *redblacktree.Tree
	if order.Side == models.Buy {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	price := order.Price
	level, found := tree.Get(price)
	if !found {
		return order // Should not happen in a consistent state
	}

	priceLevel := level.(PriceLevel)
	for i, o := range priceLevel {
		if o.ID == orderID {
			// Remove the order from the slice
			priceLevel = append(priceLevel[:i], priceLevel[i+1:]...)
			break
		}
	}

	if len(priceLevel) == 0 {
		// If the price level is empty, remove it from the tree
		tree.Remove(price)
	} else {
		// Otherwise, update the price level
		tree.Put(price, priceLevel)
	}

	return order
}

// Lock locks the order book for writing.
func (ob *OrderBook) Lock() {
	ob.mu.Lock()
}

// Unlock unlocks the order book for writing.
func (ob *OrderBook) Unlock() {
	ob.mu.Unlock()
}

// RLock locks the order book for reading.
func (ob *OrderBook) RLock() {
	ob.mu.RLock()
}

// RUnlock unlocks the order book for reading.
func (ob *OrderBook) RUnlock() {
	ob.mu.RUnlock()
}

// GetBestBid returns the best (highest) bid price and the corresponding price level.
func (ob *OrderBook) GetBestBid() *models.Order {
	if ob.Bids.Empty() {
		return nil
	}
	node := ob.Bids.Left() // For descending bids, left is the max
	if node == nil {
		return nil
	}
	priceLevel := node.Value.(PriceLevel)
	if len(priceLevel) == 0 {
		return nil
	}
	return priceLevel[0]
}

// GetBestAsk returns the best (lowest) ask price and the corresponding price level.
func (ob *OrderBook) GetBestAsk() *models.Order {
	if ob.Asks.Empty() {
		return nil
	}
	node := ob.Asks.Left() // For ascending asks, left is the min
	if node == nil {
		return nil
	}
	priceLevel := node.Value.(PriceLevel)
	if len(priceLevel) == 0 {
		return nil
	}
	return priceLevel[0]
}

// CalculateLiquidity calculates the available liquidity for a given side up to maxNeeded.
// Note: This method must be called while holding a lock on the order book if consistency is required,
// but since it iterates the tree, it should ideally use RLock.
// However, if called from ProcessOrder which holds Lock, we cannot RLock.
// So this method assumes the caller holds the lock.
func (ob *OrderBook) CalculateLiquidity(side models.Side, maxNeeded int64) int64 {
	var tree *redblacktree.Tree
	// If incoming order is Buy, it consumes Asks.
	// If incoming order is Sell, it consumes Bids.
	if side == models.Buy {
		tree = ob.Asks
	} else {
		tree = ob.Bids
	}

	if tree.Empty() {
		return 0
	}

	it := tree.Iterator()
	it.Begin()
	var available int64 = 0
	for it.Next() {
		priceLevel := it.Value().(PriceLevel)
		for _, order := range priceLevel {
			available += order.RemainingQuantity
			if available >= maxNeeded {
				return available
			}
		}
	}
	return available
}

// GetDepth returns the aggregated depth of the order book.
func (ob *OrderBook) GetDepth(depthLimit int) *OrderBookDepth {
	ob.RLock()
	defer ob.RUnlock()

	depth := &OrderBookDepth{
		Symbol:    ob.Symbol,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond), // ms timestamp
		Bids:      make([]PriceLevelData, 0),
		Asks:      make([]PriceLevelData, 0),
	}

	// Bids
	itBids := ob.Bids.Iterator()
	itBids.Begin()
	count := 0
	for itBids.Next() {
		if depthLimit > 0 && count >= depthLimit {
			break
		}
		price := itBids.Key().(int64)
		priceLevel := itBids.Value().(PriceLevel)
		var totalQuantity int64
		for _, order := range priceLevel {
			totalQuantity += order.RemainingQuantity
		}
		depth.Bids = append(depth.Bids, PriceLevelData{Price: price, Quantity: totalQuantity})
		count++
	}

	// Asks
	itAsks := ob.Asks.Iterator()
	itAsks.Begin()
	count = 0
	for itAsks.Next() {
		if depthLimit > 0 && count >= depthLimit {
			break
		}
		price := itAsks.Key().(int64)
		priceLevel := itAsks.Value().(PriceLevel)
		var totalQuantity int64
		for _, order := range priceLevel {
			totalQuantity += order.RemainingQuantity
		}
		depth.Asks = append(depth.Asks, PriceLevelData{Price: price, Quantity: totalQuantity})
		count++
	}

	return depth
}

// MatchResult contains the result of processing an order.
type MatchResult struct {
	Order  *models.Order
	Trades []*models.Trade
}

// Engine is the core of the matching engine.
type Engine struct {
	OrderBooks map[string]*OrderBook
	AllOrders  sync.Map // Map[string]*models.Order - Stores all orders for quick lookup
	mu         sync.RWMutex
	metrics    *metrics.Metrics
}

// NewEngine creates and returns a new Engine.
func NewEngine(m *metrics.Metrics) *Engine {
	return &Engine{
		OrderBooks: make(map[string]*OrderBook),
		metrics:    m,
	}
}

// getOrderBook returns the order book for a given symbol, creating it if it doesn't exist.
func (e *Engine) getOrderBook(symbol string) *OrderBook {
	e.mu.RLock()
	ob, exists := e.OrderBooks[symbol]
	e.mu.RUnlock()

	if !exists {
		e.mu.Lock()
		// Double check after acquiring write lock
		ob, exists = e.OrderBooks[symbol]
		if !exists {
			ob = NewOrderBook(symbol)
			e.OrderBooks[symbol] = ob
		}
		e.mu.Unlock()
	}
	return ob
}

// ProcessOrder processes an order and returns the match result.
func (e *Engine) ProcessOrder(order *models.Order) (*MatchResult, error) {
	startTime := time.Now()
	defer func() {
		latency := time.Since(startTime).Microseconds()
		e.metrics.AddLatency(latency)
	}()

	e.metrics.IncOrdersReceived()

	if err := order.Validate(); err != nil {
		return nil, err
	}

	// Store the order in the global map
	e.AllOrders.Store(order.ID, order)

	ob := e.getOrderBook(order.Symbol)
	ob.Lock()
	defer ob.Unlock()

	// Check liquidity for Market Orders
	if order.Type == models.Market {
		available := ob.CalculateLiquidity(order.Side, order.OriginalQuantity)
		if available < order.OriginalQuantity {
			// Reject the order
			e.AllOrders.Delete(order.ID) // Remove from store as it's rejected
			return nil, fmt.Errorf("insufficient liquidity: only %d shares available, requested %d", available, order.OriginalQuantity)
		}
	}

	trades := make([]*models.Trade, 0)

	if order.Type == models.Limit {
		trades = e.processLimitOrder(order, ob)
	} else if order.Type == models.Market {
		trades = e.processMarketOrder(order, ob)
	}

	tradeCount := int64(len(trades))
	e.metrics.IncTradesExecuted(tradeCount)
	if tradeCount > 0 {
		// Each trade matches the incoming order and a book order
		e.metrics.IncOrdersMatched(tradeCount + 1)
	}

	// Update order status based on trades
	if order.FilledQuantity > 0 {
		if order.RemainingQuantity == 0 {
			order.Status = models.Filled
		} else {
			order.Status = models.PartialFill
		}
	} else {
		// No trades, status remains Accepted
		order.Status = models.Accepted
	}

	if order.RemainingQuantity > 0 {
		// Market orders with remaining quantity should strictly NOT be added to book.
		// However, due to the pre-check above, we should only reach here if we expected to fill it but raced?
		// No, we hold the lock. So if we passed the check, we MUST be able to fill it fully?
		// Wait. CalculateLiquidity sums up ALL liquidity.
		// processMarketOrder walks the book and matches.
		// Since we hold the lock, the liquidity shouldn't change between check and process.
		// So for Market orders, if we passed the check, RemainingQuantity MUST be 0 here.
		// Unless there's a bug in CalculateLiquidity or processMarketOrder.
		
		if order.Type == models.Market {
			// This path should theoretically be unreachable if liquidity check passed and we hold the lock.
			// But for safety:
			// Do NOT add to book.
			// Maybe log a warning?
		} else {
			ob.AddOrder(order)
			e.metrics.IncOrdersInBook()
		}
	} else {
		order.Status = models.Filled
	}

	return &MatchResult{
		Order:  order,
		Trades: trades,
	}, nil
}

// processLimitOrder processes a limit order.
func (e *Engine) processLimitOrder(order *models.Order, ob *OrderBook) []*models.Trade {
	trades := make([]*models.Trade, 0)
	if order.Side == models.Buy {
		for order.RemainingQuantity > 0 && !ob.Asks.Empty() {
			bestAsk := ob.GetBestAsk()
			if order.Price < bestAsk.Price {
				break
			}
			trade := e.executeTrade(order, bestAsk, ob)
			trades = append(trades, trade)
		}
	} else { // Sell side
		for order.RemainingQuantity > 0 && !ob.Bids.Empty() {
			bestBid := ob.GetBestBid()
			if order.Price > bestBid.Price {
				break
			}
			trade := e.executeTrade(order, bestBid, ob)
			trades = append(trades, trade)
		}
	}
	return trades
}

// processMarketOrder processes a market order.
func (e *Engine) processMarketOrder(order *models.Order, ob *OrderBook) []*models.Trade {
	trades := make([]*models.Trade, 0)
	if order.Side == models.Buy {
		for order.RemainingQuantity > 0 && !ob.Asks.Empty() {
			bestAsk := ob.GetBestAsk()
			trade := e.executeTrade(order, bestAsk, ob)
			trades = append(trades, trade)
		}
	} else { // Sell side
		for order.RemainingQuantity > 0 && !ob.Bids.Empty() {
			bestBid := ob.GetBestBid()
			trade := e.executeTrade(order, bestBid, ob)
			trades = append(trades, trade)
		}
	}
	return trades
}

// executeTrade creates a trade and updates the orders and order book.
func (e *Engine) executeTrade(incomingOrder, bookOrder *models.Order, ob *OrderBook) *models.Trade {
	tradeQuantity := incomingOrder.RemainingQuantity
	if bookOrder.RemainingQuantity < tradeQuantity {
		tradeQuantity = bookOrder.RemainingQuantity
	}

	tradePrice := bookOrder.Price

	trade := models.NewTrade(
		uuid.New().String(),
		getBuyerOrderID(incomingOrder, bookOrder),
		getSellerOrderID(incomingOrder, bookOrder),
		tradePrice,
		tradeQuantity,
	)

	// Update Incoming Order
	incomingOrder.RemainingQuantity -= tradeQuantity
	incomingOrder.FilledQuantity += tradeQuantity

	// Update Book Order
	bookOrder.RemainingQuantity -= tradeQuantity
	bookOrder.FilledQuantity += tradeQuantity

	if bookOrder.RemainingQuantity == 0 {
		bookOrder.Status = models.Filled
		ob.RemoveOrder(bookOrder.ID)
		e.metrics.DecOrdersInBook()
	} else {
		bookOrder.Status = models.PartialFill
	}

	return trade
}

func getBuyerOrderID(o1, o2 *models.Order) string {
	if o1.Side == models.Buy {
		return o1.ID
	}
	return o2.ID
}

func getSellerOrderID(o1, o2 *models.Order) string {
	if o1.Side == models.Sell {
		return o1.ID
	}
	return o2.ID
}

// CancelOrder cancels an order.
func (e *Engine) CancelOrder(orderID string) (*models.Order, error) {
	// Find order in global store
	val, ok := e.AllOrders.Load(orderID)
	if !ok {
		return nil, fmt.Errorf("order not found")
	}
	order := val.(*models.Order)

	// Check if already filled or cancelled
	if order.Status == models.Filled {
		return nil, fmt.Errorf("cannot cancel: order already filled")
	}
	if order.Status == models.Cancelled {
		return order, nil // Already cancelled
	}

	// Remove from OrderBook
	ob := e.getOrderBook(order.Symbol)
	ob.Lock()
	defer ob.Unlock()

	// Double check status under lock to prevent race
	if order.Status == models.Filled {
		return nil, fmt.Errorf("cannot cancel: order already filled")
	}

	removedOrder := ob.RemoveOrder(orderID)
	if removedOrder != nil {
		// It was in the book, so we update status
		removedOrder.Status = models.Cancelled
		e.metrics.IncOrdersCancelled()
		e.metrics.DecOrdersInBook()
		return removedOrder, nil
	} else {
		// It wasn't in the book, but we have it in store.
		// It might be a partially filled market order that wasn't added to book?
		// Or a race condition?
		// If it's not in book and not filled, it might be weird.
		// But for now, we just mark it cancelled.
		order.Status = models.Cancelled
		e.metrics.IncOrdersCancelled()
		return order, nil
	}
}

// GetOrder returns an order by its ID.
func (e *Engine) GetOrder(orderID string) (*models.Order, error) {
	val, ok := e.AllOrders.Load(orderID)
	if !ok {
		return nil, fmt.Errorf("order not found")
	}
	return val.(*models.Order), nil
}

// GetOrderBookDepth returns the depth for a given symbol.
func (e *Engine) GetOrderBookDepth(symbol string, depthLimit int) (*OrderBookDepth, error) {
	ob := e.getOrderBook(symbol)
	return ob.GetDepth(depthLimit), nil
}
