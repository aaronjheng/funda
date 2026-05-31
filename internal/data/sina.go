package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aaronjheng/funda/internal/eastmoney"
)

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
		fund.NAVDate = time.Now().In(eastmoney.ShanghaiLocation).Format("2006-01-02")

		f.logger.Debug("populated from etf", "code", code, "nav", fund.NAV)

		return
	}
}
