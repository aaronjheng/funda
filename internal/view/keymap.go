package view

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Quit         key.Binding
	Refresh      key.Binding
	ReloadConfig key.Binding
	ClearCache   key.Binding
	Sort         key.Binding
	Help         key.Binding
	Up           key.Binding
	Down         key.Binding
	Left         key.Binding
	Right        key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Help,
		k.Quit, k.Refresh, k.Sort,
		k.Up, k.Down, k.Left, k.Right,
	}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			k.Quit,
			k.Refresh,
			k.ReloadConfig,
			k.ClearCache,
			k.Sort,
			k.Help,
			k.Up,
			k.Down,
			k.Left,
			k.Right,
		},
	}
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Refresh:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		ReloadConfig: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload config")),
		ClearCache:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear cache")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Sort:         key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "sort")),
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:         key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev group")),
		Right:        key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next group")),
	}
}
