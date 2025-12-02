package models

import (
	"fmt"
	"time"
)

// OrderStatus represents the state of an order.
type OrderStatus int

const (
	Accepted OrderStatus = iota
	PartialFill
	Filled
	Cancelled
)

func (os OrderStatus) String() string {
	switch os {
	case Accepted:
		return "ACCEPTED"
	case PartialFill:
		return "PARTIAL_FILL"
	case Filled:
		return "FILLED"
	case Cancelled:
		return "CANCELLED"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON converts an OrderStatus to its string representation for JSON encoding.
func (os OrderStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + os.String() + `"`), nil
}

// Side represents the side of an order (Buy or Sell).
type Side int

const (
	Buy Side = iota
	Sell
)

// String returns the string representation of a Side.
func (s Side) String() string {
	switch s {
	case Buy:
		return "BUY"
	case Sell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON converts a Side to its string representation for JSON encoding.
func (s Side) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON converts a string to a Side for JSON decoding.
func (s *Side) UnmarshalJSON(data []byte) error {
	str := string(data)
	// Remove quotes from the string
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	switch str {
	case "BUY":
		*s = Buy
	case "SELL":
		*s = Sell
	default:
		return fmt.Errorf("unknown side: %s", str)
	}
	return nil
}

// OrderType represents the type of an order (Limit or Market).
type OrderType int

const (
	Limit OrderType = iota
	Market
)

// String returns the string representation of an OrderType.
func (ot OrderType) String() string {
	switch ot {
	case Limit:
		return "LIMIT"
	case Market:
		return "MARKET"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON converts an OrderType to its string representation for JSON encoding.
func (ot OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ot.String() + `"`), nil
}

// UnmarshalJSON converts a string to an OrderType for JSON decoding.
func (ot *OrderType) UnmarshalJSON(data []byte) error {
	str := string(data)
	// Remove quotes from the string
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	switch str {
	case "LIMIT":
		*ot = Limit
	case "MARKET":
		*ot = Market
	default:
		return fmt.Errorf("unknown order type: %s", str)
	}
	return nil
}

// Order represents a single order in the order book.
type Order struct {
	ID                string      `json:"order_id"`
	Symbol            string      `json:"symbol"`
	Side              Side        `json:"side"`
	Type              OrderType   `json:"type"`
	Price             int64       `json:"price,omitempty"`
	OriginalQuantity  int64       `json:"quantity"`
	RemainingQuantity int64       `json:"remaining_quantity"`
	FilledQuantity    int64       `json:"filled_quantity"`
	Status            OrderStatus `json:"status"`
	Timestamp         int64       `json:"timestamp"`
}

// NewOrder creates and returns a new Order.
func NewOrder(id, symbol string, side Side, orderType OrderType, price, quantity int64) *Order {
	return &Order{
		ID:                id,
		Symbol:            symbol,
		Side:              side,
		Type:              orderType,
		Price:             price,
		OriginalQuantity:  quantity,
		RemainingQuantity: quantity,
		FilledQuantity:    0,
		Status:            Accepted,
		Timestamp:         time.Now().UnixNano(),
	}
}

// String returns the string representation of an Order for logging.
func (o *Order) String() string {
	return fmt.Sprintf("Order[ID: %s, Symbol: %s, Side: %s, Type: %s, Price: %d, Quantity: %d/%d, Status: %s, Timestamp: %d]",
		o.ID, o.Symbol, o.Side, o.Type, o.Price, o.RemainingQuantity, o.OriginalQuantity, o.Status, o.Timestamp)
}

// Validate checks if the order has valid fields.
func (o *Order) Validate() error {
	if o.Type == Limit && o.Price <= 0 {
		return fmt.Errorf("invalid price: must be positive for limit orders")
	}
	if o.OriginalQuantity <= 0 {
		return fmt.Errorf("invalid quantity: must be positive")
	}
	return nil
}
