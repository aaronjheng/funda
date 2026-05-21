package view

import (
	"fmt"
	"strings"
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
	clipboardDisplayDuration  = 2 * time.Second
	fundCardContentLines      = 4 // title, nav, change, estimate
	fundCardBorderLines       = 2
	fundCardHeight            = fundCardContentLines + fundCardBorderLines // 6
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

type clearClipboardMsg struct{}

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
	clipboardMsg  string
	cardCache     map[string]string
}

func cardCacheKey(fund config.Fund, fundData data.FundData, cardWidth int, lastTradingDay time.Time) string {
	return fmt.Sprintf("%s|%d|%s|%s|%v|%v|%s|%v|%s|%s",
		fund.Code,
		cardWidth,
		fundData.Name,
		fundData.Alias,
		fundData.NAV,
		fundData.PrevNAV,
		fundData.NAVDate,
		fundData.EstimateNAV,
		fundData.EstimateTime,
		lastTradingDay.Format("2006-01-02"),
	)
}

func clearClipboardMsgCmd() tea.Cmd {
	return tea.Tick(clipboardDisplayDuration, func(_ time.Time) tea.Msg {
		return clearClipboardMsg{}
	})
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
		clipboardMsg:  "",
		cardCache:     make(map[string]string),
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

	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	sections = append(sections, selectorStr)
	sections = append(sections, "")

	group := m.groups[m.currentGroup]
	lastTradingDay := data.GetLastTradingDate(time.Now())

	numRows := (len(group.Funds) + cardsPerRow - 1) / cardsPerRow
	totalHeight := numRows * fundCardHeight
	available := m.availableHeight()

	cardWidth := (m.width - cardPaddingWidth) / cardsPerRow
	if cardWidth < minCardWidth {
		cardWidth = m.width - cardPaddingWidth
	}

	// If content overflows, account for scrollbar width to avoid double rendering
	if totalHeight > available {
		cardWidth = (m.width - scrollbarWidth - (cardsPerRow - 1)) / cardsPerRow
		if cardWidth < minCardWidth {
			cardWidth = m.width - scrollbarWidth
		}
	}

	if len(group.Funds) > 0 {
		visibleStart, visibleEnd, offset := m.calcVisibleFundRange(len(group.Funds), totalHeight, available)
		visibleFunds := group.Funds[visibleStart:visibleEnd]
		rows := m.renderFundRows(visibleFunds, cardWidth, lastTradingDay)
		sections = append(sections, m.renderScrollableRows(rows, totalHeight, available, offset))
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
	v.MouseMode = tea.MouseModeCellMotion

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

		fundData := m.fundData[fund.Code]
		if fundData.Code == "" {
			fundData.Code = fund.Code
			fundData.Alias = fund.Alias
		}

		cacheKey := cardCacheKey(fund, fundData, cardWidth, lastTradingDay)
		if cached, ok := m.cardCache[cacheKey]; ok {
			pair = append(pair, cached)

			continue
		}

		card := RenderFundCard(fundData, cardWidth, lastTradingDay)
		m.cardCache[cacheKey] = card
		pair = append(pair, card)
	}

	return pair
}

func (m Model) calcMaxScrollOffset(totalLines, availableHeight int) int {
	return max(0, totalLines-availableHeight)
}

func (m Model) clampedScrollOffset(totalHeight, available int) int {
	if totalHeight <= available {
		return 0
	}

	maxOffset := m.calcMaxScrollOffset(totalHeight, available)

	return max(0, min(m.scrollOffset, maxOffset))
}

func (m Model) calcVisibleFundRange(totalFunds, totalHeight, available int) (int, int, int) {
	offset := m.clampedScrollOffset(totalHeight, available)
	end := min(offset+available, totalHeight)

	startRow := offset / fundCardHeight
	endRow := min((end+fundCardHeight-1)/fundCardHeight, (totalFunds+cardsPerRow-1)/cardsPerRow)

	startFund := startRow * cardsPerRow
	endFund := min(endRow*cardsPerRow, totalFunds)

	return startFund, endFund, offset
}

func (m Model) availableHeight() int {
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	fixed := lipgloss.Height(selectorStr) +
		lipgloss.Height(m.renderStatusBar()) +
		lipgloss.Height(RenderFooter(m.width)) + fixedSectionGaps

	return max(0, m.height-fixed)
}

func (m Model) renderScrollableRows(rows []string, totalHeight, available, offset int) string {
	if totalHeight <= available {
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	end := min(offset+available, totalHeight)
	firstRowOffset := (offset / fundCardHeight) * fundCardHeight

	startRow := (offset - firstRowOffset) / fundCardHeight
	firstRowIdx := firstRowOffset / fundCardHeight
	endRow := min((end+fundCardHeight-1)/fundCardHeight-firstRowIdx, len(rows))

	visibleRows := rows[startRow:endRow]
	allContent := lipgloss.JoinVertical(lipgloss.Left, visibleRows...)
	allLines := strings.Split(allContent, "\n")

	skipTop := (offset - firstRowOffset) % fundCardHeight
	if skipTop > 0 && len(allLines) > skipTop {
		allLines = allLines[skipTop:]
	}

	keepLines := end - offset
	if len(allLines) > keepLines {
		allLines = allLines[:keepLines]
	}

	visibleContent := strings.Join(allLines, "\n")
	scrollbar := RenderScrollbar(offset, totalHeight, available)

	return lipgloss.JoinHorizontal(lipgloss.Top, visibleContent, " ", scrollbar)
}

func (m Model) renderStatusBar() string {
	switch {
	case m.clipboardMsg != "":
		return RenderStatusBar(m.clipboardMsg, m.width, false)
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
