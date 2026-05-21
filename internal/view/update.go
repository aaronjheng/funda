package view

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.searchMode {
			return m.handleSearchKey(msg)
		}

		return m.handleNormalKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollOffset = 0

		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg), nil

	case tickMsg:
		return m.handleTick()

	case allFundsFetchedMsg:
		return m.handleFundsFetched(msg)

	case searchResultMsg:
		return m.handleSearchResult(msg)

	case clearClipboardMsg:
		m.clipboardMsg = ""

		return m, nil
	}

	return m, nil
}

const (
	scrollKeyStep     = fundCardHeight // keyboard jumps by full card row
	scrollWheelStep   = 1              // mouse/touchpad scrolls line-by-line for smoothness
	scrollPageDivisor = 2
)

func (m Model) clampedMaxScrollOffset() int {
	group := m.groups[m.currentGroup]
	numRows := (len(group.Funds) + cardsPerRow - 1) / cardsPerRow
	totalHeight := numRows * fundCardHeight
	available := m.availableHeight()

	return m.calcMaxScrollOffset(totalHeight, available)
}

func (m Model) handleScrollKey(key string) Model {
	pageSize := max(1, m.availableHeight()/scrollPageDivisor)
	maxOffset := m.clampedMaxScrollOffset()

	switch key {
	case "up", "k":
		m.scrollOffset = max(0, m.scrollOffset-scrollKeyStep)
	case "down", "j":
		m.scrollOffset = min(maxOffset, m.scrollOffset+scrollKeyStep)
	case "pgup":
		m.scrollOffset = max(0, m.scrollOffset-pageSize)
	case "pgdown":
		m.scrollOffset = min(maxOffset, m.scrollOffset+pageSize)
	}

	return m
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) Model {
	maxOffset := m.clampedMaxScrollOffset()

	switch msg.Button {
	case tea.MouseWheelUp:
		m.scrollOffset = max(0, m.scrollOffset-scrollWheelStep)
	case tea.MouseWheelDown:
		m.scrollOffset = min(maxOffset, m.scrollOffset+scrollWheelStep)
	}

	return m
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.searchMode || msg.Button != tea.MouseLeft {
		return m, nil
	}

	selectorStr, bounds := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	selectorHeight := lipgloss.Height(selectorStr)

	if msg.Y >= headerTopPadding && msg.Y < headerTopPadding+selectorHeight {
		for _, b := range bounds {
			if msg.X >= b.StartX && msg.X < b.EndX {
				if b.Index != m.currentGroup {
					m.currentGroup = b.Index
					m.scrollOffset = 0
					m = m.loadGroupCache()
				}
				return m, nil
			}
		}
		return m, nil
	}

	group := m.groups[m.currentGroup]
	if len(group.Funds) == 0 {
		return m, nil
	}

	if !m.isMouseInFundArea(msg.Y) {
		return m, nil
	}

	numRows := (len(group.Funds) + cardsPerRow - 1) / cardsPerRow

	fundIdx := m.fundIndexFromMouse(msg, numRows)
	if fundIdx < 0 || fundIdx >= len(group.Funds) {
		return m, nil
	}

	code := group.Funds[fundIdx].Code
	m.clipboardMsg = "Copied: " + m.fundDisplayName(group.Funds[fundIdx])

	return m, tea.Batch(
		tea.SetClipboard(code),
		clearClipboardMsgCmd(),
	)
}

func (m Model) fundDisplayName(fund config.Fund) string {
	if fd, ok := m.fundData[fund.Code]; ok && fd.Name != "" {
		return fd.Name + " (" + fund.Code + ")"
	}

	if fund.Alias != "" {
		return fund.Alias + " (" + fund.Code + ")"
	}

	return fund.Code
}

func (m Model) isMouseInFundArea(mouseY int) bool {
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	headerHeight := lipgloss.Height(selectorStr) + 1 + headerTopPadding
	footerHeight := lipgloss.Height(RenderFooter(m.width))

	return mouseY >= headerHeight && mouseY < m.height-footerHeight-1
}

func (m Model) fundIndexFromMouse(msg tea.MouseClickMsg, numRows int) int {
	totalHeight := numRows * fundCardHeight
	available := m.availableHeight()
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	headerHeight := lipgloss.Height(selectorStr) + 1 + headerTopPadding
	relativeY := msg.Y - headerHeight

	targetRowIdx := m.resolveRowIndex(numRows, totalHeight, available, relativeY)
	if targetRowIdx < 0 {
		return -1
	}

	targetCol := m.resolveColumn(msg.X)

	fundIdx := targetRowIdx*cardsPerRow + targetCol
	group := m.groups[m.currentGroup]

	if fundIdx >= len(group.Funds) && targetCol > 0 {
		fundIdx = targetRowIdx * cardsPerRow
	}

	return fundIdx
}

func (m Model) resolveRowIndex(numRows, totalHeight, available, relativeY int) int {
	if totalHeight <= available {
		return m.resolveRowIndexNoScroll(numRows, relativeY)
	}

	return m.resolveRowIndexWithScroll(numRows, available, relativeY)
}

func (m Model) resolveRowIndexNoScroll(numRows, relativeY int) int {
	rowIdx := relativeY / fundCardHeight
	if rowIdx >= 0 && rowIdx < numRows {
		return rowIdx
	}

	return -1
}

func (m Model) resolveRowIndexWithScroll(numRows, available, relativeY int) int {
	totalLines := numRows * fundCardHeight
	offset := m.clampedScrollOffset(totalLines, available)

	visibleLineIdx := relativeY + offset
	if visibleLineIdx >= totalLines || visibleLineIdx < 0 {
		return -1
	}

	return visibleLineIdx / fundCardHeight
}

