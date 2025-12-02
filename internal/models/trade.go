package models

import (
	"fmt"
	"time"
)

type Trade struct {
	ID            string
	BuyerOrderID  string
	SellerOrderID string
	Price         int64
	Quantity      int64
	Timestamp     int64
}

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

// returns the string representation of a Trade for logging.
func (t *Trade) String() string {
	return fmt.Sprintf("Trade[ID: %s, BuyerOrderID: %s, SellerOrderID: %s, Price: %d, Quantity: %d, Timestamp: %d]",
		t.ID, t.BuyerOrderID, t.SellerOrderID, t.Price, t.Quantity, t.Timestamp)
}
