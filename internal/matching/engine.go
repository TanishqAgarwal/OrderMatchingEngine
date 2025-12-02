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


type PriceLevel []*models.Order

type OrderBook struct {
	Symbol string
	Bids   *redblacktree.Tree // Price (int64) -> PriceLevel ([]*Order)
	Asks   *redblacktree.Tree // Price (int64) -> PriceLevel ([]*Order)
	Orders map[string]*models.Order
	mu     sync.RWMutex
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		// Bids are sorted in descending order (highest price first)
		Bids: redblacktree.NewWith(func(a, b interface{}) int {
			return utils.Int64Comparator(b, a)
		}),
		// Asks are sorted in ascending order (lowest price first)
		Asks:   redblacktree.NewWith(utils.Int64Comparator),
		Orders: make(map[string]*models.Order),
	}
}

func (ob *OrderBook) AddOrder(order *models.Order) {
	if _, exists := ob.Orders[order.ID]; exists {
		return
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
			// Remove the order
			priceLevel = append(priceLevel[:i], priceLevel[i+1:]...)
			break
		}
	}

	if len(priceLevel) == 0 {
		tree.Remove(price)
	} else {
		tree.Put(price, priceLevel)
	}

	return order
}

// Locking the order book
func (ob *OrderBook) Lock() {
	ob.mu.Lock()
}

func (ob *OrderBook) Unlock() {
	ob.mu.Unlock()
}

// Locking the order book for reading.
func (ob *OrderBook) RLock() {
	ob.mu.RLock()
}

func (ob *OrderBook) RUnlock() {
	ob.mu.RUnlock()
}

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

// returns the aggregated depth of the order book.
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


type MatchResult struct {
	Order  *models.Order
	Trades []*models.Trade
}


type Engine struct {
	OrderBooks map[string]*OrderBook
	AllOrders  sync.Map // Map[string]*models.Order - Stores all orders for quick lookup
	mu         sync.RWMutex
	metrics    *metrics.Metrics
}

func NewEngine(m *metrics.Metrics) *Engine {
	return &Engine{
		OrderBooks: make(map[string]*OrderBook),
		metrics:    m,
	}
}

func (e *Engine) getOrderBook(symbol string) *OrderBook {
	e.mu.RLock()
	ob, exists := e.OrderBooks[symbol]
	e.mu.RUnlock()

	if !exists {
		e.mu.Lock()
		ob, exists = e.OrderBooks[symbol]
		if !exists {
			ob = NewOrderBook(symbol)
			e.OrderBooks[symbol] = ob
		}
		e.mu.Unlock()
	}
	return ob
}

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

	e.AllOrders.Store(order.ID, order)

	ob := e.getOrderBook(order.Symbol)
	ob.Lock()
	defer ob.Unlock()

	// check liquidity for Market Orders
	if order.Type == models.Market {
		available := ob.CalculateLiquidity(order.Side, order.OriginalQuantity)
		if available < order.OriginalQuantity {
			// reject the order
			e.AllOrders.Delete(order.ID)
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
		e.metrics.IncOrdersMatched(tradeCount + 1)
	}

	if order.FilledQuantity > 0 {
		if order.RemainingQuantity == 0 {
			order.Status = models.Filled
		} else {
			order.Status = models.PartialFill
		}
	} else {
		order.Status = models.Accepted
	}

	if order.RemainingQuantity > 0 {
		if order.Type == models.Market {
			// theoretically unreachable if liquidity check passed and we hold the lock
			// but for safety: maybe log a warning?
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

func (e *Engine) CancelOrder(orderID string) (*models.Order, error) {
	val, ok := e.AllOrders.Load(orderID)
	if !ok {
		return nil, fmt.Errorf("order not found")
	}
	order := val.(*models.Order)

	if order.Status == models.Filled {
		return nil, fmt.Errorf("cannot cancel: order already filled")
	}
	if order.Status == models.Cancelled {
		return order, nil
	}

	ob := e.getOrderBook(order.Symbol)
	ob.Lock()
	defer ob.Unlock()

	// Double check status under lock to prevent race
	if order.Status == models.Filled {
		return nil, fmt.Errorf("cannot cancel: order already filled")
	}

	removedOrder := ob.RemoveOrder(orderID)
	if removedOrder != nil {
		removedOrder.Status = models.Cancelled
		e.metrics.IncOrdersCancelled()
		e.metrics.DecOrdersInBook()
		return removedOrder, nil
	} else {
		order.Status = models.Cancelled
		e.metrics.IncOrdersCancelled()
		return order, nil
	}
}

func (e *Engine) GetOrder(orderID string) (*models.Order, error) {
	val, ok := e.AllOrders.Load(orderID)
	if !ok {
		return nil, fmt.Errorf("order not found")
	}
	return val.(*models.Order), nil
}

func (e *Engine) GetOrderBookDepth(symbol string, depthLimit int) (*OrderBookDepth, error) {
	ob := e.getOrderBook(symbol)
	return ob.GetDepth(depthLimit), nil
}
