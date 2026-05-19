package view

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

const (
	fundLabelWidth  = 12
	fundValueOffset = 14
	fundLabelColor  = "245"
	fundBorderColor = "#3b82f6"
	positiveColor   = "#ff6b6b"
	negativeColor   = "#51cf66"
	overlayPaddingX = 2
	overlayWidthSub = 4
)

func RenderFundCard(fundData data.FundData, width int, lastTradingDay time.Time) string {
	contentWidth := max(0, width-cardFrameWidth)

	title := formatFundTitle(fundData)
	navStr := formatNAV(fundData)
	dateStr := formatNAVDate(fundData)

	changeStr, changeSty := formatDayChange(fundData)
	estimateStr, estimateSty := formatEstimate(fundData, lastTradingDay)

	labelStyle := lipgloss.NewStyle().Width(fundLabelWidth).Foreground(lipgloss.Color(fundLabelColor))
	valueMaxWidth := max(0, contentWidth-fundLabelWidth)
	valueStyle := lipgloss.NewStyle().MaxWidth(valueMaxWidth)

	dateRender := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fundLabelColor)).
		Render(dateStr)

	titleLine := lipgloss.NewStyle().Bold(true).MaxWidth(contentWidth).Render(title)

	navLine := labelStyle.Render("最新净值:") +
		valueStyle.Render(navStr+dateRender)

	changeLine := labelStyle.Render("日涨跌:") +
		changeSty.MaxWidth(valueMaxWidth).Render(changeStr)

	lines := []string{
		titleLine,
		navLine,
		changeLine,
	}

	if estimateStr != "" {
		estLabel := labelStyle.Render("实时估值:")
		estimateLine := estLabel + estimateSty.MaxWidth(valueMaxWidth).Render(estimateStr)
		lines = append(lines, estimateLine)
	} else {
		estLabel := labelStyle.Render("实时估值:")
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fundLabelColor))
		lines = append(lines, estLabel+mutedStyle.MaxWidth(valueMaxWidth).Render("--"))
	}

	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(fundBorderColor)).
		Padding(0, 1).
		Width(width).
		Render(content)
}

func formatFundTitle(fundData data.FundData) string {
	if fundData.Alias != "" {
		return fmt.Sprintf("%s (%s)", fundData.Alias, fundData.Code)
	}

	return fmt.Sprintf("%s (%s)", fundData.Name, fundData.Code)
}

func formatNAV(fundData data.FundData) string {
	if fundData.NAV > 0 {
		return fmt.Sprintf("%.4f", fundData.NAV)
	}

	return "--"
}

func formatNAVDate(fundData data.FundData) string {
	if fundData.NAVDate != "" {
		return fmt.Sprintf(" (%s)", fundData.NAVDate)
	}

	return ""
}

func formatDayChange(fundData data.FundData) (string, lipgloss.Style) {
	if fundData.NAV <= 0 || fundData.PrevNAV <= 0 {
		return "--", lipgloss.NewStyle().Foreground(lipgloss.Color(fundLabelColor))
	}

	pct := fundData.DayChangePercent()

	symbol := "+"
	if pct < 0 {
		symbol = ""
	}

	changeStr := fmt.Sprintf("%s%.2f%%", symbol, pct)

	return changeStr, changeStyleFor(pct)
}

func formatEstimate(fundData data.FundData, lastTradingDay time.Time) (string, lipgloss.Style) {
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(fundLabelColor))

	if navIsCurrent(fundData.NAVDate, lastTradingDay) {
		return "", mutedStyle
	}

	if fundData.EstimateNAV <= 0 {
		return "--", mutedStyle
	}

	pct := fundData.EstimateChangePercent()

	symbol := "+"
	if pct < 0 {
		symbol = ""
	}

	estimateStr := fmt.Sprintf("%.4f (%s%.2f%%)", fundData.EstimateNAV, symbol, pct)

	return estimateStr, changeStyleFor(pct)
}

func changeStyleFor(pct float64) lipgloss.Style {
	switch {
	case pct > 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(positiveColor))
	case pct < 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(negativeColor))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(fundLabelColor))
	}
}

func navIsCurrent(navDate string, lastTradingDay time.Time) bool {
	if navDate == "" {
		return false
	}

	d, err := time.Parse("2006-01-02", navDate)
	if err != nil {
		return false
	}

	return !d.Before(lastTradingDay)
}

func RenderGroupSelector(groups []config.Group, selectedIdx int, width int) string {
	if len(groups) == 0 {
		return ""
	}

	var parts []string

	for idx, group := range groups {
		label := fmt.Sprintf("%s (%d)", group.Name, len(group.Funds))

		if idx == selectedIdx {
			highlight := lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Render(label)
			parts = append(parts, highlight)
		} else {
			muted := lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Render(label)
			parts = append(parts, muted)
		}
	}

	content := strings.Join(parts, "  ")

	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}

func RenderFooter(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Align(lipgloss.Center).
		Width(width).
		Render("r refresh | s search | ↑/↓ scroll | ←/→ group | q quit")
}

func RenderStatusBar(msg string, width int, isError bool) string {
	if msg == "" {
		return ""
	}

	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if isError {
		style = style.Foreground(lipgloss.Color("#f85149"))
	} else {
		style = style.Foreground(lipgloss.Color("245"))
	}

	return style.Render(msg)
}

func RenderScrollbar(offset, total, visible, height int) string {
	if total <= visible || height <= 0 {
		return ""
	}

	thumbSize := min(max(1, height*visible/total), height)

	thumbPos := 0
	if total > visible {
		thumbPos = offset * (height - thumbSize) / (total - visible)
	}

	var builder strings.Builder

	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))

	for idx := range height {
		if idx >= thumbPos && idx < thumbPos+thumbSize {
			builder.WriteString(thumbStyle.Render("┃"))
		} else {
			builder.WriteString(trackStyle.Render("│"))
		}

		if idx < height-1 {
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func RenderSearchOverlay(
	query string,
	results []data.SearchHit,
	cursor int,
	width int,
) string {
	var builder strings.Builder

	builder.WriteString(lipgloss.NewStyle().Bold(true).Render("Search: "))
	builder.WriteString(query)
	builder.WriteString("_\n\n")

	for idx, result := range results {
		line := fmt.Sprintf("  %s %s  %s  %s",
			result.Code, result.Name, result.Price, result.Change)

		if idx == cursor {
			highlight := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Render("> " + line + "\n")
			builder.WriteString(highlight)
		} else {
			muted := lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Render("  " + line + "\n")
			builder.WriteString(muted)
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, overlayPaddingX).
		Width(width - overlayWidthSub).
		Render(builder.String())
}