func (m Model) resolveColumn(mouseX int) int {
	if cardsPerRow <= 1 {
		return 0
	}

	midX := m.width / cardsPerRow
	if mouseX >= midX {
		return 1
	}

	return 0
}

func (m Model) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		if !m.loading {
			m.loading = true
			m.errMsg = ""

			return m, m.fetchAllFundsCmd()
		}
	case "s":
		m.searchMode = true
		m.searchQuery = ""
		m.searchResults = nil
		m.searchCursor = 0

		return m, nil
	case "c":
		return m.handleClearCache()
	case "left", "h":
		m = m.handlePrevGroup()
	case "right", "l":
		m = m.handleNextGroup()
	case "up", "k", "down", "j", "pgup", "pgdown":
		m = m.handleScrollKey(msg.String())
	}

	return m, nil
}

func (m Model) handleClearCache() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	m.fetcher.ClearCache()
	m.fundData = make(map[string]data.FundData)
	m.cardCache = make(map[string]string)
	m.loading = true
	m.errMsg = ""

	return m, m.fetchAllFundsCmd()
}

func (m Model) handlePrevGroup() Model {
	if m.currentGroup > 0 {
		m.currentGroup--
		m.scrollOffset = 0
		m = m.loadGroupCache()
	}

	return m
}

func (m Model) handleNextGroup() Model {
	if m.currentGroup < len(m.groups)-1 {
		m.currentGroup++
		m.scrollOffset = 0
		m = m.loadGroupCache()
	}

	return m
}

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchQuery = ""
		m.searchResults = nil

		return m, nil
	case "enter":
		return m.handleSearchEnter()
	default:
		return m.handleSearchEditKey(msg.String())
	}
}

func (m Model) handleSearchEditKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m = m.moveSearchCursorUp()
	case "down", "j":
		m = m.moveSearchCursorDown()
	case "backspace":
		m = m.deleteSearchChar()
	case "ctrl+u":
		m.searchQuery = ""
	default:
		return m.handleSearchCharInput(key)
	}

	return m, nil
}

func (m Model) moveSearchCursorUp() Model {
	if m.searchCursor > 0 {
		m.searchCursor--
	}

	return m
}

func (m Model) moveSearchCursorDown() Model {
	if m.searchCursor < len(m.searchResults)-1 {
		m.searchCursor++
	}

	return m
}

func (m Model) deleteSearchChar() Model {
	if len(m.searchQuery) > 0 {
		m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
	}

	return m
}

func (m Model) handleSearchCharInput(key string) (tea.Model, tea.Cmd) {
	if len(key) == 1 && key >= " " && key <= "~" {
		m.searchQuery += key

		return m, m.searchFundCmd(m.searchQuery)
	}

	return m, nil
}

func (m Model) handleSearchEnter() (tea.Model, tea.Cmd) {
	if m.searchCursor >= 0 && m.searchCursor < len(m.searchResults) {
		result := m.searchResults[m.searchCursor]
		m = m.addFundToAll(result.Code, result.Name)
	}

	m.searchMode = false
	m.searchQuery = ""
	m.searchResults = nil

	return m, m.fetchAllFundsCmd()
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	if !m.loading && m.config.RefreshInterval > 0 {
		m.loading = true
		m.errMsg = ""

		return m, tea.Batch(
			m.fetchAllFundsCmd(),
			m.startTickCmd(),
		)
	}

	return m, m.startTickCmd()
}

func (m Model) handleFundsFetched(msg allFundsFetchedMsg) (tea.Model, tea.Cmd) {
	m.loading = false

	if msg.err != nil {
		m.errMsg = msg.err.Error()
	} else {
		m.fundData = msg.funds
		m.errMsg = ""
		m.lastRefresh = time.Now()
		m.cardCache = make(map[string]string)
	}

	return m, nil
}

func (m Model) handleSearchResult(msg searchResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.errMsg = msg.err.Error()
	} else {
		m.searchResults = msg.results
		m.searchCursor = 0
	}

	return m, nil
}

func (m Model) loadGroupCache() Model {
	group := m.groups[m.currentGroup]
	for _, fund := range group.Funds {
		if cached, ok := data.LoadFundCache(fund.Code); ok {
			cached.Alias = fund.Alias
			m.fundData[fund.Code] = cached
		}
	}

	return m
}

func (m Model) addFundToAll(code, name string) Model {
	for idx := range m.groups {
		if m.groups[idx].Name == "All" {
			for _, fund := range m.groups[idx].Funds {
				if fund.Code == code {
					return m
				}
			}

			m.groups[idx].Funds = append(m.groups[idx].Funds, config.Fund{Code: code, Alias: name})

			return m
		}
	}

	return m
}

func (m Model) fetchAllFundsCmd() tea.Cmd {
	return func() tea.Msg {
		group := m.groups[m.currentGroup]

		funds := make([]struct{ Code, Alias string }, len(group.Funds))
		for idx, fund := range group.Funds {
			funds[idx] = struct{ Code, Alias string }{Code: fund.Code, Alias: fund.Alias}
		}

		ctx := context.Background()
		result := m.fetcher.FetchAllCards(ctx, funds)

		return allFundsFetchedMsg{funds: result, err: nil}
	}
}

func (m Model) startTickCmd() tea.Cmd {
	interval := time.Duration(m.config.RefreshInterval) * time.Second
	if interval <= 0 {
		interval = defaultRefreshIntervalSec * time.Second
	}

	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) searchFundCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		results, err := m.fetcher.SearchFund(ctx, query)

		return searchResultMsg{results: results, err: err}
	}
}
