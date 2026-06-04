package data

import (
	"context"
	"log/slog"
	"runtime"
	"strconv"
	"sync"

	"github.com/aaronjheng/funda/internal/eastmoney"
)

// Fetcher handles HTTP requests and caching for fund data.
type Fetcher struct {
	eastMoney eastmoney.Client
	sem       chan struct{}
	memCache  *MemoryCache
	logger    *slog.Logger
}

// NewFetcher creates a new Fetcher with injected API clients.
func NewFetcher(eastMoney eastmoney.Client, logger *slog.Logger) *Fetcher {
	return &Fetcher{
		eastMoney: eastMoney,
		sem:       make(chan struct{}, runtime.NumCPU()),
		memCache:  NewMemoryCache(logger),
		logger:    logger,
	}
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

			latestNAV, latestTime := f.refreshEstimate(ctx, fundCode)

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

	f.populateFromFundInfo(ctx, &fund, code)

	f.addEstimate(ctx, &fund, code)

	if fund.NAV == 0 && fund.PrevNAV > 0 {
		fund.NAV = fund.PrevNAV
		fund.PrevNAV = 0

		f.logger.Debug("using prevnav as nav fallback", "code", code)
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

	ClearFundCache(f.logger)
}

// RemoveCachedEntries removes only the specified fund codes from both memory and disk cache.
func (f *Fetcher) RemoveCachedEntries(codes []string) {
	f.logger.Info("removing cached entries", "count", len(codes))

	for _, code := range codes {
		f.memCache.Remove(code)
		DeleteFundCache(f.logger, code)
	}
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

func (f *Fetcher) refreshEstimate(ctx context.Context, code string) (float64, string) {
	fundGZ, err := f.eastMoney.FetchFundEstimate(ctx, code)
	if err != nil {
		f.logger.Debug("estimate refresh failed", "code", code, "error", err)

		return 0, ""
	}

	gsz, _ := strconv.ParseFloat(fundGZ.GSZ, 64)

	return gsz, fundGZ.GZTime
}
