package sina

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
)

const (
	etfURL = "https://vip.stock.finance.sina.com.cn/quotes_service/api/jsonp.php" +
		"/IO.XSRV2.CallbackList['da_yPT46_Ll7K6WD']" +
		"/Market_Center.getHQNodeDataSimple" +
		"?page=1&num=80&sort=changepercent&asc=0&node=etf_hq_fund&_s_r_a=init"

	minRegexpMatches = 2
)

var (
	jsonpWrapRe       = regexp.MustCompile(`\((.*)\)`)
	errNoJSONPWrapper = errors.New("no jsonp wrapper found")
	errUnmarshalSina  = errors.New("unmarshal sina etf")
	errHTTPStatus     = errors.New("http error status")
)

// ETFRow represents a single ETF entry from Sina.
type ETFRow struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Trade         string `json:"trade"`
	Settlement    string `json:"settlement"`
	ChangePercent string `json:"changepercent"`
}

// Client defines the interface for Sina Finance API operations.
type Client interface {
	FetchETFData(ctx context.Context) ([]ETFRow, error)
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
			"Referer":    "https://vip.stock.finance.sina.com.cn/",
		},
	}
}

// FetchETFData fetches ETF data from Sina Finance.
func (c *APIClient) FetchETFData(ctx context.Context) ([]ETFRow, error) {
	c.logger.Info("fetching sina etf data")

	body, err := doGet(ctx, c.client, etfURL, c.headers, c.logger)
	if err != nil {
		c.logger.Error("fetch sina etf failed", "error", err)

		return nil, fmt.Errorf("fetch sina etf: %w", err)
	}

	rows, err := ParseETF(string(body))
	if err != nil {
		c.logger.Error("parse sina etf failed", "error", err)

		return nil, fmt.Errorf("parse sina etf: %w", err)
	}

	c.logger.Info("sina etf fetched", "funds", len(rows))

	return rows, nil
}

// ParseETF parses the Sina ETF JSONP response.
func ParseETF(text string) ([]ETFRow, error) {
	matches := jsonpWrapRe.FindStringSubmatch(text)
	if len(matches) < minRegexpMatches {
		return nil, errNoJSONPWrapper
	}

	var rows []ETFRow

	err := json.Unmarshal([]byte(matches[1]), &rows)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnmarshalSina, err)
	}

	return rows, nil
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
