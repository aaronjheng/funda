package log

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const (
	logDirPermissions  = 0o700
	logFilePermissions = 0o600
)

func New() *slog.Logger {
	dir := filepath.Join(xdg.StateHome, "funda")
	_ = os.MkdirAll(dir, logDirPermissions)

	file, err := os.OpenFile(
		filepath.Join(dir, "funda.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		logFilePermissions,
	)
	if err != nil {
		return slog.New(slog.DiscardHandler)
	}

	return slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
