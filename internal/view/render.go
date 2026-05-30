package view

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

const (
	fundLabelWidth = 12
	primaryColor   = "#e2e8f0"
	secondaryColor = "#64748b"
	borderColor    = "#334155"
	positiveColor  = "#ff6b6b"
	negativeColor  = "#51cf66"
	accentColor    = "#60a5fa"
	toastMaxWidth  = 60
)

func RenderFundCard(fundData data.FundData, width int, lastTradingDay time.Time, highlighted bool) string {
	contentWidth := max(0, width-cardFrameWidth)

	title := formatFundTitle(fundData)
	navStr := formatNAV(fundData)

	changeStr, changeSty := formatDayChange(fundData)
	estimateStr, estimateSty := formatEstimate(fundData, lastTradingDay)

	labelStyle := lipgloss.NewStyle().Width(fundLabelWidth).Foreground(lipgloss.Color(secondaryColor))
	valueMaxWidth := max(0, contentWidth-fundLabelWidth)
	valueStyle := lipgloss.NewStyle().MaxWidth(valueMaxWidth)

	titleLine := lipgloss.NewStyle().Bold(true).MaxWidth(contentWidth).Render(title)

	navValue := changeSty.Render(navStr + " (" + changeStr + ")")
	navLine := labelStyle.Render("最新净值:") +
		valueStyle.MaxWidth(valueMaxWidth).Render(navValue)

	indent := strings.Repeat(" ", fundLabelWidth)
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))

	dateLine := indent
	if fundData.NAVDate != "" {
		dateLine += dateStyle.Render(fundData.NAVDate)
	}

	lines := make([]string, 0, fundCardContentLines)
	lines = append(lines, titleLine, navLine, dateLine)

	estimateLine, timeLine := formatEstimateLines(estimateStr, estimateSty, labelStyle, valueMaxWidth, fundData.LatestTime)
	lines = append(lines, estimateLine, timeLine)

	content := strings.Join(lines, "\n")

	borderCol := lipgloss.Color(borderColor)
	if highlighted {
		borderCol = lipgloss.Color(accentColor)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
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

	if data.NavIsCurrent(fundData.NAVDate, lastTradingDay) {
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

	return estimateStr, changeStyleFor(pct)
}

func formatEstimateLines(
	estimateStr string,
	estimateSty lipgloss.Style,
	labelStyle lipgloss.Style,
	valueMaxWidth int,
	latestTime string,
) (string, string) {
	var estimateLine string

	if estimateStr != "" && estimateStr != "--" {
		estLabel := labelStyle.Render("最新估值:")
		estimateLine = estLabel + estimateSty.MaxWidth(valueMaxWidth).Render(estimateStr)
	} else {
		estLabel := labelStyle.Render("最新估值:")
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
		estimateLine = estLabel + mutedStyle.MaxWidth(valueMaxWidth).Render("-- (--)")
	}

	var timeLine string

	if estimateStr != "" && latestTime != "" {
		indent := strings.Repeat(" ", fundLabelWidth)
		timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))

		if strings.Count(latestTime, ":") == 1 {
			latestTime += ":00"
		}

		timeLine = indent + timeStyle.Render(latestTime)
	} else {
		indent := strings.Repeat(" ", fundLabelWidth)
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
		timeLine = indent + mutedStyle.Render("--")
	}

	return estimateLine, timeLine
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

type GroupTabBounds struct {
	Index  int
	StartX int
	EndX   int
}

func tabStyles() (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	activeBorder := lipgloss.Border{
		Top:         "─",
		Bottom:      " ",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┘",
		BottomRight: "└",
	}

	inactiveBorder := lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "┴",
		BottomRight: "┴",
	}

	tabSty := lipgloss.NewStyle().
		Border(inactiveBorder, true).
		BorderForeground(lipgloss.Color(borderColor)).
		Foreground(lipgloss.Color(secondaryColor)).
		Padding(0, 1)

	activeSty := lipgloss.NewStyle().
		Border(activeBorder, true).
		BorderForeground(lipgloss.Color(primaryColor)).
		Foreground(lipgloss.Color(primaryColor)).
		Bold(true).
		Padding(0, 1)

	gapSty := lipgloss.NewStyle().
		Border(inactiveBorder, false, false, true, false).
		BorderForeground(lipgloss.Color(borderColor))

	return activeSty, tabSty, gapSty
}

