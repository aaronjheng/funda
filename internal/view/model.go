package view

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

const (
	defaultRefreshIntervalSec   = 60
	navPostCloseRefreshInterval = 5 * time.Minute
	navFallbackRefreshInterval  = 1 * time.Hour
	cardPaddingWidth            = 2
	cardsPerRow                 = 2
	minCardWidth                = 20
	labelWidth                  = 12
	valueWidthOffset            = 14
	sortStateDirPermissions     = 0o700
	sortStateFilePermissions    = 0o600
	cardFrameWidth              = 4 // border(2) + horizontal padding(2)
	clipboardDisplayDuration    = 1 * time.Second
	fundCardContentLines        = 5 // title, nav, change, estimate, time
	fundCardBorderLines         = 2
	fundCardHeight              = fundCardContentLines + fundCardBorderLines // 6
	mainSectionsCap             = 8
	toastVerticalDiv            = 3
	toastCenterDiv              = 2
	helpPaddingHorizontal       = 2
	helpCenterDiv               = 2
	helpVerticalDiv             = 3
	searchContentPadding        = 4 // border(2) + padding(2)
	searchOverlayBorder         = 2
	searchOverlayHeightOffset   = 6 // border(2) + padding(2) + margin(2)
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

type estimatesFetchedMsg struct {
	estimates map[string]data.EstimateUpdate
}

type searchResultMsg struct {
	results    []data.SearchHit
	err        error
	generation int
}

type clearClipboardMsg struct{}

type searchItem struct {
	data.SearchHit
}

func (i searchItem) Title() string       { return i.Code + "  " + i.Name }
func (i searchItem) Description() string { return i.Price + "  " + i.Change }
func (i searchItem) FilterValue() string { return i.Code + i.Name }

type Model struct {
	config           config.Config
	groups           []config.Group
	currentGroup     int
	fundData         map[string]data.FundData
	loading          bool
	errMsg           string
	width            int
	height           int
	fetcher          *data.Fetcher
	logger           *slog.Logger
	searchMode       bool
	keymap           KeyMap
	helpModel        help.Model
	showHelp         bool
	lastRefresh      time.Time
	lastFullRefresh  time.Time
	viewport         viewport.Model
	clipboardMsg     string
	copiedCode       string
	toastMsg         string
	cardCache        map[string]string
	sortField        SortField
	sortAsc          bool
	sortedFunds      []config.Fund
	configFilepath   string
	hasUnsavedFunds  bool
	reloadConfirm    bool
	textInput        textinput.Model
	searchList       list.Model
	lastSearchQuery  string
	searchGeneration int
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

func newSearchList() list.Model {
	delegate := list.NewDefaultDelegate()
	searchList := list.New([]list.Item{}, delegate, 0, 0)
	searchList.SetShowHelp(false)
	searchList.SetShowStatusBar(false)
	searchList.SetShowTitle(false)
	searchList.SetShowFilter(false)
	searchList.SetShowPagination(false)
	searchList.SetFilteringEnabled(false)
	searchList.KeyMap.Quit.SetEnabled(false)
	searchList.KeyMap.ForceQuit.SetEnabled(false)
	searchList.KeyMap.Filter.SetEnabled(false)
	searchList.KeyMap.ClearFilter.SetEnabled(false)
	searchList.KeyMap.ShowFullHelp.SetEnabled(false)
	searchList.KeyMap.CloseFullHelp.SetEnabled(false)
	searchList.DisableQuitKeybindings()

	return searchList
}

func newViewportComponent() viewport.Model {
	comp := viewport.New()
	comp.KeyMap.Up.SetEnabled(false)
	comp.KeyMap.Down.SetEnabled(false)
	comp.KeyMap.PageUp.SetEnabled(false)
	comp.KeyMap.PageDown.SetEnabled(false)
	comp.KeyMap.HalfPageUp.SetEnabled(false)
	comp.KeyMap.HalfPageDown.SetEnabled(false)

	return comp
}

func NewModel(cfg config.Config, fetcher *data.Fetcher, configFilepath string, logger *slog.Logger) Model {
	logger.Info("funda starting", "groups", len(cfg.Groups))

	textInput := textinput.New()
	textInput.Prompt = "Search: "
	textInput.Placeholder = "fund code or name..."
	textInput.CharLimit = 20

	model := Model{
		config:           cfg,
		groups:           cfg.Groups,
		currentGroup:     0,
		fundData:         make(map[string]data.FundData),
		loading:          false,
		errMsg:           "",
		width:            0,
		height:           0,
		fetcher:          fetcher,
		logger:           logger,
		searchMode:       false,
		keymap:           DefaultKeyMap(),
		helpModel:        help.New(),
		showHelp:         false,
		lastRefresh:      time.Time{},
		lastFullRefresh:  time.Time{},
		viewport:         newViewportComponent(),
		clipboardMsg:     "",
		copiedCode:       "",
		toastMsg:         "",
		cardCache:        make(map[string]string),
		sortField:        SortDefault,
		sortAsc:          false,
		sortedFunds:      nil,
		configFilepath:   configFilepath,
		hasUnsavedFunds:  false,
		reloadConfirm:    false,
		textInput:        textInput,
		searchList:       newSearchList(),
		lastSearchQuery:  "",
		searchGeneration: 0,
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

	if m.searchMode {
		return m.renderSearchView()
	}

	view := m.renderMain()

	if m.showHelp {
		helpContent := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(accentColor)).
			Padding(1, helpPaddingHorizontal).
			Render(m.helpModel.View(m.keymap))

		helpW := lipgloss.Width(helpContent)
		helpH := lipgloss.Height(helpContent)

		mainLayer := lipgloss.NewLayer(view)
		helpLayer := lipgloss.NewLayer(helpContent).
			X((m.width - helpW) / helpCenterDiv).
			Y((m.height - helpH) / helpVerticalDiv)

		comp := lipgloss.NewCompositor(mainLayer, helpLayer)
		view = comp.Render()
	} else if m.toastMsg != "" {
		toastContent := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(accentColor)).
			Padding(1, helpCenterDiv).
			Render(truncateWidth(m.toastMsg, toastMaxWidth))

		toastW := lipgloss.Width(toastContent)
		toastH := lipgloss.Height(toastContent)

		mainLayer := lipgloss.NewLayer(view)
		toastLayer := lipgloss.NewLayer(toastContent).
			X((m.width - toastW) / toastCenterDiv).
			Y((m.height - toastH) / toastVerticalDiv)

		comp := lipgloss.NewCompositor(mainLayer, toastLayer)
		view = comp.Render()
	}

	v := tea.NewView(view)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}

func (m Model) renderSearchView() tea.View {
	m.textInput.SetWidth(m.width - searchContentPadding)
	m.searchList.SetSize(m.width-searchContentPadding, m.height-searchOverlayHeightOffset)

	content := lipgloss.JoinVertical(lipgloss.Left,
		m.textInput.View(),
		m.searchList.View(),
	)

	styled := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accentColor)).
		Padding(1, 1).
		Width(m.width - searchOverlayBorder).
		Height(m.height - 1).
		Render(content)

	v := tea.NewView(styled)
	v.AltScreen = true

	return v
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
	m.viewport.GotoTop()
	m.cardCache = make(map[string]string)
	m.saveState()

	return m
}

