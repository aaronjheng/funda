package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

//nolint:gochecknoglobals // timezone lookup is immutable and needed across the package
var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}

	return loc
}()

// ShanghaiLocation returns the Asia/Shanghai time location for use in tests.
func ShanghaiLocation() *time.Location {
	return shanghaiLoc
}

const (
	tradingCacheDuration  = 5 * time.Minute
	offHoursCacheDuration = 12 * time.Hour
	etfCacheDuration      = 60 * time.Second
	cacheDirPermissions   = 0o755
	cacheFilePermissions  = 0o600
)

// MemoryCache provides a thread-safe in-memory cache with TTL.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	data      FundData
	timestamp time.Time
}

// NewMemoryCache creates a new memory cache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{mu: sync.RWMutex{}, items: make(map[string]cacheEntry)}
}

// Get retrieves a cached FundData if not expired.
func (c *MemoryCache) Get(code string) (FundData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[code]
	if !ok {
		return FundData{}, false //nolint:exhaustruct // zero-value return for cache miss
	}

	if time.Since(entry.timestamp) > cacheTTL() {
		return FundData{}, false //nolint:exhaustruct // zero-value return for expired entry
	}

	return entry.data, true
}

// Set stores FundData in the cache.
func (c *MemoryCache) Set(code string, data FundData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[code] = cacheEntry{data: data, timestamp: time.Now()}
}

// cacheTTL returns the appropriate TTL based on trading hours.
func cacheTTL() time.Duration {
	now := time.Now().In(shanghaiLoc)
	if IsTradingHours(now) {
		return tradingCacheDuration
	}

	return offHoursCacheDuration
}

// IsTradingDay reports whether t is a weekday.
func IsTradingDay(t time.Time) bool {
	wd := t.In(shanghaiLoc).Weekday()

	return wd != time.Saturday && wd != time.Sunday
}

// IsTradingHours reports whether t is within A-share trading hours (9-15 on weekdays).
func IsTradingHours(t time.Time) bool {
	local := t.In(shanghaiLoc)
	if !IsTradingDay(local) {
		return false
	}

	hour := local.Hour()

	return hour >= 9 && hour < 15
}

// GetLastTradingDate returns the most recent trading day on or before t.
func GetLastTradingDate(t time.Time) time.Time {
	local := t.In(shanghaiLoc)
	for !IsTradingDay(local) {
		local = local.AddDate(0, 0, -1)
	}

	return local
}

func cacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			base = filepath.Join(home, ".cache")
		}
	}

	dir := filepath.Join(base, "funda", "fund_data")

	_ = os.MkdirAll(dir, cacheDirPermissions) //nolint:gosec // cache directory needs read/exec for user

	return dir
}

// LoadFundCache loads cached FundData from disk for a specific fund code.
func LoadFundCache(code string) (FundData, bool) {
	path := filepath.Join(cacheDir(), code+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return FundData{}, false //nolint:exhaustruct // zero-value return for read failure
	}

	var fundData FundData

	err = json.Unmarshal(data, &fundData)
	if err != nil {
		return FundData{}, false //nolint:exhaustruct // zero-value return for unmarshal failure
	}

	return fundData, true
}

// SaveFundCache saves FundData to disk for a specific fund code.
func SaveFundCache(fundData FundData) {
	if fundData.NAV == 0 {
		return
	}

	path := filepath.Join(cacheDir(), fundData.Code+".json")

	data, err := json.MarshalIndent(fundData, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(path, data, cacheFilePermissions)
}

// ETFTickerCache provides a short-lived cache for ETF bulk data.
type ETFTickerCache struct {
	mu        sync.RWMutex
	data      []ETFRow
	timestamp time.Time
}

// ETFRow represents a single ETF entry from Sina.
type ETFRow struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Trade         string `json:"trade"`
	Settlement    string `json:"settlement"`
	ChangePercent string `json:"changepercent"`
}

// Get returns cached ETF data if within TTL.
func (c *ETFTickerCache) Get() ([]ETFRow, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data == nil || time.Since(c.timestamp) > etfCacheDuration {
		return nil, false
	}

	return c.data, true
}

// Set stores ETF data with current timestamp.
func (c *ETFTickerCache) Set(data []ETFRow) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = data
	c.timestamp = time.Now()
}