func RenderGroupSelector(groups []config.Group, selectedIdx int, width int) (string, []GroupTabBounds) {
	if len(groups) == 0 {
		return "", nil
	}

	activeSty, tabSty, gapSty := tabStyles()

	var renderedTabs []string

	var tabWidths []int

	totalTabWidth := 0

	for idx, group := range groups {
		label := fmt.Sprintf("%s (%d)", group.Name, len(group.Funds))

		var tab string
		if idx == selectedIdx {
			tab = activeSty.Render(label)
		} else {
			tab = tabSty.Render(label)
		}

		tabW := lipgloss.Width(tab)
		renderedTabs = append(renderedTabs, tab)
		tabWidths = append(tabWidths, tabW)
		totalTabWidth += tabW
	}

	row := lipgloss.JoinHorizontal(lipgloss.Bottom, renderedTabs...)

	remaining := max(0, width-totalTabWidth)
	if remaining > 0 {
		gap := gapSty.Render(strings.Repeat(" ", remaining))
		row = lipgloss.JoinHorizontal(lipgloss.Bottom, row, gap)
	}

	var bounds []GroupTabBounds

	currentX := 0
	for idx, tabW := range tabWidths {
		bounds = append(bounds, GroupTabBounds{
			Index:  idx,
			StartX: currentX,
			EndX:   currentX + tabW,
		})
		currentX += tabW
	}

	rendered := lipgloss.NewStyle().Width(width).Render(row)

	return rendered, bounds
}

func truncateWidth(str string, maxWidth int) string {
	if lipgloss.Width(str) <= maxWidth {
		return str
	}

	var (
		out   strings.Builder
		width int
	)

	for _, char := range str {
		charWidth := lipgloss.Width(string(char))
		if width+charWidth > maxWidth-1 {
			out.WriteString("\u2026")

			break
		}

		out.WriteRune(char)

		width += charWidth
	}

	return out.String()
}

func RenderStatusBar(left, right string, width int, isError bool) string {
	leftSty := lipgloss.NewStyle().PaddingLeft(1)

	rightSty := lipgloss.NewStyle().
		Align(lipgloss.Right).
		PaddingRight(1)

	if isError {
		leftSty = leftSty.Foreground(lipgloss.Color(positiveColor))
		rightSty = rightSty.Foreground(lipgloss.Color(positiveColor))
	} else {
		leftSty = leftSty.Foreground(lipgloss.Color(secondaryColor))
		rightSty = rightSty.Foreground(lipgloss.Color(secondaryColor))
	}

	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(borderColor)).
		Width(width).
		Render(strings.Repeat("\u2500", width))

	const paddingReserve = 2

	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right)-paddingReserve)
	content := leftSty.Render(left) + strings.Repeat(" ", gap) + rightSty.Render(right)

	return lipgloss.JoinVertical(lipgloss.Left, sep, content)
}

func RenderScrollbar(view viewport.Model) string {
	total := view.TotalLineCount()
	visible := view.VisibleLineCount()

	if total <= visible || visible <= 0 {
		return ""
	}

	yOffset := view.YOffset()
	thumbSize := max(1, visible*visible/total)

	maxThumbPos := visible - thumbSize

	thumbPos := 0
	if total > visible {
		thumbPos = yOffset * maxThumbPos / (total - visible)
	}

	var builder strings.Builder

	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(secondaryColor))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))

	for line := range visible {
		if line >= thumbPos && line < thumbPos+thumbSize {
			builder.WriteString(thumbStyle.Render("\u2503"))
		} else {
			builder.WriteString(trackStyle.Render("\u2502"))
		}

		if line < visible-1 {
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}
