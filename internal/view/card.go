package view

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func cardCacheKey(
	fund config.Fund,
	fundData data.FundData,
	cardWidth int,
	lastTradingDay time.Time,
	highlighted bool,
) string {
	return fmt.Sprintf("%s|%d|%s|%s|%v|%v|%s|%v|%s|%s|%v",
		fund.Code,
		cardWidth,
		fundData.Name,
		fundData.Alias,
		fundData.NAV,
		fundData.PrevNAV,
		fundData.NAVDate,
		fundData.LatestNAV,
		fundData.LatestTime,
		lastTradingDay.Format("2006-01-02"),
		highlighted,
	)
}

func (m Model) renderFundRows(
	funds []config.Fund,
	cardWidth int,
	lastTradingDay time.Time,
) []string {
	var rows []string

	for idx := 0; idx < len(funds); idx += cardsPerRow {
		pair := m.renderFundPair(funds, idx, cardWidth, lastTradingDay)

		if len(pair) == cardsPerRow {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, pair[0], " ", pair[1]))
		} else {
			rows = append(rows, pair[0])
		}
	}

	return rows
}

func (m Model) renderFundPair(
	funds []config.Fund,
	startIdx int,
	cardWidth int,
	lastTradingDay time.Time,
) []string {
	var pair []string

	for col := 0; col < cardsPerRow && startIdx+col < len(funds); col++ {
		fund := funds[startIdx+col]

		fundData := m.fundData[fund.Code]
		if fundData.Code == "" {
			fundData.Code = fund.Code
			fundData.Alias = fund.Alias
		}

		cacheKey := cardCacheKey(fund, fundData, cardWidth, lastTradingDay, fund.Code == m.copiedCode)
		if cached, ok := m.cardCache[cacheKey]; ok {
			pair = append(pair, cached)

			continue
		}

		card := RenderFundCard(fundData, cardWidth, lastTradingDay, fund.Code == m.copiedCode)
		m.cardCache[cacheKey] = card
		pair = append(pair, card)
	}

	return pair
}

func (m Model) renderFundsContent(cardWidth int, lastTradingDay time.Time) string {
	if len(m.sortedFunds) == 0 {
		return lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			Render("No funds in this group")
	}

	rows := m.renderFundRows(m.sortedFunds, cardWidth, lastTradingDay)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) computeCardWidth() int {
	cardWidth := (m.width - cardPaddingWidth) / cardsPerRow
	if cardWidth < minCardWidth {
		cardWidth = m.width - cardPaddingWidth
	}

	return cardWidth
}
