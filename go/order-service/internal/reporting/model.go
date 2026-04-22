package reporting

import "time"

type DailyRevenue struct {
	Day         time.Time `json:"day"`
	ProductID   string    `json:"productId"`
	ProductName string    `json:"productName"`
	Category    string    `json:"category"`
	RevenueCents int      `json:"revenueCents"`
	UnitsSold   int       `json:"unitsSold"`
	OrderCount  int       `json:"orderCount"`
}

type SalesTrend struct {
	Day          time.Time `json:"day"`
	DailyRevenue int       `json:"dailyRevenue"`
	Rolling7Day  int       `json:"rolling7Day"`
	Rolling30Day int       `json:"rolling30Day"`
}

type ProductPerformance struct {
	ProductID          string  `json:"productId"`
	ProductName        string  `json:"productName"`
	Category           string  `json:"category"`
	CurrentStock       int     `json:"currentStock"`
	TotalUnitsSold     int     `json:"totalUnitsSold"`
	TotalRevenueCents  int     `json:"totalRevenueCents"`
	TotalOrders        int     `json:"totalOrders"`
	AvgOrderValueCents int     `json:"avgOrderValueCents"`
	ReturnCount        int     `json:"returnCount"`
	ReturnRatePct      float64 `json:"returnRatePct"`
}

type InventoryTurnover struct {
	ProductID    string  `json:"productId"`
	ProductName  string  `json:"productName"`
	UnitsSold    int     `json:"unitsSold"`
	CurrentStock int     `json:"currentStock"`
	TurnoverRate float64 `json:"turnoverRate"`
	Rank         int     `json:"rank"`
}

type CustomerSummary struct {
	UserID             string    `json:"userId"`
	OrderCount         int       `json:"orderCount"`
	TotalSpendCents    int       `json:"totalSpendCents"`
	FirstOrderAt       time.Time `json:"firstOrderAt"`
	LastOrderAt        time.Time `json:"lastOrderAt"`
	AvgOrderValueCents int       `json:"avgOrderValueCents"`
	Rank               int       `json:"rank"`
}
