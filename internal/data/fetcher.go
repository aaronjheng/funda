package data

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errHTTPStatus = errors.New("http error status")

const (
	httpClientTimeout = 30 * time.Second
	maxConcurrent     = 3
	maxSearchResults  = 10
	minFundInfoPoints = 2
)

const (
	eastMoneyBulkURL = "https://fund.eastmoney.com/Data/Fund_JJJZ_Data.aspx" +
		"?t=1&lx=1&letter=&gsid=&text=&sort=zdf,desc" +
		"&page=1,50000&dt=1580914040623&atfc=&onlySale=0"

	sinaETFURL = "https://vip.stock.finance.sina.com.cn/quotes_service/api/jsonp.php" +
		"/IO.XSRV2.CallbackList['da_yPT46_Ll7K6WD']" +
		"/Market_Center.getHQNodeDataSimple" +
		"?page=1&num=80&sort=changepercent&asc=0&node=etf_hq_fund&_s_r_a=init"

	fundGZURL = "https://fundgz.1234567.com.cn/js/%s.js"
)

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

// Fetcher handles HTTP requests and caching for fund data.
type Fetcher struct {
	client         *http.Client
	sem            chan struct{}
	memCache       *MemoryCache
	etfCache       *ETFTickerCache
	logger         *slog.Logger
	defaultHeaders map[string]string
	sinaHeaders    map[string]string
}

// NewFetcher creates a new Fetcher with default settings.
func NewFetcher(logger *slog.Logger) *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: httpClientTimeout},
		sem:      make(chan struct{}, maxConcurrent),
		memCache: NewMemoryCache(logger),
		etfCache: &ETFTickerCache{
			mu: sync.RWMutex{}, data: nil, timestamp: time.Time{}, logger: logger,
		},
		logger: logger,
		defaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Referer":    "https://fund.eastmoney.com/",
		},
		sinaHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Referer":    "https://vip.stock.finance.sina.com.cn/",
		},
	}
}

// FetchAllFunds fetches the bulk EastMoney fund data.
func (f *Fetcher) FetchAllFunds(ctx context.Context) ([]FundRow, string, string, error) {
	f.logger.Info("fetching eastmoney bulk data")

	body, err := f.get(ctx, eastMoneyBulkURL, f.defaultHeaders)
	if err != nil {
		f.logger.Error("fetch eastmoney bulk failed", "error", err)

		return nil, "", "", fmt.Errorf("fetch eastmoney bulk: %w", err)
	}

	rows, navDate, prevDate, err := ParseEastMoneyBulk(string(body))
	if err != nil {
		f.logger.Error("parse eastmoney bulk failed", "error", err)

		return nil, "", "", err
	}

	f.logger.Info("eastmoney bulk fetched", "funds", len(rows), "nav_date", navDate)

	return rows, navDate, prevDate, nil
}

// FetchETFData fetches ETF data from Sina Finance.
func (f *Fetcher) FetchETFData(ctx context.Context) ([]ETFRow, error) {
	if data, ok := f.etfCache.Get(); ok {
		return data, nil
	}

	f.logger.Info("fetching sina etf data")

	body, err := f.get(ctx, sinaETFURL, f.sinaHeaders)
	if err != nil {
		f.logger.Error("fetch sina etf failed", "error", err)

		return nil, fmt.Errorf("fetch sina etf: %w", err)
	}

	rows, err := ParseSinaETF(string(body))
	if err != nil {
		f.logger.Error("parse sina etf failed", "error", err)

		return nil, fmt.Errorf("parse sina etf: %w", err)
	}

	f.etfCache.Set(rows)
	f.logger.Info("sina etf fetched", "funds", len(rows))

	return rows, nil
}

// FetchFundInfo fetches per-fund detail for NAV fallback.
func (f *Fetcher) FetchFundInfo(ctx context.Context, code string) (FundInfo, error) {
	f.logger.Debug("fetching per-fund info", "code", code)

	url := fmt.Sprintf("https://fund.eastmoney.com/pingzhongdata/%s.js", code)

	body, err := f.get(ctx, url, f.defaultHeaders)
	if err != nil {
		f.logger.Warn("fetch per-fund info failed", "code", code, "error", err)

		return FundInfo{}, fmt.Errorf("fetch fund info for %s: %w", code, err)
	}

	return ParseFundInfo(string(body))
}

