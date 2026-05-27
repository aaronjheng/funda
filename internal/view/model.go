package view

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"

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
	headerTopPadding          = 1
	sortStateDirPermissions   = 0o700
	sortStateFilePermissions  = 0o600
	scrollbarWidth            = 2
	cardFrameWidth            = 4 // border(2) + horizontal padding(2)
	clipboardDisplayDuration  = 1 * time.Second
	fundCardContentLines      = 4 // title, nav, change, estimate
	fundCardBorderLines       = 2
	fundCardHeight            = fundCardContentLines + fundCardBorderLines // 6
	mainSectionsCap           = 8
	toastVerticalDiv          = 2
)

type SortField int

const (
	SortDefault SortField = iota
	SortNAV
	SortDayChange
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
	config          config.Config
	groups          []config.Group
	currentGroup    int
	fundData        map[string]data.FundData
	loading         bool
	errMsg          string
	width           int
	height          int
	fetcher         *data.Fetcher
	searchMode      bool
	searchQuery     string
	searchCursor    int
	searchResults   []data.SearchHit
	keymap          KeyMap
	lastRefresh     time.Time
	scrollOffset    int
	clipboardMsg    string
	copiedCode      string
	toastMsg        string
	cardCache       map[string]string
	sortField       SortField
	sortAsc         bool
	sortedFunds     []config.Fund
	configFilepath  string
	hasUnsavedFunds bool
	reloadConfirm   bool
}

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

func clearClipboardMsgCmd() tea.Cmd {
	return tea.Tick(clipboardDisplayDuration, func(_ time.Time) tea.Msg {
		return clearClipboardMsg{}
	})
}

func NewModel(cfg config.Config, fetcher *data.Fetcher, configFilepath string) Model {
	model := Model{
		config:          cfg,
		groups:          cfg.Groups,
		currentGroup:    0,
		fundData:        make(map[string]data.FundData),
		loading:         false,
		errMsg:          "",
		width:           0,
		height:          0,
		fetcher:         fetcher,
		searchMode:      false,
		searchQuery:     "",
		searchCursor:    0,
		searchResults:   nil,
		keymap:          DefaultKeyMap(),
		lastRefresh:     time.Time{},
		scrollOffset:    0,
		clipboardMsg:    "",
		copiedCode:      "",
		toastMsg:        "",
		cardCache:       make(map[string]string),
		sortField:       SortDefault,
		sortAsc:         false,
		sortedFunds:     nil,
		configFilepath:  configFilepath,
		hasUnsavedFunds: false,
		reloadConfirm:   false,
	}
	model = model.loadGroupCacheIgnoreTTL()
	model = model.loadState()

	return model
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

	view := m.renderMain()

	if m.searchMode {
		overlay := RenderSearchOverlay(
			m.searchQuery,
			m.searchResults,
			m.searchCursor,
			m.width,
		)
		view = overlay + "\n" + view
	}

	if m.toastMsg != "" {
		view = m.overlayToast(view)
	}

	v := tea.NewView(view)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}

func (m Model) overlayToast(view string) string {
	toastContent := RenderToast(m.toastMsg)
	toastLines := strings.Split(toastContent, "\n")
	toastH := len(toastLines)
	toastWidth := lipgloss.Width(toastContent)

	viewLines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	totalLines := len(viewLines)

	startLine := max(0, (totalLines-toastH)/toastVerticalDiv)
	startCol := max(0, (m.width-toastWidth)/toastVerticalDiv)

	for i := 0; i < toastH && startLine+i < totalLines; i++ {
		viewLines[startLine+i] = overlayLine(viewLines[startLine+i], toastLines[i], startCol, toastWidth)
	}

	return strings.Join(viewLines, "\n")
}

type ansiStyle struct {
	active []string
}

func (s *ansiStyle) record(seq string, beforeCol bool) {
	if seq == "\x1b[0m" {
		s.active = nil
	} else if beforeCol {
		s.active = append(s.active, seq)
	}
}

func (s *ansiStyle) restore() string {
	var out strings.Builder

	for _, st := range s.active {
		out.WriteString(st)
	}

	return out.String()
}

func overlayLine(orig, overlay string, startCol, overlayWidth int) string {
	var (
		out, esc strings.Builder
		style    ansiStyle
	)

	visCol := 0
	inEsc := false
	inserted := false

	for _, char := range orig {
		if char == '\x1b' {
			inEsc = true

			esc.Reset()
			esc.WriteRune(char)
			out.WriteRune(char)

			continue
		}

		if inEsc {
			esc.WriteRune(char)
			out.WriteRune(char)

			if char == 'm' {
				style.record(esc.String(), visCol < startCol)

				inEsc = false

				esc.Reset()
			}

			continue
		}

		if visCol == startCol && !inserted {
			out.WriteString("\x1b[0m")
			out.WriteString(overlay)
			out.WriteString("\x1b[0m")
			out.WriteString(style.restore())

			inserted = true
		}

		if visCol < startCol || visCol >= startCol+overlayWidth {
			out.WriteRune(char)
		}

		visCol += lipgloss.Width(string(char))
	}

	if !inserted {
		out.WriteString("\x1b[0m")
		out.WriteString(overlay)
	}

	return out.String()
}

type persistedState struct {
	SortField SortField `json:"sort_field"`
	SortAsc   bool      `json:"sort_asc"`
}

func statePath() string {
	return filepath.Join(xdg.StateHome, "funda", "state.json")
}

func (m Model) saveState() {
	dir := filepath.Dir(statePath())
	_ = os.MkdirAll(dir, sortStateDirPermissions)

	data, err := json.Marshal(persistedState{SortField: m.sortField, SortAsc: m.sortAsc})
	if err != nil {
		return
	}

	_ = os.WriteFile(statePath(), data, sortStateFilePermissions)
}

