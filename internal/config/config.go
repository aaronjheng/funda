package config

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

const (
	defaultRefreshInterval = 60
)

func defaultConfig() Config {
	return Config{
		Groups:          []Group{{Name: "全部", Funds: []Fund{}}},
		RefreshInterval: defaultRefreshInterval,
	}
}

func resolveConfigPath() string {
	cwd := filepath.Join(".", "funda.yaml")

	_, err := os.Stat(cwd)
	if err == nil {
		return cwd
	}

	cfg, err := xdg.SearchConfigFile("funda/funda.yaml")
	if err == nil {
		return cfg
	}

	return ""
}

// LoadConfig loads configuration from file or returns defaults.
func LoadConfig(cfgFilepath string) Config {
	cfg := defaultConfig()

	viperInstance := viper.New()
	viperInstance.SetDefault("refresh_interval", defaultRefreshInterval)

	var path string
	if cfgFilepath != "" {
		path = cfgFilepath
	} else {
		path = resolveConfigPath()
	}

	if path != "" {
		viperInstance.SetConfigFile(path)

		err := viperInstance.ReadInConfig()
		if err != nil {
			return cfg
		}
	}

	err := viperInstance.Unmarshal(&cfg)
	if err != nil {
		return cfg
	}

	cfg.Groups = buildAllGroup(cfg.Groups)

	return cfg
}

func buildAllGroup(groups []Group) []Group {
	var all Group

	all.Name = "全部"
	seen := make(map[string]struct{})

	for _, group := range groups {
		if group.Name == "全部" {
			continue
		}

		for _, fund := range group.Funds {
			if _, ok := seen[fund.Code]; !ok {
				seen[fund.Code] = struct{}{}
				all.Funds = append(all.Funds, fund)
			}
		}
	}

	result := make([]Group, 0, len(groups)+1)
	result = append(result, all)

	for _, group := range groups {
		if group.Name != "全部" {
			result = append(result, group)
		}
	}

	return result
}
