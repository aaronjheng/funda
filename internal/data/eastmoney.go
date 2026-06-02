package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aaronjheng/funda/internal/eastmoney"
)

func (f *Fetcher) populateFromFundInfo(ctx context.Context, fund *FundData, code string) {
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

	fund.IsQDII = strings.Contains(fund.Name, "QDII")

	fund.NAV = latest.Y
	fund.NAVDate = eastmoney.FormatFundInfoDate(latest.X)

	if len(info.NetWorthTrend) >= eastmoney.MinFundInfoPoints {
		fund.PrevNAV = info.NetWorthTrend[len(info.NetWorthTrend)-2].Y
		fund.DayChange = fund.NAV - fund.PrevNAV
	}

	f.logger.Debug("populated from fund info", "code", code, "nav", fund.NAV)
}

func (f *Fetcher) addEstimate(ctx context.Context, fund *FundData, code string) {
	if f.addETFEstimate(ctx, fund) {
		return
	}

	fundGZ, err := f.eastMoney.FetchFundEstimate(ctx, code)
	if err != nil {
		f.logger.Debug("add estimate failed", "code", code, "error", err)

		return
	}

	f.addFundGZEstimate(fund, fundGZ)

	f.logger.Debug("estimate added", "code", code, "latest_nav", fund.LatestNAV)
}

func (f *Fetcher) addETFEstimate(ctx context.Context, fund *FundData) bool {
	etfRows, err := f.FetchETFData(ctx)
	if err != nil {
		return false
	}

	for _, row := range etfRows {
		if !strings.HasSuffix(row.Symbol, fund.Code) {
			continue
		}

		trade, _ := strconv.ParseFloat(row.Trade, 64)
		if trade > 0 {
			fund.LatestNAV = trade
			fund.LatestTime = time.Now().In(eastmoney.ShanghaiLocation).Format("15:04:05")

			return true
		}
	}

	return false
}

func (f *Fetcher) addFundGZEstimate(fund *FundData, fundGZ eastmoney.FundGZ) {
	if fund.Name == "" {
		fund.Name = fundGZ.Name
		fund.IsQDII = strings.Contains(fund.Name, "QDII")
	}

	gsz, _ := strconv.ParseFloat(fundGZ.GSZ, 64)
	if gsz > 0 {
		fund.LatestNAV = gsz
		fund.LatestTime = fundGZ.GZTime
	}

	if fund.NAV == 0 || fundGZ.JZRQ >= fund.NAVDate {
		dwjz, _ := strconv.ParseFloat(fundGZ.DWJZ, 64)
		if dwjz > 0 {
			fund.NAV = dwjz
			fund.NAVDate = fundGZ.JZRQ

			if fund.PrevNAV > 0 {
				fund.DayChange = fund.NAV - fund.PrevNAV
			}
		}
	}
}