// EstimateUpdate holds a single fund estimate refresh result.
type EstimateUpdate struct {
	LatestNAV  float64
	LatestTime string
}

// RefreshAllEstimates fetches estimates for multiple fund codes concurrently.
// It only updates LatestNAV and LatestTime, without re-fetching NAV data.
func (f *Fetcher) RefreshAllEstimates(ctx context.Context, codes []string) map[string]EstimateUpdate {
	f.logger.Info("refreshing estimates", "count", len(codes))

	etfRows, _ := f.FetchETFData(ctx)

	result := make(map[string]EstimateUpdate)

	var (
		estMu sync.Mutex
		group sync.WaitGroup
	)

	for _, code := range codes {
		group.Add(1)

		go func(fundCode string) {
			defer group.Done()

			f.sem <- struct{}{}
			defer func() { <-f.sem }()

			latestNAV, latestTime := f.refreshEstimate(ctx, fundCode, etfRows)

			estMu.Lock()
			result[fundCode] = EstimateUpdate{LatestNAV: latestNAV, LatestTime: latestTime}
			estMu.Unlock()
		}(code)
	}

	group.Wait()

	f.logger.Info("estimates refreshed", "updated", len(result))

	return result
}

// GetFundDataFull returns complete fund data for a single fund code.
func (f *Fetcher) GetFundDataFull(ctx context.Context, code, alias string) (FundData, error) {
	if cached, ok := f.memCache.Get(code); ok {
		cached.Alias = alias

		return cached, nil
	}

	if cached, ok := LoadFundCache(f.logger, code); ok {
		cached.Alias = alias
		f.memCache.Set(code, cached)

		return cached, nil
	}

	f.logger.Info("hydrating fund data", "code", code)

	var fund FundData

	fund.Code = code
	fund.Alias = alias

	f.populateFromBulk(ctx, &fund, code)
	f.populateFromFundInfo(ctx, &fund, code)
	f.populatePrevNAV(ctx, &fund, code)
	f.populateFromETF(ctx, &fund, code)

	// Last resort: if all APIs failed, use PrevNAV from the bulk API.
	if fund.NAV == 0 && fund.PrevNAV > 0 {
		fund.NAV = fund.PrevNAV
		fund.PrevNAV = 0

		f.logger.Debug("using prevnav as nav fallback", "code", code)
	}

	if fund.NAV > 0 {
		f.addEstimate(ctx, &fund, code)
	}

	if fund.NAV > 0 {
		f.memCache.Set(code, fund)
		SaveFundCache(f.logger, fund)
		f.logger.Info("fund data hydrated", "code", code, "nav", fund.NAV, "nav_date", fund.NAVDate)
	} else {
		f.logger.Warn("fund data hydration incomplete", "code", code)
	}

	return fund, nil
}

// ClearCache clears all in-memory and on-disk caches.
func (f *Fetcher) ClearCache() {
	f.logger.Info("clearing all caches")

	f.memCache.Clear()
	f.etfCache = &ETFTickerCache{mu: sync.RWMutex{}, data: nil, timestamp: time.Time{}, logger: f.logger}

	ClearFundCache(f.logger)
}

// RemoveCachedEntries removes only the specified fund codes from both memory and disk cache.
func (f *Fetcher) RemoveCachedEntries(codes []string) {
	f.logger.Info("removing cached entries", "count", len(codes))

	for _, code := range codes {
		f.memCache.Remove(code)
		DeleteFundCache(f.logger, code)
	}

	f.etfCache = &ETFTickerCache{mu: sync.RWMutex{}, data: nil, timestamp: time.Time{}, logger: f.logger}
}

// SearchFund searches for funds by keyword.
func (f *Fetcher) SearchFund(ctx context.Context, keyword string) ([]SearchHit, error) {
	f.logger.Info("searching funds", "keyword", keyword)

	var results []SearchHit

	results = f.searchInBulkFunds(ctx, keyword, results)
	results = f.searchInETFFunds(ctx, keyword, results)

	f.logger.Info("search completed", "keyword", keyword, "results", len(results))

	return results, nil
}

