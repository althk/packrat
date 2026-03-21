package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/harish/packrat/internal/config"
)

// RunHooks executes hooks that match the given "when" phase.
func RunHooks(ctx context.Context, hooks []config.HookConfig, when string) error {
	for _, h := range hooks {
		if h.When != when {
			continue
		}

		slog.Info("running hook", "name", h.Name, "when", when)

		if err := runHook(ctx, h); err != nil {
			if h.FailAction == "abort" {
				return fmt.Errorf("hook %q failed (aborting): %w", h.Name, err)
			}
			slog.Warn("hook failed (continuing)", "name", h.Name, "error", err)
		}
	}
	return nil
}

func runHook(ctx context.Context, h config.HookConfig) error {
	timeout := 30 * time.Second
	if h.Timeout != "" {
		d, err := time.ParseDuration(h.Timeout)
		if err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	slog.Debug("hook completed", "name", h.Name, "output", string(output))
	return nil
}