func (m Model) handleSortDirectionKey() Model {
	m = m.toggleSortDirection()
	m.viewport.GotoTop()
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

func (m Model) handleGroupKey(key string) (Model, tea.Cmd) {
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

	m.logger.Info("user triggered manual refresh")

	group := m.groups[m.currentGroup]
	codes := make([]string, 0, len(group.Funds))

	for _, fund := range group.Funds {
		codes = append(codes, fund.Code)
	}

	m.fetcher.RemoveCachedEntries(codes)

	m.cardCache = make(map[string]string)
	m.loading = true
	m.errMsg = ""
	m.lastFullRefresh = time.Now()

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
	m.logger.Info("reloading config")

	cfg := config.LoadConfig(m.configFilepath)
	m.config = cfg
	m.groups = cfg.Groups
	m.currentGroup = 0
	m.fundData = make(map[string]data.FundData)
	m.errMsg = ""
	m.viewport.GotoTop()
	m.cardCache = make(map[string]string)
	m.sortField = SortDefault
	m.sortAsc = false
	m.hasUnsavedFunds = false
	m = m.loadGroupCacheIgnoreTTL()
	m = m.loadState()
	m.loading = true
	m.lastFullRefresh = time.Now()

	return m, m.fetchAllFundsCmd()
}

func (m Model) syncViewport() Model {
	scrollbarReserve := 2
	m.viewport.SetWidth(m.width - scrollbarReserve)
	m.viewport.SetHeight(m.availableHeight())

	lastTradingDay := data.GetLastTradingDate(time.Now())
	cardWidth := m.computeCardWidth()
	fundsContent := m.renderFundsContent(cardWidth, lastTradingDay)
	m.viewport.SetContent(fundsContent)

	return m
}

func (m Model) renderMain() string {
	sections := make([]string, 0, mainSectionsCap)

	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	sections = append(sections, selectorStr)

	scrollbarReserve := 2
	m.viewport.SetWidth(m.width - scrollbarReserve)
	m.viewport.SetHeight(m.availableHeight())

	lastTradingDay := data.GetLastTradingDate(time.Now())
	cardWidth := m.computeCardWidth()
	fundsContent := m.renderFundsContent(cardWidth, lastTradingDay)
	m.viewport.SetContent(fundsContent)

	viewportStr := m.viewport.View()
	if s := RenderScrollbar(m.viewport); s != "" {
		viewportStr = lipgloss.JoinHorizontal(lipgloss.Top, viewportStr, " ", s)
	}

	sections = append(sections, viewportStr)

	sections = append(sections,
		"",
		m.renderStatusBar(),
	)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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

func (m Model) availableHeight() int {
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width)
	fixed := 1 + lipgloss.Height(selectorStr) + 1 + lipgloss.Height(m.renderStatusBar())

	return max(0, m.height-fixed)
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
