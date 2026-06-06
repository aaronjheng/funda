package view

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

type clearClipboardMsg struct{}

type Model struct {
	//nolint:containedctx // context is passed through the Bubble Tea command chain
	ctx             context.Context
	cancel          context.CancelFunc
	config          config.Config
	groups          []config.Group
	currentGroup    int
	fundData        map[string]data.FundData
	loading         bool
	errMsg          string
	width           int
	height          int
	fetcher         *data.Fetcher
	logger          *slog.Logger
	keymap          KeyMap
	helpModel       help.Model
	showHelp        bool
	lastRefresh     time.Time
	lastFullRefresh time.Time
	viewport        viewport.Model
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
	colors          themeColors
}

func clearClipboardMsgCmd() tea.Cmd {
	return tea.Tick(clipboardDisplayDuration, func(_ time.Time) tea.Msg {
		return clearClipboardMsg{}
	})
}

func newViewportComponent() viewport.Model {
	comp := viewport.New()
	comp.KeyMap.Up.SetEnabled(false)
	comp.KeyMap.Down.SetEnabled(false)
	comp.KeyMap.PageUp.SetEnabled(false)
	comp.KeyMap.PageDown.SetEnabled(false)
	comp.KeyMap.HalfPageUp.SetEnabled(false)
	comp.KeyMap.HalfPageDown.SetEnabled(false)
	comp.FillHeight = true

	return comp
}

func NewModel(cfg config.Config, fetcher *data.Fetcher, configFilepath string, logger *slog.Logger) Model {
	logger.Info("funda starting", "groups", len(cfg.Groups))

	ctx, cancel := context.WithCancel(context.Background())

	model := Model{
		ctx:             ctx,
		cancel:          cancel,
		config:          cfg,
		groups:          cfg.Groups,
		currentGroup:    0,
		fundData:        make(map[string]data.FundData),
		loading:         false,
		errMsg:          "",
		width:           0,
		height:          0,
		fetcher:         fetcher,
		logger:          logger,
		keymap:          DefaultKeyMap(),
		helpModel:       help.New(),
		showHelp:        false,
		lastRefresh:     time.Time{},
		lastFullRefresh: time.Time{},
		viewport:        newViewportComponent(),
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
		colors:          themeForBackground(true),
	}
	model = model.loadGroupCacheIgnoreTTL()
	model = model.loadState()

	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.removeCachedAndFetch(),
		m.startTickCmd(),
		tea.RequestBackgroundColor,
	)
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	view := m.renderMain()

	if statusStr := m.renderStatusBar(); statusStr != "" {
		statusH := lipgloss.Height(statusStr)
		mainLayer := lipgloss.NewLayer(view)
		statusLayer := lipgloss.NewLayer(statusStr).
			Y(m.height - statusH)
		comp := lipgloss.NewCompositor(mainLayer, statusLayer)
		view = comp.Render()
	}

	if m.showHelp {
		helpContent := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.colors.accent)).
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
			BorderForeground(lipgloss.Color(m.colors.accent)).
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

func (m Model) removeCachedAndFetch() tea.Cmd {
	group := m.groups[m.currentGroup]

	codes := make([]string, 0, len(group.Funds))
	for _, fund := range group.Funds {
		codes = append(codes, fund.Code)
	}

	m.fetcher.RemoveCachedEntries(codes)
	m.loading = true
	m.lastFullRefresh = time.Now()

	return m.fetchAllFundsCmd()
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

	m.fundData = make(map[string]data.FundData)
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

	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width, m.colors)
	sections = append(sections, selectorStr)

	scrollbarReserve := 2
	m.viewport.SetWidth(m.width - scrollbarReserve)
	m.viewport.SetHeight(m.availableHeight())

	lastTradingDay := data.GetLastTradingDate(time.Now())
	cardWidth := m.computeCardWidth()
	fundsContent := m.renderFundsContent(cardWidth, lastTradingDay)
	m.viewport.SetContent(fundsContent)

	viewportStr := m.viewport.View()
	if s := RenderScrollbar(m.viewport, m.colors); s != "" {
		viewportStr = lipgloss.JoinHorizontal(lipgloss.Top, viewportStr, " ", s)
	}

	sections = append(sections, viewportStr)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) availableHeight() int {
	selectorStr, _ := RenderGroupSelector(m.groups, m.currentGroup, m.width, m.colors)

	top := 1 + lipgloss.Height(selectorStr)

	return max(0, m.height-top)
}

func (m Model) renderStatusBar() string {
	group := m.groups[m.currentGroup]
	left := fmt.Sprintf("%s (%d)", group.Name, len(group.Funds))

	var right string

	switch {
	case m.clipboardMsg != "":
		right = m.clipboardMsg
	case m.errMsg != "":
		return RenderStatusBar(left, m.errMsg, m.width, true, m.colors)
	case !m.lastRefresh.IsZero():
		right = "上次更新: " + m.lastRefresh.Format("15:04:05")
	case m.loading:
		right = "Loading..."
	}

	if m.sortField != SortDefault {
		dir := "↓"
		if m.sortAsc {
			dir = "↑"
		}

		if right != "" {
			right += " | "
		}

		right += "排序: " + m.sortFieldLabel() + " " + dir
	}

	if right == "" {
		return ""
	}

	return RenderStatusBar(left, right, m.width, false, m.colors)
}
