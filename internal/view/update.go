package view

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

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

	case tickMsg:
		return m.handleTick()

	case allFundsFetchedMsg:
		return m.handleFundsFetched(msg)

	case searchResultMsg:
		return m.handleSearchResult(msg)
	}

	return m, nil
}

const scrollPageSize = 3

func (m Model) handleScrollKey(key string) Model {
	switch key {
	case "up", "k":
		m.scrollOffset = max(0, m.scrollOffset-1)
	case "down", "j":
		m.scrollOffset++
	case "pgup":
		m.scrollOffset = max(0, m.scrollOffset-scrollPageSize)
	case "pgdown":
		m.scrollOffset += scrollPageSize
	}

	return m
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
	case "left", "h":
		if m.currentGroup > 0 {
			m.currentGroup--
			m.scrollOffset = 0
			m = m.loadGroupCache()
		}
	case "right", "l":
		if m.currentGroup < len(m.groups)-1 {
			m.currentGroup++
			m.scrollOffset = 0
			m = m.loadGroupCache()
		}
	case "up", "k", "down", "j", "pgup", "pgdown":
		m = m.handleScrollKey(msg.String())
	}

	return m, nil
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
