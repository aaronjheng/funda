package config

// Fund represents a single fund configuration.
type Fund struct {
	Code  string `mapstructure:"code"`
	Alias string `mapstructure:"alias"`
}

// Group represents a named collection of funds.
type Group struct {
	Name  string `mapstructure:"name"`
	Funds []Fund `mapstructure:"funds"`
}

// Config holds the full application configuration.
type Config struct {
	Groups          []Group `mapstructure:"groups"`
	RefreshInterval int     `mapstructure:"refresh_interval"`
}
