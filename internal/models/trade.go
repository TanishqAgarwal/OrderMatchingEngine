package models

import (
	"fmt"
	"time"
)

// Trade represents a matched trade between a buyer and a seller.
type Trade struct {
	ID            string
	BuyerOrderID  string
	SellerOrderID string
	Price         int64
	Quantity      int64
	Timestamp     int64
}

// NewTrade creates and returns a new Trade.
func NewTrade(id, buyerOrderID, sellerOrderID string, price, quantity int64) *Trade {
	return &Trade{
		ID:            id,
		BuyerOrderID:  buyerOrderID,
		SellerOrderID: sellerOrderID,
		Price:         price,
		Quantity:      quantity,
		Timestamp:     time.Now().UnixNano(),
	}
}

// String returns the string representation of a Trade for logging.
func (t *Trade) String() string {
	return fmt.Sprintf("Trade[ID: %s, BuyerOrderID: %s, SellerOrderID: %s, Price: %d, Quantity: %d, Timestamp: %d]",
		t.ID, t.BuyerOrderID, t.SellerOrderID, t.Price, t.Quantity, t.Timestamp)
}
