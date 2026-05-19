package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	unquotedKeyRe   = regexp.MustCompile(`([a-zA-Z_]\w*)\s*:`)
	trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
	jsonpWrapRe     = regexp.MustCompile(`\((.*)\)`)
	netWorthRe      = regexp.MustCompile(`var\s+Data_netWorthTrend\s*=\s*(\[.*?\]);`)

	errInsufficientShowday    = errors.New("insufficient showday data")
	errNoJSONPWrapper         = errors.New("no jsonp wrapper found")
	errNoNetWorthTrend        = errors.New("no Data_netWorthTrend found")
	errUnmarshalEastMoney     = errors.New("unmarshal eastmoney")
	errUnmarshalSinaETF       = errors.New("unmarshal sina etf")
	errUnmarshalNetWorthTrend = errors.New("unmarshal net worth trend")
)

const (
	minShowdayEntries = 2
	minDataItems      = 9
	minRegexpMatches  = 2
)

// FundRow represents a parsed row from EastMoney bulk data.
type FundRow struct {
	Code       string
	Name       string
	NAV        float64
	AccNAV     float64
	PrevNAV    float64
	PrevAccNAV float64
	DayChange  float64
	DayPct     float64
	NAVDate    string
	PrevDate   string
}

// ParseEastMoneyBulk parses the EastMoney bulk fund response.
func ParseEastMoneyBulk(text string) ([]FundRow, string, string, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "var db=")

	text = unquotedKeyRe.ReplaceAllString(text, `"$1":`)
	text = trailingCommaRe.ReplaceAllString(text, "$1")

	var raw struct {
		Showday []string            `json:"showday"`
		Datas   [][]json.RawMessage `json:"datas"`
	}

	err := json.Unmarshal([]byte(text), &raw)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: %w", errUnmarshalEastMoney, err)
	}

	if len(raw.Showday) < minShowdayEntries {
		return nil, "", "", errInsufficientShowday
	}

	rows := parseEastMoneyRows(raw.Datas, raw.Showday)

	return rows, raw.Showday[0], raw.Showday[1], nil
}

func parseEastMoneyRows(datas [][]json.RawMessage, showday []string) []FundRow {
	var rows []FundRow

	for _, item := range datas {
		if len(item) < minDataItems {
			continue
		}

		row := FundRow{} //nolint:exhaustruct // fields populated individually below
		row.Code = unmarshalString(item[0])
		row.Name = unmarshalString(item[1])
		row.NAV = unmarshalFloat(item[3])
		row.AccNAV = unmarshalFloat(item[4])
		row.PrevNAV = unmarshalFloat(item[5])
		row.PrevAccNAV = unmarshalFloat(item[6])
		row.DayChange = unmarshalFloat(item[7])
		row.DayPct = unmarshalFloat(item[8])
		row.NAVDate = showday[0]
		row.PrevDate = showday[1]
		rows = append(rows, row)
	}

	return rows
}

// ParseSinaETF parses the Sina ETF JSONP response.
func ParseSinaETF(text string) ([]ETFRow, error) {
	matches := jsonpWrapRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return nil, errNoJSONPWrapper
	}

	var rows []ETFRow

	err := json.Unmarshal([]byte(matches[1]), &rows)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnmarshalSinaETF, err)
	}

	return rows, nil
}

// ParseFundInfo extracts net worth trend from EastMoney per-fund JS.
func ParseFundInfo(text string) ([]NetWorthPoint, error) {
	matches := netWorthRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return nil, errNoNetWorthTrend
	}

	var entries []NetWorthPoint

	err := json.Unmarshal([]byte(matches[1]), &entries)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnmarshalNetWorthTrend, err)
	}

	return entries, nil
}

func unmarshalString(raw json.RawMessage) string {
	var s string

	_ = json.Unmarshal(raw, &s)

	return s
}

func unmarshalFloat(raw json.RawMessage) float64 {
	var str string

	err := json.Unmarshal(raw, &str)
	if err == nil {
		value, _ := strconv.ParseFloat(str, 64)

		return value
	}

	var value float64

	_ = json.Unmarshal(raw, &value)

	return value
}
