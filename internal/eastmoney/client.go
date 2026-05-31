package eastmoney

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	bulkURL = "https://fund.eastmoney.com/Data/Fund_JJJZ_Data.aspx" +
		"?t=1&lx=1&letter=&gsid=&text=&sort=zdf,desc" +
		"&page=1,50000&dt=1580914040623&atfc=&onlySale=0"

	fundGZURL = "https://fundgz.1234567.com.cn/js/%s.js"

	minShowdayEntries = 2
	minDataItems      = 7
	minRegexpMatches  = 2

	fundRowCodeIndex    = 0
	fundRowNameIndex    = 1
	fundRowNAVIndex     = 3
	fundRowAccNAVIndex  = 4
	fundRowPrevNAVIndex = 5

	// MinFundInfoPoints is the minimum number of net worth points required.
	MinFundInfoPoints = 2

	percentFactor = 100.0
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
	errUnmarshalNetWorthTrend = errors.New("unmarshal net worth trend")
	errHTTPStatus             = errors.New("http error status")
)

// ShanghaiLocation is the Asia/Shanghai time location.
//
//nolint:gochecknoglobals // timezone lookup is immutable
var ShanghaiLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}

	return loc
}()

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

// NetWorthPoint represents a single data point from fund net worth trend.
type NetWorthPoint struct {
	X int64   `json:"x"`
	Y float64 `json:"y"`
}

// FundInfo represents data extracted from EastMoney per-fund detail.
type FundInfo struct {
	Name          string
	NetWorthTrend []NetWorthPoint
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

// Client defines the interface for EastMoney API operations.
type Client interface {
	FetchBulk(ctx context.Context) ([]FundRow, string, string, error)
	FetchFundInfo(ctx context.Context, code string) (FundInfo, error)
	FetchFundEstimate(ctx context.Context, code string) (FundGZ, error)
}

// Doer abstracts http.Client.Do for testability.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// APIClient implements Client using HTTP requests.
type APIClient struct {
	client  Doer
	logger  *slog.Logger
	headers map[string]string
}

// NewAPIClient creates a new APIClient.
func NewAPIClient(client Doer, logger *slog.Logger) *APIClient {
	return &APIClient{
		client: client,
		logger: logger,
		headers: map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Referer":    "https://fund.eastmoney.com/",
		},
	}
}

// FetchBulk fetches the bulk EastMoney fund data.
func (c *APIClient) FetchBulk(ctx context.Context) ([]FundRow, string, string, error) {
	c.logger.Info("fetching eastmoney bulk data")

	body, err := doGet(ctx, c.client, bulkURL, c.headers, c.logger)
	if err != nil {
		c.logger.Error("fetch eastmoney bulk failed", "error", err)

		return nil, "", "", fmt.Errorf("fetch eastmoney bulk: %w", err)
	}

	rows, navDate, prevDate, err := ParseBulk(string(body))
	if err != nil {
		c.logger.Error("parse eastmoney bulk failed", "error", err)

		return nil, "", "", err
	}

	c.logger.Info("eastmoney bulk fetched", "funds", len(rows), "nav_date", navDate)

	return rows, navDate, prevDate, nil
}

// FetchFundInfo fetches per-fund detail for NAV fallback.
func (c *APIClient) FetchFundInfo(ctx context.Context, code string) (FundInfo, error) {
	c.logger.Debug("fetching per-fund info", "code", code)

	url := fmt.Sprintf("https://fund.eastmoney.com/pingzhongdata/%s.js", code)

	body, err := doGet(ctx, c.client, url, c.headers, c.logger)
	if err != nil {
		c.logger.Warn("fetch per-fund info failed", "code", code, "error", err)

		return FundInfo{}, fmt.Errorf("fetch fund info for %s: %w", code, err)
	}

	return ParseFundInfo(string(body))
}

// FetchFundEstimate fetches the latest fund estimate from EastMoney fundgz API.
func (c *APIClient) FetchFundEstimate(ctx context.Context, code string) (FundGZ, error) {
	url := fmt.Sprintf(fundGZURL, code)

	body, err := doGet(ctx, c.client, url, c.headers, c.logger)
	if err != nil {
		return FundGZ{}, fmt.Errorf("fetch fund estimate for %s: %w", code, err)
	}

	return ParseFundGZ(string(body))
}

// ParseBulk parses the EastMoney bulk fund response.
func ParseBulk(text string) ([]FundRow, string, string, error) {
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

	rows := parseRows(raw.Datas, raw.Showday)

	return rows, raw.Showday[0], raw.Showday[1], nil
}

func parseRows(datas [][]json.RawMessage, showday []string) []FundRow {
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

// FormatFundInfoDate formats a millisecond timestamp to a date string.
func FormatFundInfoDate(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}

	return time.UnixMilli(timestamp).In(ShanghaiLocation).Format("2006-01-02")
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

func doGet(
	ctx context.Context,
	client Doer,
	url string,
	headers map[string]string,
	logger *slog.Logger,
) ([]byte, error) {
	logger.Debug("http request", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", url, err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Warn("http response not ok", "url", url, "status", resp.StatusCode)

		return nil, fmt.Errorf("%w: %d", errHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	logger.Debug("http response", "url", url, "bytes", len(body))

	return body, nil
}
