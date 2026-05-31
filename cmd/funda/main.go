package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/aaronjheng/funda/internal/config"
	"github.com/aaronjheng/funda/internal/data"
	"github.com/aaronjheng/funda/internal/eastmoney"
	"github.com/aaronjheng/funda/internal/sina"
	"github.com/aaronjheng/funda/internal/telemetry/log"
	"github.com/aaronjheng/funda/internal/view"
)

const httpClientTimeout = 30 * time.Second

func rootCmd() *cobra.Command {
	var cfg config.Config

	var cfgFilepath string

	cmd := &cobra.Command{
		Use:          "funda",
		Short:        "A terminal UI tool for tracking fund valuation data",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if _, ok := cmd.Annotations["skipConfigLoad"]; ok {
				return nil
			}

			var err error

			cfgFilepath, err = cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("config flag error: %w", err)
			}

			cfg = config.LoadConfig(cfgFilepath)

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			logger, logCleanup := log.New()
			defer func() { _ = logCleanup() }()

			httpClient := &http.Client{Timeout: httpClientTimeout}
			eastMoneyClient := eastmoney.NewAPIClient(httpClient, logger)
			sinaClient := sina.NewAPIClient(httpClient, logger)
			fetcher := data.NewFetcher(eastMoneyClient, sinaClient, logger)

			return view.Run(cfg, fetcher, cfgFilepath, logger)
		},
	}

	cmd.SetHelpCommand(&cobra.Command{Hidden: true})

	cmd.PersistentFlags().StringP("config", "f", "", "Config file path.")

	cmd.AddCommand(versionCmd())

	return cmd
}

func main() {
	err := rootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}
