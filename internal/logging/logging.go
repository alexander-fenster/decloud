package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/alexander-fenster/decloud/internal/config"
)

// Init configures the default slog handler. JSON output goes to stderr and,
// when filesystem access permits, also to <root>/logs/decloud.log. If root
// is the empty string, config.DefaultRoot is used (matching config.NewPaths
// semantics). If the log directory cannot be created OR the log file cannot
// be opened, Init falls back to stderr-only and emits one warning line to
// stderr describing the cause.
//
// DECLOUD_LOG_TO_STDERR_ONLY=1 short-circuits before any filesystem access
// and is the deterministic test escape hatch.
func Init(root string) error {
	if os.Getenv("DECLOUD_LOG_TO_STDERR_ONLY") == "1" {
		setStderrOnly()
		return nil
	}
	if root == "" {
		root = config.DefaultRoot
	}
	logsDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "decloud: log dir unavailable, using stderr only: %v\n", err)
		setStderrOnly()
		return nil
	}
	logPath := filepath.Join(logsDir, "decloud.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decloud: log file unavailable, using stderr only: %v\n", err)
		setStderrOnly()
		return nil
	}
	w := io.MultiWriter(os.Stderr, f)
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return nil
}

func setStderrOnly() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
}
