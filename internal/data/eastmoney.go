package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aaronjheng/funda/internal/eastmoney"
)

func (f *Fetcher) populateFromBulk(ctx context.Context, fund *FundData, code string) {
	rows, navDate, prevDate, err := f.eastMoney.FetchBulk(ctx)
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

	info, err := f.eastMoney.FetchFundInfo(ctx, code)
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
	fund.NAVDate = eastmoney.FormatFundInfoDate(latest.X)

	if len(info.NetWorthTrend) >= eastmoney.MinFundInfoPoints {
		fund.PrevNAV = info.NetWorthTrend[len(info.NetWorthTrend)-2].Y
		fund.DayChange = fund.NAV - fund.PrevNAV
	}

	f.logger.Debug("populated from fund info", "code", code, "nav", fund.NAV)
}

func (f *Fetcher) populatePrevNAV(ctx context.Context, fund *FundData, code string) {
	if fund.PrevNAV != 0 || fund.NAV <= 0 {
		return
	}

	info, err := f.eastMoney.FetchFundInfo(ctx, code)
	if err != nil || len(info.NetWorthTrend) < eastmoney.MinFundInfoPoints {
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
					fund.LatestTime = time.Now().In(eastmoney.ShanghaiLocation).Format("15:04:05")

					return
				}
			}
		}
	}

	fundGZ, err := f.eastMoney.FetchFundEstimate(ctx, code)
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
