package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"repello/internal/matching"
	"repello/internal/metrics"
	"repello/internal/models"
	"strconv"
	"time"

	"github.com/google/uuid"
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

// OrderBookLevelResponse is reused from engine struct but we define it here for clarity if needed,
// but the engine returns OrderBookDepth which has nested structs.
// We can just serialize the engine response directly if it matches.
// The engine returns matching.OrderBookDepth which has Bids/Asks as []matching.PriceLevelData
// matching.PriceLevelData has Price and Quantity fields with json tags "price" and "quantity".
// So it matches the spec: {"price": 10045, "quantity": 500}

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
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/orders", s.handleCreateOrder)
	mux.HandleFunc("DELETE /api/v1/orders/{id}", s.handleCancelOrder)
	mux.HandleFunc("GET /api/v1/orderbook/{symbol}", s.handleGetOrderBook)
	mux.HandleFunc("GET /api/v1/orders/{id}", s.handleGetOrder)
	mux.HandleFunc("GET /health", s.handleHealthCheck)
	mux.HandleFunc("GET /metrics", s.handleGetMetrics)

	return http.ListenAndServe(s.listenAddr, mux)
}

func (s *APIServer) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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
		// Check for specific error messages
		if err.Error() == "insufficient liquidity" {
			// Spec requires specific message: "Insufficient liquidity: only X shares available, requested Y"
			// But since we didn't fill it, filled is 0? Or partial fill?
			// The engine returns error only if filled == 0 for Market orders.
			msg := fmt.Sprintf("Insufficient liquidity: only %d shares available, requested %d", order.FilledQuantity, order.OriginalQuantity)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Build the detailed response
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
		writeJSON(w, http.StatusCreated, response)
	case models.PartialFill:
		response.FilledQuantity = order.FilledQuantity
		response.RemainingQuantity = order.RemainingQuantity
		writeJSON(w, http.StatusAccepted, response)
	case models.Filled:
		response.FilledQuantity = order.FilledQuantity
		writeJSON(w, http.StatusOK, response)
	case models.Cancelled:
		// Should not happen on create unless immediate cancel (IOC) logic exists, which we don't have yet.
		// Or if engine cancelled it.
		writeJSON(w, http.StatusOK, response) 
	}
}

func (s *APIServer) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")
	
	// No symbol required anymore
	order, err := s.engine.CancelOrder(orderID)
	if err != nil {
		if err.Error() == "cannot cancel: order already filled" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		} else if err.Error() == "order not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	response := CancelOrderResponse{
		OrderID: order.ID,
		Status:  order.Status.String(),
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *APIServer) handleGetOrderBook(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")

	// Handle 'depth' query parameter
	depthParam := r.URL.Query().Get("depth")
	depthVal := 0
	if depthParam != "" {
		var err error
		depthVal, err = strconv.Atoi(depthParam)
		if err != nil {
			depthVal = 0 // Default to full (or engine handles it)
		}
	}

	depth, err := s.engine.GetOrderBookDepth(symbol, depthVal)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// The engine returns matching.OrderBookDepth which matches the JSON structure required.
	writeJSON(w, http.StatusOK, depth)
}

func (s *APIServer) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")
	
	order, err := s.engine.GetOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found"})
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

	writeJSON(w, http.StatusOK, response)
}

func (s *APIServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// We need metrics for orders processed. 
	// Ideally we get this from s.metrics.
	// We didn't expose a GetOrdersProcessed in metrics, but we can access the atomic value or use JSON.
	// Using the json output of metrics is one way, or adding a method.
	// For simplicity, I'll just use a placeholder or read the public field if I made it public (I did).
	// Metrics fields are exported (e.g. OrdersReceived).
	
	uptime := int64(time.Since(s.startTime).Seconds())
	processed := s.metrics.OrdersReceived.Load() // Spec says "processed", likely received or matched? "150000" in example. "Received" fits.

	resp := HealthResponse{
		Status:          "healthy",
		UptimeSeconds:   uptime,
		OrdersProcessed: processed,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *APIServer) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
