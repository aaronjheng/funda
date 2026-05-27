package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	unquotedKeyRe   = regexp.MustCompile(`([a-zA-Z_]\w*)\s*:`)
	trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
	jsonpWrapRe     = regexp.MustCompile(`\((.*)\)`)
	fundNameRe      = regexp.MustCompile(`var\s+fS_name\s*=\s*"([^"]*)";`)
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
	minDataItems      = 7
	minRegexpMatches  = 2

	fundRowCodeIndex    = 0
	fundRowNameIndex    = 1
	fundRowNAVIndex     = 5
	fundRowAccNAVIndex  = 6
	fundRowPrevNAVIndex = 7
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

		var row FundRow

		row.Code = unmarshalString(item[fundRowCodeIndex])
		row.Name = unmarshalString(item[fundRowNameIndex])
		row.NAV = unmarshalFloat(item[fundRowNAVIndex])
		row.AccNAV = unmarshalFloat(item[fundRowAccNAVIndex])

		if len(item) > fundRowPrevNAVIndex {
			row.PrevNAV = unmarshalFloat(item[fundRowPrevNAVIndex])
			row.DayChange = row.NAV - row.PrevNAV
			row.DayPct = calculatePercent(row.DayChange, row.PrevNAV)
		}

		row.NAVDate = showday[0]
		row.PrevDate = showday[1]
		rows = append(rows, row)
	}

	return rows
}

// FundGZ represents the real-time fund estimate from EastMoney fundgz API.
type FundGZ struct {
	FundCode string `json:"fundcode"`
	Name     string `json:"name"`
	JZRQ     string `json:"jzrq"`
	DWJZ     string `json:"dwjz"`
	GSZ      string `json:"gsz"`
	GSZZL    string `json:"gszzl"`
	GZTime   string `json:"gztime"`
}

// ParseFundGZ parses the EastMoney fundgz JSONP response.
func ParseFundGZ(text string) (FundGZ, error) {
	matches := jsonpWrapRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return FundGZ{}, errNoJSONPWrapper
	}

	var fundGZ FundGZ

	err := json.Unmarshal([]byte(matches[1]), &fundGZ)
	if err != nil {
		return FundGZ{}, fmt.Errorf("%w: %w", errUnmarshalEastMoney, err)
	}

	return fundGZ, nil
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

// ParseFundInfo extracts fund metadata and net worth trend from EastMoney per-fund JS.
func ParseFundInfo(text string) (FundInfo, error) {
	matches := netWorthRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return FundInfo{}, errNoNetWorthTrend
	}

	var entries []NetWorthPoint

	err := json.Unmarshal([]byte(matches[1]), &entries)
	if err != nil {
		return FundInfo{}, fmt.Errorf("%w: %w", errUnmarshalNetWorthTrend, err)
	}

	info := FundInfo{
		Name:          "",
		NetWorthTrend: entries,
	}

	nameMatches := fundNameRe.FindStringSubmatch(text)
	if len(nameMatches) >= minRegexpMatches {
		info.Name = nameMatches[1]
	}

	return info, nil
}

func formatFundInfoDate(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}

	return time.UnixMilli(timestamp).In(shanghaiLoc).Format("2006-01-02")
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

func calculatePercent(change, base float64) float64 {
	if base == 0 {
		return 0
	}

	return change / base * percentFactor
}
