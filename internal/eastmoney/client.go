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
	"time"
)

const (
	fundGZURL = "https://fundgz.1234567.com.cn/js/%s.js"

	minRegexpMatches = 2

	// MinFundInfoPoints is the minimum number of net worth points required.
	MinFundInfoPoints = 2

	httpClientTimeout = 30 * time.Second
)

var (
	jsonpWrapRe = regexp.MustCompile(`\((.*)\)`)
	fundNameRe  = regexp.MustCompile(`var\s+fS_name\s*=\s*"([^"]*)";`)
	netWorthRe  = regexp.MustCompile(`var\s+Data_netWorthTrend\s*=\s*(\[.*?\]);`)

	errNoJSONPWrapper         = errors.New("no jsonp wrapper found")
	errNoNetWorthTrend        = errors.New("no Data_netWorthTrend found")
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
func NewAPIClient(logger *slog.Logger) *APIClient {
	return &APIClient{
		client: &http.Client{Timeout: httpClientTimeout},
		logger: logger,
		headers: map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Referer":    "https://fund.eastmoney.com/",
		},
	}
}

// FetchFundInfo fetches per-fund detail including net worth trend.
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

// ParseFundGZ parses the EastMoney fundgz JSONP response.
func ParseFundGZ(text string) (FundGZ, error) {
	matches := jsonpWrapRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return FundGZ{}, errNoJSONPWrapper
	}

	var fundGZ FundGZ

	err := json.Unmarshal([]byte(matches[1]), &fundGZ)
	if err != nil {
		return FundGZ{}, fmt.Errorf("unmarshal fund gz: %w", err)
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
