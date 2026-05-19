package view

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func Run(cfg config.Config, fetcher *data.Fetcher) error {
	p := tea.NewProgram(
		NewModel(cfg, fetcher),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("run view: %w", err)
	}

	return nil
}
