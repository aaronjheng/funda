package view

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
)

func Run(cfg config.Config, fetcher *data.Fetcher, configFilepath string, logger *slog.Logger) error {
	model := NewModel(cfg, fetcher, configFilepath, logger)

	program := tea.NewProgram(
		model,
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			switch msg.(type) {
			case tea.QuitMsg, tea.InterruptMsg:
				if mm, ok := m.(Model); ok {
					mm.cancel()
				}
			}

			return msg
		}),
	)

	_, err := program.Run()
	if err != nil {
		return fmt.Errorf("run view: %w", err)
	}

	return nil
}
