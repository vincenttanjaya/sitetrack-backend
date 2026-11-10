package domain

import "time"

type StockStatus string

const (
	OutOfStock StockStatus = "out_of_stock"
	LowStock   StockStatus = "low_stock"
	Available  StockStatus = "available"
)

func (s StockStatus) Valid() bool {
	return s == OutOfStock || s == LowStock || s == Available
}

type CompanyStockItem struct {
	Material
	Quantity          Quantity    `json:"quantity"`
	InTransitQuantity Quantity    `json:"in_transit_quantity"`
	ReorderPoint      Quantity    `json:"reorder_point"`
	Status            StockStatus `json:"status"`
}

type ProjectStockItem struct {
	Material
	Quantity       Quantity   `json:"quantity"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
}

type CompanyStockFilter struct {
	Search, Category, Sort, Order string
	Status                        StockStatus
	Page                          Page
}

type ProjectStockFilter struct {
	Search, Category, Sort, Order string
	Page                          Page
}
