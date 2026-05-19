package view

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

const (
	defaultRefreshIntervalSec = 60
	cardPaddingWidth          = 2
	cardsPerRow               = 2
	minCardWidth              = 20
	labelWidth                = 12
	valueWidthOffset          = 14
	fixedSectionGaps          = 3
	scrollbarWidth            = 2
	cardFrameWidth            = 4 // border(2) + horizontal padding(2)
)

type tickMsg time.Time

type allFundsFetchedMsg struct {
	funds map[string]data.FundData
	err   error
}

type searchResultMsg struct {
	results []data.SearchHit
	err     error
}

type Model struct {
	config        config.Config
	groups        []config.Group
	currentGroup  int
	fundData      map[string]data.FundData
	loading       bool
	errMsg        string
	width         int
	height        int
	fetcher       *data.Fetcher
	searchMode    bool
	searchQuery   string
	searchCursor  int
	searchResults []data.SearchHit
	keymap        KeyMap
	lastRefresh   time.Time
	scrollOffset  int
}

func NewModel(cfg config.Config, fetcher *data.Fetcher) Model {
	return Model{
		config:        cfg,
		groups:        cfg.Groups,
		currentGroup:  0,
		fundData:      make(map[string]data.FundData),
		loading:       false,
		errMsg:        "",
		width:         0,
		height:        0,
		fetcher:       fetcher,
		searchMode:    false,
		searchQuery:   "",
		searchCursor:  0,
		searchResults: nil,
		keymap:        DefaultKeyMap(),
		lastRefresh:   time.Time{},
		scrollOffset:  0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchAllFundsCmd(),
		m.startTickCmd(),
	)
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var sections []string

	sections = append(sections, RenderGroupSelector(m.groups, m.currentGroup, m.width))
	sections = append(sections, "")

	group := m.groups[m.currentGroup]
	lastTradingDay := data.GetLastTradingDate(time.Now())

	cardWidth := (m.width - cardPaddingWidth) / cardsPerRow
	if cardWidth < minCardWidth {
		cardWidth = m.width - cardPaddingWidth
	}

	rows := m.renderFundRows(group.Funds, cardWidth, lastTradingDay)

	if len(rows) > 0 {
		sections = append(sections, m.renderScrollableRows(rows, cardWidth))
	} else {
		noFundsStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(m.availableHeight()).
			Align(lipgloss.Center)

		sections = append(sections, noFundsStyle.Render("No funds in this group"))
	}

	sections = append(sections, "")
	sections = append(sections, m.renderStatusBar())
	sections = append(sections, "")
	sections = append(sections, RenderFooter(m.width))

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.searchMode {
		overlay := RenderSearchOverlay(
			m.searchQuery,
			m.searchResults,
			m.searchCursor,
			m.width,
		)
		view = overlay + "\n" + view
	}

	v := tea.NewView(view)
	v.AltScreen = true

	return v
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

		fd := m.fundData[fund.Code]
		if fd.Code == "" {
			fd = data.FundData{Code: fund.Code, Alias: fund.Alias} //nolint:exhaustruct // placeholder for missing data
		}

		pair = append(pair, RenderFundCard(fd, cardWidth, lastTradingDay))
	}

	return pair
}

func (m Model) calcMaxScrollOffset(rows []string, availableHeight int) int {
	excess := 0
	for _, row := range rows {
		excess += lipgloss.Height(row)
	}

	excess -= availableHeight
	if excess <= 0 {
		return 0
	}

	cutoff := 0

	for idx, row := range rows {
		cutoff += lipgloss.Height(row)

		if cutoff >= excess {
			return idx + 1
		}
	}

	return len(rows)
}

func (m Model) availableHeight() int {
	fixed := lipgloss.Height(RenderGroupSelector(m.groups, m.currentGroup, m.width)) +
		lipgloss.Height(m.renderStatusBar()) +
		lipgloss.Height(RenderFooter(m.width)) + fixedSectionGaps

	return max(0, m.height-fixed)
}

func (m Model) renderScrollableRows(rows []string, cardWidth int) string {
	available := m.availableHeight()

	totalHeight := 0
	for _, row := range rows {
		totalHeight += lipgloss.Height(row)
	}

	if totalHeight <= available {
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	adjustedCardWidth := (m.width - scrollbarWidth - (cardsPerRow - 1)) / cardsPerRow
	if adjustedCardWidth < minCardWidth {
		adjustedCardWidth = m.width - scrollbarWidth
	}

	if adjustedCardWidth != cardWidth {
		lastTradingDay := data.GetLastTradingDate(time.Now())
		group := m.groups[m.currentGroup]
		rows = m.renderFundRows(group.Funds, adjustedCardWidth, lastTradingDay)
	}

	maxScroll := m.calcMaxScrollOffset(rows, available)
	offset := max(0, min(m.scrollOffset, maxScroll))

	var visible []string

	visibleHeight := 0

	for idx := offset; idx < len(rows); idx++ {
		rowHeight := lipgloss.Height(rows[idx])
		if visibleHeight+rowHeight > available && len(visible) > 0 {
			break
		}

		visible = append(visible, rows[idx])
		visibleHeight += rowHeight
	}

	content := lipgloss.JoinVertical(lipgloss.Left, visible...)
	scrollbar := RenderScrollbar(offset, len(rows), len(visible), visibleHeight)

	return lipgloss.JoinHorizontal(lipgloss.Top, content, " ", scrollbar)
}

func (m Model) renderStatusBar() string {
	switch {
	case m.errMsg != "":
		return RenderStatusBar(m.errMsg, m.width, true)
	case m.loading:
		return RenderStatusBar("Loading...", m.width, false)
	case !m.lastRefresh.IsZero():
		msg := "Last updated: " + m.lastRefresh.Format("15:04:05")

		return RenderStatusBar(msg, m.width, false)
	default:
		return ""
	}
}
