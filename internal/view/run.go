package view

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func Run(cfg config.Config, fetcher *data.Fetcher, configFilepath string, logger *slog.Logger) error {
	p := tea.NewProgram(
		NewModel(cfg, fetcher, configFilepath, logger),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("run view: %w", err)
	}

	return nil
}
