package management

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type Runner struct {
	RepoRoot       string
	MaxOutputBytes int
	MaxTimeout     time.Duration
}

func (r Runner) Run(ctx context.Context, action Action) ActionResult {
	startedAt := time.Now().UTC()
	result := ActionResult{
		ActionID:   action.ID,
		RiskTier:   action.RiskTier,
		ScriptPath: action.ScriptPath,
		StartedAt:  startedAt,
	}

	fullPath, err := safeScriptPath(r.RepoRoot, action.ScriptPath)
	if err != nil {
		result.Status = StatusFailed
		result.Stderr = "unsafe script path: " + err.Error()
		result.CompletedAt = time.Now().UTC()
		result.DurationMillis = result.CompletedAt.Sub(startedAt).Milliseconds()
		return result
	}

	timeout := effectiveTimeout(action.Timeout, r.MaxTimeout)
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(runCtx, "bash", fullPath)
	cmd.Dir = r.RepoRoot
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	result.CompletedAt = time.Now().UTC()
	result.DurationMillis = result.CompletedAt.Sub(startedAt).Milliseconds()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	result.Stdout, result.Stderr, result.OutputTruncated, result.RedactionsApplied = filterOutput(stdout.String(), stderr.String(), r.MaxOutputBytes)

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.Status = StatusTimedOut
		return result
	}
	if err != nil {
		result.Status = StatusFailed
		if result.Stderr == "" {
			result.Stderr = fmt.Sprintf("run script: %v", err)
		}
		return result
	}
	result.Status = StatusSucceeded
	return result
}

func effectiveTimeout(actionTimeout, maxTimeout time.Duration) time.Duration {
	if actionTimeout <= 0 {
		return maxTimeout
	}
	if maxTimeout > 0 && maxTimeout < actionTimeout {
		return maxTimeout
	}
	return actionTimeout
}

func filterOutput(stdout, stderr string, maxBytes int) (string, string, bool, int) {
	stdout, stdoutTruncated := boundOutput(stdout, maxBytes)
	stderr, stderrTruncated := boundOutput(stderr, maxBytes)
	stdout, stdoutRedactions := redactOutput(stdout)
	stderr, stderrRedactions := redactOutput(stderr)
	return stdout, stderr, stdoutTruncated || stderrTruncated, stdoutRedactions + stderrRedactions
}
