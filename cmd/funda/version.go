package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aaronjheng/funda/internal/version"
)

//go:embed VERSION
var embeddedVersion string

func versionCmd() *cobra.Command {
	versionStr := strings.TrimRight(embeddedVersion, "\n")
	cmd := &cobra.Command{
		Use:         "version",
		Short:       "Print the version information",
		Annotations: map[string]string{"skipConfigLoad": "true"},
		Run: func(_ *cobra.Command, _ []string) {
			commit := version.BuildCommit()
			if commit == "" {
				_, _ = fmt.Fprintf(os.Stdout, "%s (%s %s/%s)\n", versionStr, runtime.Version(), runtime.GOOS, runtime.GOARCH)

				return
			}

			_, _ = fmt.Fprintf(
				os.Stdout,
				"%s (%s %s/%s) (commit: %s)\n",
				versionStr,
				runtime.Version(),
				runtime.GOOS,
				runtime.GOARCH,
				commit,
			)
		},
	}

	return cmd
}
