package api

import (
	"encoding/json"
	"repello/internal/matching"
	"repello/internal/metrics"
	"repello/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// --- Request/Response Structs ---

type CreateOrderRequest struct {
	Symbol   string           `json:"symbol"`
	Side     models.Side      `json:"side"`
	Type     models.OrderType `json:"type"`
	Price    int64            `json:"price,omitempty"` // Required for LIMIT, omit for MARKET
	Quantity int64            `json:"quantity"`
}

type TradeResponse struct {
	TradeID   string `json:"trade_id"`
	Price     int64  `json:"price"`
	Quantity  int64  `json:"quantity"`
	Timestamp int64  `json:"timestamp"`
}

type CreateOrderResponse struct {
	OrderID           string          `json:"order_id"`
	Status            string          `json:"status"`
	Message           string          `json:"message,omitempty"`
	FilledQuantity    int64           `json:"filled_quantity,omitempty"`
	RemainingQuantity int64           `json:"remaining_quantity,omitempty"`
	Trades            []TradeResponse `json:"trades,omitempty"`
}

type CancelOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type GetOrderResponse struct {
	OrderID        string           `json:"order_id"`
	Symbol         string           `json:"symbol"`
	Side           models.Side      `json:"side"`
	Type           models.OrderType `json:"type"`
	Price          int64            `json:"price"`
	Quantity       int64            `json:"quantity"`
	FilledQuantity int64            `json:"filled_quantity"`
	Status         string           `json:"status"`
	Timestamp      int64            `json:"timestamp"`
}

type HealthResponse struct {
	Status          string `json:"status"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	OrdersProcessed int64  `json:"orders_processed"`
}

// APIServer is the HTTP server for the matching engine.
type APIServer struct {
	listenAddr string
	engine     *matching.Engine
	metrics    *metrics.Metrics
	startTime  time.Time
}

// NewAPIServer creates a new APIServer.
func NewAPIServer(listenAddr string, engine *matching.Engine, metrics *metrics.Metrics) *APIServer {
	return &APIServer{
		listenAddr: listenAddr,
		engine:     engine,
		metrics:    metrics,
		startTime:  time.Now(),
	}
}

// Run starts the HTTP server.
func (s *APIServer) Run() error {
	// fasthttp RequestHandler
	handler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		method := string(ctx.Method())

		switch path {
		case "/api/v1/orders":
			if method == "POST" {
				s.handleCreateOrder(ctx)
			} else {
				ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
			}
		case "/health":
			if method == "GET" {
				s.handleHealthCheck(ctx)
			} else {
				ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
			}
		case "/metrics":
			if method == "GET" {
				s.handleGetMetrics(ctx)
			} else {
				ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
			}
		default:
			// Handle paths with parameters (e.g., /api/v1/orders/{id})
			if strings.HasPrefix(path, "/api/v1/orders/") {
				if method == "DELETE" {
					// Extract ID: /api/v1/orders/{id}
					id := strings.TrimPrefix(path, "/api/v1/orders/")
					s.handleCancelOrder(ctx, id)
				} else if method == "GET" {
					id := strings.TrimPrefix(path, "/api/v1/orders/")
					s.handleGetOrder(ctx, id)
				} else {
					ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
				}
				return
			}
			if strings.HasPrefix(path, "/api/v1/orderbook/") {
				if method == "GET" {
					symbol := strings.TrimPrefix(path, "/api/v1/orderbook/")
					s.handleGetOrderBook(ctx, symbol)
				} else {
					ctx.Error("Method not allowed", fasthttp.StatusMethodNotAllowed)
				}
				return
			}
			ctx.Error("Not Found", fasthttp.StatusNotFound)
		}
	}

	return fasthttp.ListenAndServe(s.listenAddr, handler)
}

func (s *APIServer) handleCreateOrder(ctx *fasthttp.RequestCtx) {
	var req CreateOrderRequest
	// fasthttp provides body via ctx.PostBody()
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	order := models.NewOrder(
		uuid.New().String(),
		req.Symbol,
		req.Side,
		req.Type,
		req.Price,
		req.Quantity,
	)

	result, err := s.engine.ProcessOrder(order)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient liquidity") {
			writeJSON(ctx, fasthttp.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	response := CreateOrderResponse{
		OrderID: order.ID,
		Status:  order.Status.String(),
	}

	if result != nil && len(result.Trades) > 0 {
		response.Trades = make([]TradeResponse, len(result.Trades))
		for i, trade := range result.Trades {
			response.Trades[i] = TradeResponse{
				TradeID:   trade.ID,
				Price:     trade.Price,
				Quantity:  trade.Quantity,
				Timestamp: trade.Timestamp,
			}
		}
	}

	switch order.Status {
	case models.Accepted:
		response.Message = "Order added to book"
		writeJSON(ctx, fasthttp.StatusCreated, response)
	case models.PartialFill:
		response.FilledQuantity = order.FilledQuantity
		response.RemainingQuantity = order.RemainingQuantity
		writeJSON(ctx, fasthttp.StatusAccepted, response)
	case models.Filled:
		response.FilledQuantity = order.FilledQuantity
		writeJSON(ctx, fasthttp.StatusOK, response)
	case models.Cancelled:
		writeJSON(ctx, fasthttp.StatusOK, response)
	}
}

func (s *APIServer) handleCancelOrder(ctx *fasthttp.RequestCtx, orderID string) {
	order, err := s.engine.CancelOrder(orderID)
	if err != nil {
		if err.Error() == "cannot cancel: order already filled" {
			writeJSON(ctx, fasthttp.StatusBadRequest, map[string]string{"error": err.Error()})
		} else if err.Error() == "order not found" {
			writeJSON(ctx, fasthttp.StatusNotFound, map[string]string{"error": "Order not found"})
		} else {
			writeJSON(ctx, fasthttp.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	response := CancelOrderResponse{
		OrderID: order.ID,
		Status:  order.Status.String(),
	}
	writeJSON(ctx, fasthttp.StatusOK, response)
}

func (s *APIServer) handleGetOrderBook(ctx *fasthttp.RequestCtx, symbol string) {
	depthParam := string(ctx.QueryArgs().Peek("depth"))
	depthVal := 0
	if depthParam != "" {
		var err error
		depthVal, err = strconv.Atoi(depthParam)
		if err != nil {
			depthVal = 0
		}
	}

	depth, err := s.engine.GetOrderBookDepth(symbol, depthVal)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(ctx, fasthttp.StatusOK, depth)
}

func (s *APIServer) handleGetOrder(ctx *fasthttp.RequestCtx, orderID string) {
	order, err := s.engine.GetOrder(orderID)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusNotFound, map[string]string{"error": "Order not found"})
		return
	}

	response := GetOrderResponse{
		OrderID:        order.ID,
		Symbol:         order.Symbol,
		Side:           order.Side,
		Type:           order.Type,
		Price:          order.Price,
		Quantity:       order.OriginalQuantity,
		FilledQuantity: order.FilledQuantity,
		Status:         order.Status.String(),
		Timestamp:      order.Timestamp,
	}

	writeJSON(ctx, fasthttp.StatusOK, response)
}

func (s *APIServer) handleHealthCheck(ctx *fasthttp.RequestCtx) {
	uptime := int64(time.Since(s.startTime).Seconds())
	processed := s.metrics.OrdersReceived.Load()

	resp := HealthResponse{
		Status:          "healthy",
		UptimeSeconds:   uptime,
		OrdersProcessed: processed,
	}
	writeJSON(ctx, fasthttp.StatusOK, resp)
}

func (s *APIServer) handleGetMetrics(ctx *fasthttp.RequestCtx) {
	writeJSON(ctx, fasthttp.StatusOK, s.metrics)
}

func writeJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	ctx.Response.Header.SetContentType("application/json")
	ctx.SetStatusCode(status)
	if err := json.NewEncoder(ctx).Encode(v); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
	}
}