func (m Model) loadState() Model {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return m
	}

	var state persistedState

	err = json.Unmarshal(data, &state)
	if err != nil {
		return m
	}

	m.sortField = state.SortField
	m.sortAsc = state.SortAsc

	return m.sortFunds()
}

func (m Model) sortFieldLabel() string {
	switch m.sortField {
	case SortDefault:
		return ""
	case SortNAV:
		return "净值"
	case SortDayChange:
		return "日涨跌"
	}

	return ""
}

func (m Model) sortFunds() Model {
	group := m.groups[m.currentGroup]
	funds := group.Funds

	if m.sortField == SortDefault {
		m.sortedFunds = funds

		return m
	}

	sorted := make([]config.Fund, len(funds))
	copy(sorted, funds)

	sort.SliceStable(sorted, func(i, j int) bool {
		fundI := m.fundData[sorted[i].Code]
		fundJ := m.fundData[sorted[j].Code]

		var less bool

		switch m.sortField {
		case SortNAV:
			less = fundI.NAV < fundJ.NAV
		case SortDayChange:
			less = fundI.DayChangePercent() < fundJ.DayChangePercent()
		case SortDefault:
			return false
		}

		if m.sortAsc {
			return less
		}

		return !less
	})

	m.sortedFunds = sorted

	return m
}

func (m Model) cycleSortMode() Model {
	switch m.sortField {
	case SortDefault:
		m.sortField = SortNAV
	case SortNAV:
		m.sortField = SortDayChange
	case SortDayChange:
		m.sortField = SortDefault
	}

	return m.sortFunds()
}

func (m Model) toggleSortDirection() Model {
	m.sortAsc = !m.sortAsc

	return m.sortFunds()
}

func (m Model) handleSortKey() Model {
	m = m.cycleSortMode()
	m.scrollOffset = 0
	m.cardCache = make(map[string]string)
	m.saveState()

	return m
}

func (m Model) handleSortDirectionKey() Model {
	m = m.toggleSortDirection()
	m.scrollOffset = 0
	m.cardCache = make(map[string]string)
	m.saveState()

	return m
}

func (m Model) handleSortModeKey(key string) Model {
	switch key {
	case "o":
		return m.handleSortKey()
	default:
		return m.handleSortDirectionKey()
	}
}

func (m Model) handleGroupKey(key string) Model {
	switch key {
	case "left", "h":
		return m.handlePrevGroup()
	default:
		return m.handleNextGroup()
	}
}

func (m Model) handleRefreshKey() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	m.loading = true
	m.errMsg = ""

	return m, m.fetchAllFundsCmd()
}

func (m Model) handleReloadKey() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}

	if m.hasUnsavedFunds {
		m.reloadConfirm = true
		m.errMsg = "有未保存的基金数据，按 Y 确认重新加载，按其他键取消"

		return m, nil
	}

	return m.handleReloadConfig()
}

func (m Model) handleReloadConfig() (tea.Model, tea.Cmd) {
	cfg := config.LoadConfig(m.configFilepath)
	m.config = cfg
	m.groups = cfg.Groups
	m.currentGroup = 0
	m.fundData = make(map[string]data.FundData)
	m.errMsg = ""
	m.scrollOffset = 0
	m.cardCache = make(map[string]string)
	m.sortField = SortDefault
	m.sortAsc = false
	m.hasUnsavedFunds = false
	m = m.loadGroupCacheIgnoreTTL()
	m = m.loadState()
	m.loading = true

	return m, m.fetchAllFundsCmd()
}

func (m Model) renderMain() string {
	sections := make([]string, 0, mainSectionsCap)

	sections = append(sections, "")

	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	sections = append(sections, selectorStr, "")

	sections = append(sections, m.renderFundsSection())

	sections = append(sections,
		"",
		m.renderStatusBar(),
		"",
		RenderFooter(m.width),
	)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderFundsSection() string {
	lastTradingDay := data.GetLastTradingDate(time.Now())
	numFunds := len(m.sortedFunds)

	numRows := (numFunds + cardsPerRow - 1) / cardsPerRow
	totalHeight := numRows * fundCardHeight
	available := m.availableHeight()

	cardWidth := m.computeCardWidth(totalHeight, available)

	if numFunds == 0 {
		return lipgloss.NewStyle().
			Width(m.width).
			Height(m.availableHeight()).
			Align(lipgloss.Center).
			Render("No funds in this group")
	}

	visibleStart, visibleEnd, offset := m.calcVisibleFundRange(numFunds, totalHeight, available)
	visibleFunds := m.sortedFunds[visibleStart:visibleEnd]
	rows := m.renderFundRows(visibleFunds, cardWidth, lastTradingDay)

	return m.renderScrollableRows(rows, totalHeight, available, offset)
}

func (m Model) computeCardWidth(totalHeight, available int) int {
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

	return cardWidth
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
		lipgloss.Height(RenderFooter(m.width)) + fixedSectionGaps + headerTopPadding

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
	var msg string

	switch {
	case m.clipboardMsg != "":
		msg = m.clipboardMsg
	case m.errMsg != "":
		return RenderStatusBar(m.errMsg, m.width, true)
	case m.loading:
		msg = "Loading..."
	case !m.lastRefresh.IsZero():
		msg = "上次更新: " + m.lastRefresh.Format("15:04:05")
	}

	if m.sortField != SortDefault {
		dir := "↓"
		if m.sortAsc {
			dir = "↑"
		}

		if msg != "" {
			msg += " | "
		}

		msg += "排序: " + m.sortFieldLabel() + " " + dir
	}

	if msg == "" {
		return ""
	}

	return RenderStatusBar(msg, m.width, false)
}
