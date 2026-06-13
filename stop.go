package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultStopTimeout = 30 * time.Second

// StopConfig configures [StopFromConfig].
type StopConfig struct {
	EndpointFile string
	PIDFile      string
	Timeout      time.Duration
}

// StopFromConfig sends SIGTERM to a spanemuboost serve process and waits for it
// to exit. The process is identified by StopConfig.PIDFile or lifecycle metadata
// in StopConfig.EndpointFile. Remote or manually started endpoints without a PID
// cannot be stopped through this API.
func StopFromConfig(ctx context.Context, cfg StopConfig) error {
	pid, err := resolveServePID(cfg)
	if err != nil {
		return err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultStopTimeout
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("spanemuboost: find serve process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if isProcessDone(err) {
			return finishStopCleanup(cfg)
		}
		return fmt.Errorf("spanemuboost: signal serve process %d: %w", pid, err)
	}

	deadline := time.Now().Add(cfg.Timeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return finishStopCleanup(cfg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	if err := proc.Signal(syscall.SIGKILL); err == nil {
		for time.Now().Before(deadline) {
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				return finishStopCleanup(cfg)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	return fmt.Errorf("spanemuboost: serve process %d did not exit within %s", pid, cfg.Timeout)
}

func finishStopCleanup(cfg StopConfig) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return waitForEndpointCleanup(cleanupCtx, cfg, time.Now().Add(time.Second))
}

func isProcessDone(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || strings.Contains(err.Error(), "no such process")
}

func resolveServePID(cfg StopConfig) (int, error) {
	if path := strings.TrimSpace(cfg.PIDFile); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("spanemuboost: read pid file %q: %w", path, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			return 0, fmt.Errorf("spanemuboost: invalid pid in %q", path)
		}
		return pid, nil
	}
	if path := strings.TrimSpace(cfg.EndpointFile); path != "" {
		endpoint, err := ReadEndpointFile(path)
		if err != nil {
			return 0, err
		}
		if endpoint.PID <= 0 {
			return 0, fmt.Errorf("spanemuboost: endpoint file %q has no managed serve pid", path)
		}
		return endpoint.PID, nil
	}
	return 0, fmt.Errorf("spanemuboost: --endpoint-file or --pid-file is required")
}

func waitForEndpointCleanup(ctx context.Context, cfg StopConfig, deadline time.Time) error {
	if path := strings.TrimSpace(cfg.PIDFile); path != "" {
		_ = os.Remove(path)
	}
	endpointPath := strings.TrimSpace(cfg.EndpointFile)
	if endpointPath == "" {
		return nil
	}
	cleanupDeadline := time.Now().Add(time.Second)
	if cleanupDeadline.After(deadline) {
		cleanupDeadline = deadline
	}
	for time.Now().Before(cleanupDeadline) {
		if _, err := os.Stat(endpointPath); os.IsNotExist(err) {
			return nil
		}
		select {
		case <-ctx.Done():
			goto removeStale
		case <-time.After(50 * time.Millisecond):
		}
	}
removeStale:
	if err := os.Remove(endpointPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("spanemuboost: remove stale endpoint file %q: %w", endpointPath, err)
	}
	return nil
}

// ParseStopArgs parses `spanemuboost stop --endpoint-file path [--pid-file path]`.
func ParseStopArgs(args []string) (StopConfig, error) {
	cfg := StopConfig{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint-file", "-o":
			if i+1 >= len(args) {
				return StopConfig{}, fmt.Errorf("--endpoint-file requires a value")
			}
			cfg.EndpointFile = args[i+1]
			i++
		case "--pid-file":
			if i+1 >= len(args) {
				return StopConfig{}, fmt.Errorf("--pid-file requires a value")
			}
			cfg.PIDFile = args[i+1]
			i++
		default:
			return StopConfig{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if cfg.EndpointFile == "" && cfg.PIDFile == "" {
		return StopConfig{}, fmt.Errorf("usage: spanemuboost stop --endpoint-file path [--pid-file path]")
	}
	return cfg, nil
}
