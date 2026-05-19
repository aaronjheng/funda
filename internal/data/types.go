package data

const percentFactor = 100.0

// SearchHit holds a single fund search result.
type SearchHit struct {
	Code   string
	Name   string
	Price  string
	Change string
}

// FundData holds all data for a single fund.
type FundData struct {
	Code         string  `json:"code"`
	Alias        string  `json:"alias"`
	Name         string  `json:"name"`
	NAV          float64 `json:"nav"`
	AccNAV       float64 `json:"acc_nav"`       //nolint:tagliatelle // snake_case for cache compatibility
	NAVDate      string  `json:"nav_date"`      //nolint:tagliatelle // snake_case for cache compatibility
	DayChange    float64 `json:"day_change"`    //nolint:tagliatelle // snake_case for cache compatibility
	EstimateNAV  float64 `json:"estimate_nav"`  //nolint:tagliatelle // snake_case for cache compatibility
	EstimateTime string  `json:"estimate_time"` //nolint:tagliatelle // snake_case for cache compatibility
	PrevNAV      float64 `json:"prev_nav"`      //nolint:tagliatelle // snake_case for cache compatibility
}

// DayChangePercent calculates the daily change percentage.
func (f FundData) DayChangePercent() float64 {
	if f.PrevNAV == 0 {
		return 0
	}

	return (f.NAV - f.PrevNAV) / f.PrevNAV * percentFactor
}

// EstimateChangePercent calculates the estimate change percentage.
func (f FundData) EstimateChangePercent() float64 {
	if f.NAV == 0 {
		return 0
	}

	return (f.EstimateNAV - f.NAV) / f.NAV * percentFactor
}
