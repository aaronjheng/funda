package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	helpPanelMaxWidth    = 72
	helpPanelScreenInset = 4
	helpPanelBorderWidth = 2
	helpPanelFrameWidth  = helpPanelBorderWidth + helpPaddingHorizontal*2
	helpWideContentWidth = 58
	helpSectionsPerRow   = 2
	helpColumnGapWidth   = 4
	helpKeyWidth         = 10
	helpStatusSeparator  = "   "
)

type helpItem struct {
	Key   string
	Label string
}

type helpSection struct {
	Title string
	Items []helpItem
}

func (m Model) renderHelpPanel() string {
	panelWidth := m.helpPanelWidth()
	contentWidth := max(0, panelWidth-helpPanelFrameWidth)
	content := m.renderHelpContent(contentWidth)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.colors.accent)).
		Padding(1, helpPaddingHorizontal).
		Width(panelWidth).
		Render(content)
}

func (m Model) helpPanelWidth() int {
	availableWidth := max(1, m.width-helpPanelScreenInset)

	return min(helpPanelMaxWidth, availableWidth)
}

func (m Model) renderHelpContent(width int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(m.colors.accent)).
		Width(width).
		Render("快捷键")

	status := m.renderHelpStatus(width)
	sections := renderHelpSections(m.helpSections(), width, m.colors)

	return lipgloss.JoinVertical(lipgloss.Left, title, status, "", sections)
}

func (m Model) renderHelpStatus(width int) string {
	parts := []string{
		m.helpGroupSummary(),
		"排序: " + m.helpSortSummary(),
	}

	if m.loading {
		parts = append(parts, "刷新中")
	}

	if m.hasUnsavedFunds {
		parts = append(parts, "配置未保存")
	}

	line := truncateWidth(strings.Join(parts, helpStatusSeparator), width)

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.colors.secondary)).
		Width(width).
		Render(line)
}

func (m Model) helpGroupSummary() string {
	if len(m.groups) == 0 || m.currentGroup < 0 || m.currentGroup >= len(m.groups) {
		return "组: --"
	}

	group := m.groups[m.currentGroup]

	return fmt.Sprintf("组: %s (%d)", group.Name, len(group.Funds))
}

func (m Model) helpSortSummary() string {
	if m.sortField == SortDefault {
		return "默认"
	}

	direction := "降序"
	if m.sortAsc {
		direction = "升序"
	}

	return fmt.Sprintf("%s %s", m.sortFieldLabel(), direction)
}

func (m Model) helpSections() []helpSection {
	return []helpSection{
		{
			Title: "导航",
			Items: []helpItem{
				{Key: "↑/k", Label: "上滚一行"},
				{Key: "↓/j", Label: "下滚一行"},
				{Key: "PgUp", Label: "上翻页"},
				{Key: "PgDn", Label: "下翻页"},
				{Key: "←/h", Label: "上一组"},
				{Key: "→/l", Label: "下一组"},
			},
		},
		{
			Title: "数据",
			Items: []helpItem{
				{Key: "r", Label: "刷新当前组"},
				{Key: "R", Label: "重载配置"},
				{Key: "c", Label: "清缓存"},
			},
		},
		{
			Title: "排序",
			Items: []helpItem{
				{Key: "o", Label: "切换排序字段"},
				{Key: "O", Label: "切换升降序"},
			},
		},
		{
			Title: "鼠标 / 其他",
			Items: []helpItem{
				{Key: "点击组", Label: "切换分组"},
				{Key: "点击基金", Label: "复制代码"},
				{Key: "?/Esc", Label: "关闭帮助"},
				{Key: "q/Ctrl+C", Label: "退出"},
			},
		},
	}
}

func renderHelpSections(sections []helpSection, width int, colors themeColors) string {
	if width >= helpWideContentWidth {
		return renderWideHelpSections(sections, width, colors)
	}

	return renderNarrowHelpSections(sections, width, colors)
}

func renderWideHelpSections(sections []helpSection, width int, colors themeColors) string {
	columnWidth := max(0, (width-helpColumnGapWidth)/helpSectionsPerRow)
	gap := strings.Repeat(" ", helpColumnGapWidth)
	rows := make([]string, 0, (len(sections)+1)/helpSectionsPerRow)

	for idx := 0; idx < len(sections); idx += helpSectionsPerRow {
		left := renderHelpSection(sections[idx], columnWidth, colors)
		if idx+1 >= len(sections) {
			rows = append(rows, left)

			continue
		}

		right := renderHelpSection(sections[idx+1], columnWidth, colors)
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func renderNarrowHelpSections(sections []helpSection, width int, colors themeColors) string {
	rows := make([]string, 0, len(sections))

	for _, section := range sections {
		rows = append(rows, renderHelpSection(section, width, colors))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func renderHelpSection(section helpSection, width int, colors themeColors) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colors.accent)).
		Width(width)
	rows := make([]string, 0, len(section.Items)+1)
	rows = append(rows, titleStyle.Render(section.Title))

	for _, item := range section.Items {
		rows = append(rows, renderHelpItem(item, width, colors))
	}

	return lipgloss.NewStyle().
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func renderHelpItem(item helpItem, width int, colors themeColors) string {
	if width <= helpKeyWidth {
		return truncateWidth(item.Key, width)
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors.accent)).
		Width(helpKeyWidth)
	labelWidth := max(0, width-helpKeyWidth)
	label := truncateWidth(item.Label, labelWidth)

	return keyStyle.Render(truncateWidth(item.Key, helpKeyWidth)) +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colors.secondary)).
			Width(labelWidth).
			Render(label)
}
