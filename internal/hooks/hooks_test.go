package hooks

import (
	"context"
	"testing"

	"github.com/harish/packrat/internal/config"
)

func TestRunHooksSuccess(t *testing.T) {
	hooks := []config.HookConfig{
		{
			Name:       "test-hook",
			When:       "pre-backup",
			Command:    "echo hello",
			Timeout:    "5s",
			FailAction: "continue",
		},
	}

	err := RunHooks(context.Background(), hooks, "pre-backup")
	if err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
}

func TestRunHooksSkipWrongPhase(t *testing.T) {
	hooks := []config.HookConfig{
		{
			Name:       "post-only",
			When:       "post-backup",
			Command:    "false", // would fail
			FailAction: "abort",
		},
	}

	// Running pre-backup should skip the post-backup hook
	err := RunHooks(context.Background(), hooks, "pre-backup")
	if err != nil {
		t.Fatalf("should skip post-backup hook: %v", err)
	}
}

func TestRunHooksAbortOnFailure(t *testing.T) {
	hooks := []config.HookConfig{
		{
			Name:       "failing-hook",
			When:       "pre-backup",
			Command:    "false",
			Timeout:    "5s",
			FailAction: "abort",
		},
	}

	err := RunHooks(context.Background(), hooks, "pre-backup")
	if err == nil {
		t.Fatal("expected error from failing hook with abort action")
	}
}

func TestRunHooksContinueOnFailure(t *testing.T) {
	hooks := []config.HookConfig{
		{
			Name:       "failing-hook",
			When:       "pre-backup",
			Command:    "false",
			Timeout:    "5s",
			FailAction: "continue",
		},
	}

	err := RunHooks(context.Background(), hooks, "pre-backup")
	if err != nil {
		t.Fatalf("should continue on failure: %v", err)
	}
}

func TestRunHooksTimeout(t *testing.T) {
	hooks := []config.HookConfig{
		{
			Name:       "slow-hook",
			When:       "pre-backup",
			Command:    "sleep 10",
			Timeout:    "100ms",
			FailAction: "continue",
		},
	}

	err := RunHooks(context.Background(), hooks, "pre-backup")
	// Should not error because fail_action is continue
	if err != nil {
		t.Fatalf("should continue after timeout: %v", err)
	}
}
