package view

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

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
