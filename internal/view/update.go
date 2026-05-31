package view

import (
	"maps"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.applySearchDimensions()
		m = m.syncViewport()

		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd

	case tickMsg:
		return m.handleTick()

	case allFundsFetchedMsg:
		return m.handleFundsFetched(msg)

	case estimatesFetchedMsg:
		return m.handleEstimatesFetched(msg)

	case searchResultMsg:
		return m.handleSearchResult(msg)

	case clearClipboardMsg:
		m.toastMsg = ""
		m.clipboardMsg = ""
		m.copiedCode = ""

		return m, nil
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.searchMode {
		return m.handleSearchKey(msg)
	}

	return m.handleNormalKey(msg)
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.searchMode || msg.Button != tea.MouseLeft {
		return m, nil
	}

	if handled, model, cmd := m.handleSelectorClick(msg); handled {
		return model, cmd
	}

	if len(m.sortedFunds) == 0 {
		return m, nil
	}

	if !m.isMouseInFundArea(msg.Y) {
		return m, nil
	}

	numRows := (len(m.sortedFunds) + cardsPerRow - 1) / cardsPerRow

	fundIdx := m.fundIndexFromMouse(msg, numRows)
	if fundIdx < 0 || fundIdx >= len(m.sortedFunds) {
		return m, nil
	}

	code := m.sortedFunds[fundIdx].Code
	m.toastMsg = "已复制: " + m.fundDisplayName(m.sortedFunds[fundIdx])
	m.copiedCode = code

	return m, tea.Batch(
		tea.SetClipboard(code),
		clearClipboardMsgCmd(),
	)
}

func (m Model) handleSelectorClick(msg tea.MouseClickMsg) (bool, Model, tea.Cmd) {
	selectorStr, bounds := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	selectorHeight := lipgloss.Height(selectorStr)

	if msg.Y < 1 || msg.Y >= 1+selectorHeight {
		return false, m, nil
	}

	for _, b := range bounds {
		if msg.X >= b.StartX && msg.X < b.EndX {
			if b.Index != m.currentGroup {
				m.currentGroup = b.Index
				m.viewport.GotoTop()
				m.loading = true
				m.errMsg = ""
				m.lastFullRefresh = time.Now()

				m.logger.Info("selector clicked to switch group", "group", m.groups[m.currentGroup].Name)

				return true, m, m.fetchAllFundsCmd()
			}

			return true, m, nil
		}
	}

	return true, m, nil
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
	headerHeight := 1 + lipgloss.Height(selectorStr)
	bottomReserve := lipgloss.Height(m.renderStatusBar())

	return mouseY >= headerHeight && mouseY < m.height-bottomReserve
}

func (m Model) fundIndexFromMouse(msg tea.MouseClickMsg, numRows int) int {
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	headerHeight := 1 + lipgloss.Height(selectorStr)
	relativeY := msg.Y - headerHeight + m.viewport.YOffset()

	targetRowIdx := relativeY / fundCardHeight
	if targetRowIdx < 0 || targetRowIdx >= numRows {
		return -1
	}

	targetCol := m.resolveColumn(msg.X)

	fundIdx := targetRowIdx*cardsPerRow + targetCol

	if fundIdx >= len(m.sortedFunds) && targetCol > 0 {
		fundIdx = targetRowIdx * cardsPerRow
	}

	return fundIdx
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
	if m.reloadConfirm {
		return m.handleReloadConfirmKey(msg)
	}

	if handled, model := m.handleHelpKey(msg); handled {
		return model, nil
	}

	return m.handleActionKey(msg)
}

func (m Model) handleActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.cancel()

		return m, tea.Quit
	case "r":
		return m.handleRefreshKey()
	case "R":
		return m.handleReloadKey()
	case "s":
		m.searchMode = true
		m.textInput.Reset()
		m.searchList.ResetSelected()
		m.lastSearchQuery = ""
		m.searchGeneration = 0
		m = m.applySearchDimensions()

		return m, nil
	case "c":
		return m.handleClearCache()
	case "?":
		m.helpModel.ShowAll = true
		m.showHelp = true

		return m, nil
	case "o", "O":
		m = m.handleSortModeKey(msg.String())
		m = m.syncViewport()

		return m, nil
	case "left", "h", "right", "l":
		return m.handleGroupKey(msg.String())
	case "up", "k", "down", "j", "pgup", "pgdown":
		m = m.handleScrollKey(msg.String())

		return m, nil
	}

	return m, nil
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (bool, Model) {
	if !m.showHelp {
		return false, m
	}

	switch msg.String() {
	case "?", "esc":
		m.helpModel.ShowAll = false
		m.showHelp = false
	}

	return true, m
}

func (m Model) handleScrollKey(key string) Model {
	switch key {
	case "up", "k":
		m.viewport.ScrollUp(fundCardHeight)
	case "down", "j":
		m.viewport.ScrollDown(fundCardHeight)
	case "pgup":
		m.viewport.PageUp()
	case "pgdown":
		m.viewport.PageDown()
	}

	return m
}

func (m Model) handleReloadConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.reloadConfirm = false

	if msg.String() == "y" || msg.String() == "Y" {
		return m.handleReloadConfig()
	}

	m.errMsg = ""

	return m, nil
}

func (m Model) handleClearCache() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	m.logger.Info("user triggered clear cache")

	m.fetcher.ClearCache()
	m.fundData = make(map[string]data.FundData)
	m.cardCache = make(map[string]string)
	m.loading = true
	m.errMsg = ""
	m.lastFullRefresh = time.Now()

	return m, m.fetchAllFundsCmd()
}

