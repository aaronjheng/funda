package data

const percentFactor = 100.0

// FundData holds all data for a single fund.
type FundData struct {
	Code       string  `json:"code"`
	Alias      string  `json:"alias"`
	Name       string  `json:"name"`
	NAV        float64 `json:"nav"`
	AccNAV     float64 `json:"acc_nav"`
	NAVDate    string  `json:"nav_date"`
	DayChange  float64 `json:"day_change"`
	LatestNAV  float64 `json:"latest_nav"`
	LatestTime string  `json:"latest_time"`
	PrevNAV    float64 `json:"prev_nav"`
	IsQDII     bool    `json:"is_qdii"`
}

// DayChangePercent calculates the daily change percentage.
func (f FundData) DayChangePercent() float64 {
	if f.PrevNAV == 0 {
		return 0
	}

	return (f.NAV - f.PrevNAV) / f.PrevNAV * percentFactor
}

// LatestChangePercent calculates the latest change percentage.
func (f FundData) LatestChangePercent() float64 {
	if f.NAV == 0 {
		return 0
	}

	return (f.LatestNAV - f.NAV) / f.NAV * percentFactor
}
