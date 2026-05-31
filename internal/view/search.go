package view

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

type searchItem struct {
	data.SearchHit
}

func (i searchItem) Title() string       { return i.Code + "  " + i.Name }
func (i searchItem) Description() string { return i.Price + "  " + i.Change }
func (i searchItem) FilterValue() string { return i.Code + i.Name }

type searchResultMsg struct {
	results    []data.SearchHit
	err        error
	generation int
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

func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.textInput.Reset()
		m.lastSearchQuery = ""
		m.searchGeneration = 0
		m.searchList.ResetSelected()

		return m, nil
	case "enter":
		return m.selectSearchResult()
	default:
		return m.handleSearchInput(msg)
	}
}

func (m Model) selectSearchResult() (tea.Model, tea.Cmd) {
	if item := m.searchList.SelectedItem(); item != nil {
		if si, ok := item.(searchItem); ok {
			m.logger.Info("search result selected", "code", si.Code, "name", si.Name)
			m = m.addFundToAll(si.Code, si.Name)
		}
	}

	m.searchMode = false
	m.textInput.Reset()
	m.searchList.ResetSelected()
	m.lastSearchQuery = ""

	return m, m.fetchAllFundsCmd()
}

func (m Model) handleSearchInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m = m.applySearchDimensions()

	var cmds []tea.Cmd

	newTI, tiCmd := m.textInput.Update(msg)
	m.textInput = newTI

	if tiCmd != nil {
		cmds = append(cmds, tiCmd)
	}

	newList, listCmd := m.searchList.Update(msg)
	m.searchList = newList

	if listCmd != nil {
		cmds = append(cmds, listCmd)
	}

	currentQuery := m.textInput.Value()
	if currentQuery != m.lastSearchQuery {
		m.lastSearchQuery = currentQuery
		if len(currentQuery) > 0 {
			m.searchGeneration++
			gen := m.searchGeneration

			cmds = append(cmds, m.searchFundCmd(currentQuery, gen))
		} else {
			m.searchList.SetItems(nil)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) applySearchDimensions() Model {
	m.textInput.SetWidth(m.width - searchContentPadding)
	m.searchList.SetSize(m.width-searchContentPadding, m.height-searchOverlayHeightOffset)

	return m
}

func (m Model) handleSearchResult(msg searchResultMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.searchGeneration {
		return m, nil
	}

	if msg.err != nil {
		m.errMsg = msg.err.Error()
	} else {
		items := make([]list.Item, len(msg.results))
		for i, hit := range msg.results {
			items[i] = searchItem{hit}
		}

		m.searchList.SetItems(items)
	}

	return m, nil
}

func (m Model) addFundToAll(code, name string) Model {
	for idx := range m.groups {
		if m.groups[idx].Name == "全部" {
			for _, fund := range m.groups[idx].Funds {
				if fund.Code == code {
					return m
				}
			}

			m.groups[idx].Funds = append(m.groups[idx].Funds, config.Fund{Code: code, Alias: name})
			m.hasUnsavedFunds = true

			m.logger.Info("fund added to all group", "code", code, "name", name)

			return m
		}
	}

	return m
}

func (m Model) searchFundCmd(query string, generation int) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		results, err := m.fetcher.SearchFund(ctx, query)

		return searchResultMsg{results: results, err: err, generation: generation}
	}
}