func (m Model) handlePrevGroup() (Model, tea.Cmd) {
	if m.currentGroup > 0 {
		m.currentGroup--
		m.viewport.GotoTop()
		m = m.loadGroupCache()
		m.loading = true
		m.errMsg = ""

		m.logger.Info("switched to previous group", "group", m.groups[m.currentGroup].Name)

		return m, m.fetchAllFundsCmd()
	}

	return m, nil
}

func (m Model) handleNextGroup() (Model, tea.Cmd) {
	if m.currentGroup < len(m.groups)-1 {
		m.currentGroup++
		m.viewport.GotoTop()
		m = m.loadGroupCache()
		m.loading = true
		m.errMsg = ""

		m.logger.Info("switched to next group", "group", m.groups[m.currentGroup].Name)

		return m, m.fetchAllFundsCmd()
	}

	return m, nil
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	if m.config.RefreshInterval <= 0 {
		return m, nil
	}

	if m.loading {
		return m, m.startTickCmd()
	}

	now := time.Now()
	cmds := []tea.Cmd{m.startTickCmd()}

	if m.shouldRefreshNAV(now) {
		m.logger.Info("tick triggered full refresh")

		m.lastFullRefresh = now
		m.loading = true
		m.errMsg = ""

		group := m.groups[m.currentGroup]
		codes := make([]string, 0, len(group.Funds))

		for _, fund := range group.Funds {
			codes = append(codes, fund.Code)
		}

		m.fetcher.RemoveCachedEntries(codes)

		cmds = append(cmds, m.fetchAllFundsCmd())
	}

	if data.IsTradingHours(now) && len(m.fundData) > 0 {
		cmds = append(cmds, m.fetchEstimatesCmd())
	}

	return m, tea.Batch(cmds...)
}

func (m Model) shouldRefreshNAV(now time.Time) bool {
	if m.lastFullRefresh.IsZero() {
		return true
	}

	if !m.anyNAVStale(now) {
		return false
	}

	if data.IsTradingDay(now) {
		local := now.In(data.ShanghaiLocation())
		if local.Hour() >= 15 && local.Hour() < 22 {
			return now.Sub(m.lastFullRefresh) >= navPostCloseRefreshInterval
		}
	}

	return now.Sub(m.lastFullRefresh) >= navFallbackRefreshInterval
}

func (m Model) anyNAVStale(now time.Time) bool {
	if len(m.fundData) == 0 {
		return true
	}

	lastTradingDay := data.GetLastTradingDate(now)

	for _, fd := range m.fundData {
		if fd.IsQDII {
			continue
		}

		if !data.NavIsCurrent(fd.NAVDate, lastTradingDay) {
			return true
		}
	}

	return false
}

func (m Model) fetchEstimatesCmd() tea.Cmd {
	return func() tea.Msg {
		group := m.groups[m.currentGroup]

		codes := make([]string, 0, len(group.Funds))
		for _, fund := range group.Funds {
			codes = append(codes, fund.Code)
		}

		ctx := m.ctx
		estimates := m.fetcher.RefreshAllEstimates(ctx, codes)

		return estimatesFetchedMsg{estimates: estimates}
	}
}

func (m Model) handleEstimatesFetched(msg estimatesFetchedMsg) (tea.Model, tea.Cmd) {
	updated := 0

	for code, est := range msg.estimates {
		if fd, ok := m.fundData[code]; ok {
			fd.LatestNAV = est.LatestNAV
			fd.LatestTime = est.LatestTime
			m.fundData[code] = fd
			updated++
		}
	}

	m.lastRefresh = time.Now()
	m.cardCache = make(map[string]string)
	m = m.syncViewport()

	m.logger.Debug("estimates applied", "updated", updated)

	return m, nil
}

func (m Model) handleFundsFetched(msg allFundsFetchedMsg) (tea.Model, tea.Cmd) {
	m.loading = false

	if msg.err != nil {
		m.errMsg = msg.err.Error()
		m.logger.Error("funds fetch failed", "error", msg.err)
	} else {
		maps.Copy(m.fundData, msg.funds)

		m.errMsg = ""
		m.lastRefresh = time.Now()
		m.cardCache = make(map[string]string)
		m = m.sortFunds()
		m = m.syncViewport()

		m.logger.Info("funds fetched and applied", "count", len(msg.funds))
	}

	return m, nil
}

func (m Model) loadGroupCache() Model {
	group := m.groups[m.currentGroup]
	for _, fund := range group.Funds {
		if cached, ok := data.LoadFundCache(m.logger, fund.Code); ok {
			cached.Alias = fund.Alias
			m.fundData[fund.Code] = cached
		}
	}

	return m.sortFunds()
}

func (m Model) loadGroupCacheIgnoreTTL() Model {
	group := m.groups[m.currentGroup]
	for _, fund := range group.Funds {
		if cached, ok := data.LoadFundCacheIgnoreTTL(m.logger, fund.Code); ok {
			cached.Alias = fund.Alias
			m.fundData[fund.Code] = cached
		}
	}

	return m.sortFunds()
}

func (m Model) fetchAllFundsCmd() tea.Cmd {
	return func() tea.Msg {
		group := m.groups[m.currentGroup]

		funds := make([]struct{ Code, Alias string }, len(group.Funds))
		for idx, fund := range group.Funds {
			funds[idx] = struct{ Code, Alias string }{Code: fund.Code, Alias: fund.Alias}
		}

		ctx := m.ctx
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
