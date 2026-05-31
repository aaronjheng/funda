package data

import (
	"context"
	"strconv"
	"strings"
	"time"
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
		fund.NAVDate = time.Now().In(shanghaiLoc).Format("2006-01-02")

		f.logger.Debug("populated from etf", "code", code, "nav", fund.NAV)

		return
	}
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
