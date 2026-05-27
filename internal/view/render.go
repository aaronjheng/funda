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
	fundLabelWidth   = 12
	fundValueOffset  = 14
	primaryColor     = "#e2e8f0"
	secondaryColor   = "#64748b"
	borderColor      = "#334155"
	positiveColor    = "#ff6b6b"
	negativeColor    = "#51cf66"
	accentColor      = "#60a5fa"
	overlayPaddingX  = 2
	overlayWidthSub  = 4
	tabCenterDivisor = 2
)

func RenderFundCard(fundData data.FundData, width int, lastTradingDay time.Time, highlighted bool) string {
	contentWidth := max(0, width-cardFrameWidth)

	title := formatFundTitle(fundData)
	navStr := formatNAV(fundData)
	dateStr := formatNAVDate(fundData)

	changeStr, changeSty := formatDayChange(fundData)
	estimateStr, estimateSty := formatEstimate(fundData, lastTradingDay)

	labelStyle := lipgloss.NewStyle().Width(fundLabelWidth).Foreground(lipgloss.Color(secondaryColor))
	valueMaxWidth := max(0, contentWidth-fundLabelWidth)
	valueStyle := lipgloss.NewStyle().MaxWidth(valueMaxWidth)

	dateRender := lipgloss.NewStyle().
		Foreground(lipgloss.Color(secondaryColor)).
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
		estLabel := labelStyle.Render("最新估值:")
		estimateLine := estLabel + estimateSty.MaxWidth(valueMaxWidth).Render(estimateStr)
		lines = append(lines, estimateLine)
	} else {
		estLabel := labelStyle.Render("最新估值:")
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
		lines = append(lines, estLabel+mutedStyle.MaxWidth(valueMaxWidth).Render("--"))
	}

	content := strings.Join(lines, "\n")

	borderCol := lipgloss.Color(borderColor)
	if highlighted {
		borderCol = lipgloss.Color(accentColor)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderCol).
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
		return "--", lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
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
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))

	if navIsCurrent(fundData.NAVDate, lastTradingDay) {
		return "", mutedStyle
	}

	if fundData.LatestNAV <= 0 {
		return "--", mutedStyle
	}

	pct := fundData.LatestChangePercent()

	symbol := "+"
	if pct < 0 {
		symbol = ""
	}

	estimateStr := fmt.Sprintf("%.4f (%s%.2f%%)", fundData.LatestNAV, symbol, pct)

	if fundData.LatestTime != "" {
		muted := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor)).Render(" " + fundData.LatestTime)
		estimateStr += muted
	}

	return estimateStr, changeStyleFor(pct)
}

func changeStyleFor(pct float64) lipgloss.Style {
	switch {
	case pct > 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(positiveColor))
	case pct < 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(negativeColor))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
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

type GroupTabBounds struct {
	Index  int
	StartX int
	EndX   int
}

type tabInfo struct {
	raw   string
	width int
}

func renderGroupTabs(groups []config.Group, selectedIdx int) ([]tabInfo, int) {
	tabs := make([]tabInfo, len(groups))
	totalWidth := 0

	for idx, group := range groups {
		label := fmt.Sprintf("%s (%d)", group.Name, len(group.Funds))

		var rendered string
		if idx == selectedIdx {
			rendered = lipgloss.NewStyle().
				Foreground(lipgloss.Color(primaryColor)).
				Render(label)
		} else {
			rendered = lipgloss.NewStyle().
				Foreground(lipgloss.Color(secondaryColor)).
				Render(label)
		}

		w := lipgloss.Width(rendered)
		tabs[idx] = tabInfo{raw: rendered, width: w}
		totalWidth += w
	}

	return tabs, totalWidth
}

func RenderGroupSelector(groups []config.Group, selectedIdx int, width int) (string, []GroupTabBounds) {
	if len(groups) == 0 {
		return "", nil
	}

	tabs, totalWidth := renderGroupTabs(groups, selectedIdx)
	gap := 2
	totalWidth += gap * (len(groups) - 1)

	startX := 0
	if width > totalWidth {
		startX = (width - totalWidth) / tabCenterDivisor
	}

	bounds := make([]GroupTabBounds, 0, len(tabs))

	var builder strings.Builder

	builder.WriteString(strings.Repeat(" ", startX))

	currentX := startX

	for idx, tab := range tabs {
		bounds = append(bounds, GroupTabBounds{
			Index:  idx,
			StartX: currentX,
			EndX:   currentX + tab.width,
		})
		builder.WriteString(tab.raw)
		currentX += tab.width

		if idx < len(tabs)-1 {
			builder.WriteString(strings.Repeat(" ", gap))
			currentX += gap
		}
	}

	content := builder.String()
	rendered := lipgloss.NewStyle().Width(width).Render(content)

	return rendered, bounds
}

func RenderFooter(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(secondaryColor)).
		Align(lipgloss.Center).
		Width(width).
		Render("r refresh | R reload | s search | o sort | c clear cache | click copy | ↑/↓ scroll | ←/→ group | q quit")
}

func RenderToast(msg string) string {
	if msg == "" {
		return ""
	}

	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color(primaryColor)).
		Bold(true).
		Render(msg)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accentColor)).
		Padding(0, 1).
		Render(content)
}

func RenderStatusBar(msg string, width int, isError bool) string {
	if msg == "" {
		return ""
	}

	style := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if isError {
		style = style.Foreground(lipgloss.Color(positiveColor))
	} else {
		style = style.Foreground(lipgloss.Color(secondaryColor))
	}

	return style.Render(msg)
}

func RenderScrollbar(offset, totalLines, trackHeight int) string {
	if totalLines <= trackHeight || trackHeight <= 0 {
		return ""
	}

	thumbSize := min(max(1, trackHeight*trackHeight/totalLines), trackHeight)

	thumbPos := 0
	if totalLines > trackHeight {
		thumbPos = offset * (trackHeight - thumbSize) / (totalLines - trackHeight)
	}

	var builder strings.Builder

	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))

	for idx := range trackHeight {
		if idx >= thumbPos && idx < thumbPos+thumbSize {
			builder.WriteString(thumbStyle.Render("┃"))
		} else {
			builder.WriteString(trackStyle.Render("│"))
		}

		if idx < trackHeight-1 {
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
				Foreground(lipgloss.Color(primaryColor)).
				Render("> " + line + "\n")
			builder.WriteString(highlight)
		} else {
			muted := lipgloss.NewStyle().
				Foreground(lipgloss.Color(secondaryColor)).
				Render("  " + line + "\n")
			builder.WriteString(muted)
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(accentColor)).
		Padding(1, overlayPaddingX).
		Width(width - overlayWidthSub).
		Render(builder.String())
}
