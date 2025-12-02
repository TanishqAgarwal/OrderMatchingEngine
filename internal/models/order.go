package models

import (
	"fmt"
	"time"
)

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

func (os OrderStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + os.String() + `"`), nil
}

type Side int

const (
	Buy Side = iota
	Sell
)

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

func (s Side) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s *Side) UnmarshalJSON(data []byte) error {
	str := string(data)
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

type OrderType int

const (
	Limit OrderType = iota
	Market
)

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

func (ot OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ot.String() + `"`), nil
}

func (ot *OrderType) UnmarshalJSON(data []byte) error {
	str := string(data)
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

// returns the string representation of an Order for logging.
func (o *Order) String() string {
	return fmt.Sprintf("Order[ID: %s, Symbol: %s, Side: %s, Type: %s, Price: %d, Quantity: %d/%d, Status: %s, Timestamp: %d]",
		o.ID, o.Symbol, o.Side, o.Type, o.Price, o.RemainingQuantity, o.OriginalQuantity, o.Status, o.Timestamp)
}

func (o *Order) Validate() error {
	if o.Type == Limit && o.Price <= 0 {
		return fmt.Errorf("invalid price: must be positive for limit orders")
	}
	if o.OriginalQuantity <= 0 {
		return fmt.Errorf("invalid quantity: must be positive")
	}
	return nil
}
