package data

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	defaultHeaders map[string]string
	sinaHeaders    map[string]string
}

// NewFetcher creates a new Fetcher with default settings.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client:   &http.Client{Timeout: httpClientTimeout},
		sem:      make(chan struct{}, maxConcurrent),
		memCache: NewMemoryCache(),
		etfCache: &ETFTickerCache{mu: sync.RWMutex{}, data: nil, timestamp: time.Time{}},
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
	body, err := f.get(ctx, eastMoneyBulkURL, f.defaultHeaders)
	if err != nil {
		return nil, "", "", fmt.Errorf("fetch eastmoney bulk: %w", err)
	}

	return ParseEastMoneyBulk(string(body))
}

// FetchETFData fetches ETF data from Sina Finance.
func (f *Fetcher) FetchETFData(ctx context.Context) ([]ETFRow, error) {
	if data, ok := f.etfCache.Get(); ok {
		return data, nil
	}

	body, err := f.get(ctx, sinaETFURL, f.sinaHeaders)
	if err != nil {
		return nil, fmt.Errorf("fetch sina etf: %w", err)
	}

	rows, err := ParseSinaETF(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse sina etf: %w", err)
	}

	f.etfCache.Set(rows)

	return rows, nil
}

// FetchFundInfo fetches per-fund detail for NAV fallback.
func (f *Fetcher) FetchFundInfo(ctx context.Context, code string) (FundInfo, error) {
	url := fmt.Sprintf("https://fund.eastmoney.com/pingzhongdata/%s.js", code)

	body, err := f.get(ctx, url, f.defaultHeaders)
	if err != nil {
		return FundInfo{}, fmt.Errorf("fetch fund info for %s: %w", code, err)
	}

	return ParseFundInfo(string(body))
}

// GetFundDataFull returns complete fund data for a single fund code.
func (f *Fetcher) GetFundDataFull(ctx context.Context, code, alias string) (FundData, error) {
	if cached, ok := f.memCache.Get(code); ok {
		cached.Alias = alias

		return cached, nil
	}

	if cached, ok := LoadFundCache(code); ok {
		cached.Alias = alias
		f.memCache.Set(code, cached)

		return cached, nil
	}

	var fund FundData

	fund.Code = code
	fund.Alias = alias

	f.populateFromBulk(ctx, &fund, code)
	f.populateFromFundInfo(ctx, &fund, code)
	f.populatePrevNAV(ctx, &fund, code)
	f.populateFromETF(ctx, &fund, code)

	if fund.NAV > 0 {
		f.addEstimate(ctx, &fund, code)
	}

	if fund.NAV > 0 {
		f.memCache.Set(code, fund)
		SaveFundCache(fund)
	}

	return fund, nil
}

// ClearCache clears all in-memory and on-disk caches.
func (f *Fetcher) ClearCache() {
	f.memCache.Clear()
	f.etfCache = &ETFTickerCache{mu: sync.RWMutex{}, data: nil, timestamp: time.Time{}}

	ClearFundCache()
}

// SearchFund searches for funds by keyword.
func (f *Fetcher) SearchFund(ctx context.Context, keyword string) ([]SearchHit, error) {
	var results []SearchHit

	results = f.searchInBulkFunds(ctx, keyword, results)
	results = f.searchInETFFunds(ctx, keyword, results)

	return results, nil
}

// FetchAllCards fetches data for multiple funds concurrently with semaphore limiting.
func (f *Fetcher) FetchAllCards(
	ctx context.Context,
	funds []struct{ Code, Alias string },
) map[string]FundData {
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

	return result
}

func (f *Fetcher) get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
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
		return nil, fmt.Errorf("%w: %d", errHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return body, nil
}

func (f *Fetcher) populateFromBulk(ctx context.Context, fund *FundData, code string) {
	rows, navDate, prevDate, err := f.FetchAllFunds(ctx)
	if err != nil {
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

			// Fallback: if today's NAV is not yet available, use previous day's
			if fund.NAV == 0 && fund.PrevNAV > 0 {
				fund.NAV = fund.PrevNAV
				fund.NAVDate = prevDate
				fund.PrevNAV = 0
			}

			return
		}
	}
}

func (f *Fetcher) populatePrevNAV(ctx context.Context, fund *FundData, code string) {
	if fund.PrevNAV != 0 || fund.NAV <= 0 {
		return
	}

	info, err := f.FetchFundInfo(ctx, code)
	if err != nil || len(info.NetWorthTrend) < minFundInfoPoints {
		return
	}

	fund.PrevNAV = info.NetWorthTrend[len(info.NetWorthTrend)-2].Y
	fund.DayChange = fund.NAV - fund.PrevNAV
}

func (f *Fetcher) populateFromFundInfo(ctx context.Context, fund *FundData, code string) {
	if fund.NAV > 0 {
		return
	}

	info, err := f.FetchFundInfo(ctx, code)
	if err != nil || len(info.NetWorthTrend) == 0 {
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
}

func (f *Fetcher) populateFromETF(ctx context.Context, fund *FundData, code string) {
	if fund.NAV != 0 {
		return
	}

	etfRows, err := f.FetchETFData(ctx)
	if err != nil {
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
		return
	}

	gsz, _ := strconv.ParseFloat(fundGZ.GSZ, 64)
	if gsz > 0 {
		fund.LatestNAV = gsz
		fund.LatestTime = fundGZ.GZTime
	}
}