// FetchAllCards fetches data for multiple funds concurrently with semaphore limiting.
func (f *Fetcher) FetchAllCards(
	ctx context.Context,
	funds []struct{ Code, Alias string },
) map[string]FundData {
	f.logger.Info("fetching all cards", "count", len(funds))

	result := make(map[string]FundData)

	var (
		mut   sync.Mutex
		group sync.WaitGroup
	)

	for _, fund := range funds {
		group.Add(1)

		go func(code, alias string) {
			defer group.Done()

			f.sem <- struct{}{}
			defer func() { <-f.sem }()

			data, err := f.GetFundDataFull(ctx, code, alias)

			mut.Lock()
			if err == nil {
				result[code] = data
			}
			mut.Unlock()
		}(fund.Code, fund.Alias)
	}

	group.Wait()

	f.logger.Info("all cards fetched", "fetched", len(result))

	return result
}

func (f *Fetcher) get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	f.logger.Debug("http request", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", url, err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.logger.Warn("http response not ok", "url", url, "status", resp.StatusCode)

		return nil, fmt.Errorf("%w: %d", errHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	f.logger.Debug("http response", "url", url, "bytes", len(body))

	return body, nil
}

func (f *Fetcher) populateFromBulk(ctx context.Context, fund *FundData, code string) {
	rows, navDate, prevDate, err := f.FetchAllFunds(ctx)
	if err != nil {
		f.logger.Debug("populate from bulk skipped", "code", code, "error", err)

		return
	}

	for _, row := range rows {
		if row.Code == code {
			fund.Name = row.Name
			fund.NAV = row.NAV
			fund.AccNAV = row.AccNAV
			fund.PrevNAV = row.PrevNAV
			fund.DayChange = row.DayChange
			fund.NAVDate = navDate
			fund.IsQDII = strings.Contains(row.Name, "QDII")

			// If the bulk API doesn't have NAV for today, note the previous date
			// but don't override NAV — let populateFromFundInfo try the per-fund API first.
			if fund.NAV == 0 {
				fund.NAVDate = prevDate
			}

			return
		}
	}

	f.logger.Debug("fund not found in bulk", "code", code)
}

func (f *Fetcher) populatePrevNAV(ctx context.Context, fund *FundData, code string) {
	if fund.PrevNAV != 0 || fund.NAV <= 0 {
		return
	}

	info, err := f.FetchFundInfo(ctx, code)
	if err != nil || len(info.NetWorthTrend) < minFundInfoPoints {
		f.logger.Debug("populate prevnav skipped", "code", code)

		return
	}

	fund.PrevNAV = info.NetWorthTrend[len(info.NetWorthTrend)-2].Y
	fund.DayChange = fund.NAV - fund.PrevNAV
	f.logger.Debug("populated prevnav from fund info", "code", code, "prevnav", fund.PrevNAV)
}

func (f *Fetcher) populateFromFundInfo(ctx context.Context, fund *FundData, code string) {
	if fund.NAV > 0 {
		return
	}

	info, err := f.FetchFundInfo(ctx, code)
	if err != nil || len(info.NetWorthTrend) == 0 {
		f.logger.Debug("populate from fund info skipped", "code", code)

		return
	}

	latest := info.NetWorthTrend[len(info.NetWorthTrend)-1]
	if latest.Y <= 0 {
		return
	}

	if fund.Name == "" {
		fund.Name = info.Name
	}

	fund.NAV = latest.Y
	fund.NAVDate = formatFundInfoDate(latest.X)

	if len(info.NetWorthTrend) >= minFundInfoPoints {
		fund.PrevNAV = info.NetWorthTrend[len(info.NetWorthTrend)-2].Y
		fund.DayChange = fund.NAV - fund.PrevNAV
	}

	f.logger.Debug("populated from fund info", "code", code, "nav", fund.NAV)
}

func (f *Fetcher) populateFromETF(ctx context.Context, fund *FundData, code string) {
	if fund.NAV != 0 {
		return
	}

	etfRows, err := f.FetchETFData(ctx)
	if err != nil {
		f.logger.Debug("populate from etf skipped", "code", code, "error", err)

		return
	}

	for _, row := range etfRows {
		if !strings.HasSuffix(row.Symbol, code) {
			continue
		}

		fund.Name = row.Name

		trade, _ := strconv.ParseFloat(row.Trade, 64)
		settle, _ := strconv.ParseFloat(row.Settlement, 64)

		fund.NAV = trade
		fund.LatestNAV = trade
		fund.PrevNAV = settle
		fund.DayChange = trade - settle
		fund.NAVDate = time.Now().In(shanghaiLoc).Format("2006-01-02")

		f.logger.Debug("populated from etf", "code", code, "nav", fund.NAV)

		return
	}
}

func (f *Fetcher) searchInBulkFunds(
	ctx context.Context,
	keyword string,
	results []SearchHit,
) []SearchHit {
	rows, _, _, err := f.FetchAllFunds(ctx)
	if err != nil {
		return results
	}

	count := 0

	for _, row := range rows {
		if !strings.Contains(row.Code, keyword) && !strings.Contains(row.Name, keyword) {
			continue
		}

		results = append(results, SearchHit{
			Code:   row.Code,
			Name:   row.Name,
			Price:  strconv.FormatFloat(row.NAV, 'f', 4, 64),
			Change: strconv.FormatFloat(row.DayPct, 'f', 2, 64) + "%",
		})

		count++
		if count >= maxSearchResults {
			break
		}
	}

	return results
}

func (f *Fetcher) searchInETFFunds(
	ctx context.Context,
	keyword string,
	results []SearchHit,
) []SearchHit {
	if len(results) > 0 {
		return results
	}

	etfRows, err := f.FetchETFData(ctx)
	if err != nil {
		return results
	}

	count := 0

	for _, row := range etfRows {
		if !strings.Contains(row.Symbol, keyword) && !strings.Contains(row.Name, keyword) {
			continue
		}

		code := strings.TrimPrefix(row.Symbol, "sh")
		code = strings.TrimPrefix(code, "sz")

		results = append(results, SearchHit{
			Code:   code,
			Name:   row.Name,
			Price:  row.Trade,
			Change: row.ChangePercent + "%",
		})

		count++
		if count >= maxSearchResults {
			break
		}
	}

	return results
}

func (f *Fetcher) refreshEstimate(ctx context.Context, code string, etfRows []ETFRow) (float64, string) {
	for _, row := range etfRows {
		if strings.HasSuffix(row.Symbol, code) {
			trade, _ := strconv.ParseFloat(row.Trade, 64)
			if trade > 0 {
				return trade, time.Now().In(shanghaiLoc).Format("15:04:05")
			}
		}
	}

	fundGZ, err := f.fetchFundEstimate(ctx, code)
	if err != nil {
		f.logger.Debug("estimate refresh failed", "code", code, "error", err)

		return 0, ""
	}

	gsz, _ := strconv.ParseFloat(fundGZ.GSZ, 64)

	return gsz, fundGZ.GZTime
}

// fetchFundEstimate fetches the latest fund estimate from EastMoney fundgz API.
func (f *Fetcher) fetchFundEstimate(ctx context.Context, code string) (FundGZ, error) {
	url := fmt.Sprintf(fundGZURL, code)

	body, err := f.get(ctx, url, f.defaultHeaders)
	if err != nil {
		return FundGZ{}, fmt.Errorf("fetch fund estimate for %s: %w", code, err)
	}

	return ParseFundGZ(string(body))
}

func (f *Fetcher) addEstimate(ctx context.Context, fund *FundData, code string) {
	etfRows, err := f.FetchETFData(ctx)
	if err == nil {
		for _, row := range etfRows {
			if strings.HasSuffix(row.Symbol, code) {
				trade, _ := strconv.ParseFloat(row.Trade, 64)
				if trade > 0 {
					fund.LatestNAV = trade
					fund.LatestTime = time.Now().In(shanghaiLoc).Format("15:04:05")

					return
				}
			}
		}
	}

	fundGZ, err := f.fetchFundEstimate(ctx, code)
	if err != nil {
		f.logger.Debug("add estimate failed", "code", code, "error", err)

		return
	}

	if fund.NAV == 0 {
		dwjz, _ := strconv.ParseFloat(fundGZ.DWJZ, 64)
		if dwjz > 0 {
			fund.NAV = dwjz
			fund.NAVDate = fundGZ.JZRQ

			if fund.PrevNAV > 0 {
				fund.DayChange = fund.NAV - fund.PrevNAV
			}
		}
	}

	gsz, _ := strconv.ParseFloat(fundGZ.GSZ, 64)
	if gsz > 0 {
		fund.LatestNAV = gsz
		fund.LatestTime = fundGZ.GZTime
	}

	f.logger.Debug("estimate added", "code", code, "latest_nav", fund.LatestNAV)
}
