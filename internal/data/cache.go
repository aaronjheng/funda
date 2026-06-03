package data

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adrg/xdg"
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
	tradingCacheDuration    = 5 * time.Minute
	navPublishCacheDuration = 30 * time.Minute
	offHoursCacheDuration   = 12 * time.Hour
	cacheDirPermissions     = 0o700
	cacheFilePermissions    = 0o600
)

// MemoryCache provides a thread-safe in-memory cache with TTL.
type MemoryCache struct {
	mu     sync.RWMutex
	items  map[string]cacheEntry
	logger *slog.Logger
}

type cacheEntry struct {
	data      FundData
	timestamp time.Time
}

// NewMemoryCache creates a new memory cache.
func NewMemoryCache(logger *slog.Logger) *MemoryCache {
	return &MemoryCache{mu: sync.RWMutex{}, items: make(map[string]cacheEntry), logger: logger}
}

// Get retrieves a cached FundData if not expired.
func (c *MemoryCache) Get(code string) (FundData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[code]
	if !ok {
		c.logger.Debug("memory cache miss", "code", code)

		var empty FundData

		return empty, false
	}

	if time.Since(entry.timestamp) > cacheTTL() {
		c.logger.Debug("memory cache expired", "code", code)

		var empty FundData

		return empty, false
	}

	c.logger.Debug("memory cache hit", "code", code)

	return entry.data, true
}

// Remove deletes a specific entry from the memory cache.
func (c *MemoryCache) Remove(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, code)
	c.logger.Debug("memory cache removed", "code", code)
}

// Clear removes all entries from the memory cache.
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.items)
	c.items = make(map[string]cacheEntry)

	c.logger.Debug("memory cache cleared", "count", count)
}

// Set stores FundData in the cache.
func (c *MemoryCache) Set(code string, data FundData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[code] = cacheEntry{data: data, timestamp: time.Now()}
	c.logger.Debug("memory cache set", "code", code)
}

// cacheTTL returns the appropriate TTL based on trading hours.
func cacheTTL() time.Duration {
	now := time.Now().In(shanghaiLoc)
	if IsTradingHours(now) {
		return tradingCacheDuration
	}

	if IsTradingDay(now) {
		hour := now.Hour()
		// After market close (15:00-22:00), refresh every 30 min to pick up NAV publications.
		if hour >= 15 && hour < 22 {
			return navPublishCacheDuration
		}
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

// GetLastTradingDate returns the most recent trading day on or before t, at midnight.
func GetLastTradingDate(t time.Time) time.Time {
	local := t.In(shanghaiLoc)
	for !IsTradingDay(local) {
		local = local.AddDate(0, 0, -1)
	}

	y, m, d := local.Date()

	return time.Date(y, m, d, 0, 0, 0, 0, shanghaiLoc)
}

// NavIsCurrent reports whether the NAV date is on or after the last trading day.
func NavIsCurrent(navDate string, lastTradingDay time.Time) bool {
	if navDate == "" {
		return false
	}

	nav, err := time.Parse("2006-01-02", navDate)
	if err != nil {
		return false
	}

	return !nav.Before(lastTradingDay)
}

func cacheDir() string {
	dir := filepath.Join(xdg.CacheHome, "funda", "fund_data")

	_ = os.MkdirAll(dir, cacheDirPermissions)

	return dir
}

// LoadFundCache loads cached FundData from disk for a specific fund code.
func LoadFundCache(logger *slog.Logger, code string) (FundData, bool) {
	path := filepath.Join(cacheDir(), code+".json")

	info, err := os.Stat(path)
	if err != nil {
		logger.Debug("disk cache miss (no file)", "code", code)

		var empty FundData

		return empty, false
	}

	if time.Since(info.ModTime()) > cacheTTL() {
		logger.Debug("disk cache expired", "code", code)

		var empty FundData

		return empty, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("disk cache read error", "code", code, "error", err)

		var empty FundData

		return empty, false
	}

	var fundData FundData

	err = json.Unmarshal(data, &fundData)
	if err != nil {
		logger.Warn("disk cache unmarshal error", "code", code, "error", err)

		var empty FundData

		return empty, false
	}

	logger.Debug("disk cache hit", "code", code)

	return fundData, true
}

// LoadFundCacheIgnoreTTL loads cached FundData from disk regardless of TTL.
// It is used on cold start to show stale data before fresh data arrives.
func LoadFundCacheIgnoreTTL(logger *slog.Logger, code string) (FundData, bool) {
	path := filepath.Join(cacheDir(), code+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Debug("disk cache ignore-TTL miss", "code", code)

		var empty FundData

		return empty, false
	}

	var fundData FundData

	err = json.Unmarshal(data, &fundData)
	if err != nil {
		logger.Warn("disk cache ignore-TTL unmarshal error", "code", code, "error", err)

		var empty FundData

		return empty, false
	}

	logger.Debug("disk cache ignore-TTL hit", "code", code)

	return fundData, true
}

// ClearFundCache removes all cached fund data files from disk.
func ClearFundCache(logger *slog.Logger) {
	dir := cacheDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("disk cache clear read dir error", "error", err)

		return
	}

	count := 0

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
			count++
		}
	}

	logger.Info("disk cache cleared", "count", count)
}

// DeleteFundCache removes the cached FundData file from disk.
func DeleteFundCache(logger *slog.Logger, code string) {
	path := filepath.Join(cacheDir(), code+".json")
	_ = os.Remove(path)

	logger.Debug("disk cache deleted", "code", code)
}

// SaveFundCache saves FundData to disk for a specific fund code.
func SaveFundCache(logger *slog.Logger, fundData FundData) {
	if fundData.NAV == 0 {
		return
	}

	path := filepath.Join(cacheDir(), fundData.Code+".json")

	data, err := json.MarshalIndent(fundData, "", "  ")
	if err != nil {
		logger.Warn("disk cache marshal error", "code", fundData.Code, "error", err)

		return
	}

	_ = os.WriteFile(path, data, cacheFilePermissions)

	logger.Debug("disk cache saved", "code", fundData.Code)
}
