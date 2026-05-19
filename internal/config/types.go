package config

// Fund represents a single fund configuration.
type Fund struct {
	Code  string `yaml:"code"`
	Alias string `yaml:"alias"`
}

// Group represents a named collection of funds.
type Group struct {
	Name  string `yaml:"name"`
	Funds []Fund `yaml:"funds"`
}

// Alerts holds alert configuration.
type Alerts struct {
	HighlightThreshold float64 `yaml:"highlight_threshold"` //nolint:tagliatelle
}

// Config holds the full application configuration.
type Config struct {
	Groups          []Group `yaml:"groups"`
	RefreshInterval int     `yaml:"refresh_interval"` //nolint:tagliatelle
	Alerts          Alerts  `yaml:"alerts"`
}
