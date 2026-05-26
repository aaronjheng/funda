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

// Config holds the full application configuration.
type Config struct {
	Groups          []Group `yaml:"groups"`
	RefreshInterval int     `yaml:"refresh_interval"`
}
