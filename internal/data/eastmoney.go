package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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

// fetchFundEstimate fetches the latest fund estimate from EastMoney fundgz API.
func (f *Fetcher) fetchFundEstimate(ctx context.Context, code string) (FundGZ, error) {
	url := fmt.Sprintf(fundGZURL, code)

	body, err := f.get(ctx, url, f.defaultHeaders)
	if err != nil {
		return FundGZ{}, fmt.Errorf("fetch fund estimate for %s: %w", code, err)
	}

	return ParseFundGZ(string(body))
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

			if fund.NAV == 0 {
				fund.NAVDate = prevDate
			}

			return
		}
	}

	f.logger.Debug("fund not found in bulk", "code", code)
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
