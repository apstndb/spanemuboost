package spanemuboost

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	switch os.Getenv("SPANEMUBOOST_STOP_TEST_HELPER") {
	case "ignore-term":
		runStopTestHelper(true)
	case "exit-on-term":
		runStopTestHelper(false)
	default:
		os.Exit(m.Run())
	}
}

func runStopTestHelper(ignoreTerm bool) {
	// Install the disposition before announcing readiness. StopFromConfig can
	// signal as soon as it reads "ready", and the default SIGTERM action would
	// otherwise terminate the graceful helper.
	if ignoreTerm {
		signal.Ignore(syscall.SIGTERM)
		fmt.Println("ready")
		_ = os.Stdout.Sync()
		select {}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	fmt.Println("ready")
	_ = os.Stdout.Sync()
	<-ch
}

func TestStopFromConfigKillsProcessThatIgnoresSIGTERM(t *testing.T) {
	_, done, pidPath, endpointPath := startStopHelper(t, "ignore-term")
	err := StopFromConfig(context.Background(), StopConfig{
		EndpointFile: endpointPath,
		PIDFile:      pidPath,
		Timeout:      200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("StopFromConfig() error = %v", err)
	}
	waitHelperExit(t, done, "signal: killed")
	assertStopMetadataRemoved(t, pidPath, endpointPath)
}

func TestStopFromConfigCleansUpAfterGracefulExit(t *testing.T) {
	_, done, pidPath, endpointPath := startStopHelper(t, "exit-on-term")
	err := StopFromConfig(context.Background(), StopConfig{
		EndpointFile: endpointPath,
		PIDFile:      pidPath,
		Timeout:      2 * time.Second,
	})
	if err != nil {
		t.Fatalf("StopFromConfig() error = %v", err)
	}
	waitHelperExit(t, done, "")
	assertStopMetadataRemoved(t, pidPath, endpointPath)
}

func TestStopFromConfigHonorsCanceledContext(t *testing.T) {
	_, _, pidPath, endpointPath := startStopHelper(t, "ignore-term")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := StopFromConfig(ctx, StopConfig{
		EndpointFile: endpointPath,
		PIDFile:      pidPath,
		Timeout:      2 * time.Second,
	})
	if err == nil || err != context.Canceled {
		t.Fatalf("StopFromConfig() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(pidPath); statErr != nil {
		t.Fatalf("pid file removed on canceled stop: %v", statErr)
	}
	if _, statErr := os.Stat(endpointPath); statErr != nil {
		t.Fatalf("endpoint file removed on canceled stop: %v", statErr)
	}
}

func startStopHelper(t *testing.T, mode string) (*exec.Cmd, <-chan error, string, string) {
	t.Helper()
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	endpointPath := filepath.Join(dir, "endpoint.json")
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "SPANEMUBOOST_STOP_TEST_HELPER="+mode)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper error = %v", err)
	}
	// Reap in the background. A zombie still answers signal 0, so Stop would
	// otherwise treat an exited child as still running. The exit error is
	// delivered on done; Cmd.ProcessState is not read concurrently with Wait.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			ready <- ""
			return
		}
		ready <- scanner.Text()
	}()
	select {
	case text := <-ready:
		if text != "ready" {
			t.Fatalf("helper not ready: %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not become ready within 5s")
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o600); err != nil {
		t.Fatalf("WriteFile(pid) error = %v", err)
	}
	endpoint := Endpoint{
		Backend:    BackendEmulator,
		URI:        "127.0.0.1:9010",
		ProjectID:  DefaultProjectID,
		InstanceID: DefaultInstanceID,
		PID:        cmd.Process.Pid,
		ManagedBy:  "spanemuboost serve",
	}
	if err := SaveEndpoint(endpointPath, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}
	return cmd, done, pidPath, endpointPath
}

func waitHelperExit(t *testing.T, done <-chan error, want string) {
	t.Helper()
	select {
	case err := <-done:
		if want == "" {
			if err != nil {
				t.Fatalf("helper exit = %v, want nil", err)
			}
			return
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("helper exit = %v, want %q", err, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("helper still running")
	}
}

func assertStopMetadataRemoved(t *testing.T, pidPath, endpointPath string) {
	t.Helper()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists: err=%v", err)
	}
	if _, err := os.Stat(endpointPath); !os.IsNotExist(err) {
		t.Fatalf("endpoint file still exists: err=%v", err)
	}
}

func TestStopFromConfigCleansStaleEndpointWhenProcessDead(t *testing.T) {
	dir := t.TempDir()
	endpointPath := filepath.Join(dir, "endpoint.json")
	pidPath := filepath.Join(dir, "serve.pid")
	endpoint := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
		ManagedBy:  "spanemuboost serve",
		PID:        999999,
	}
	if err := SaveEndpoint(endpointPath, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(pid) error = %v", err)
	}

	if err := StopFromConfig(context.Background(), StopConfig{
		EndpointFile: endpointPath,
		PIDFile:      pidPath,
	}); err != nil {
		t.Fatalf("StopFromConfig() error = %v", err)
	}
	if _, err := os.Stat(endpointPath); !os.IsNotExist(err) {
		t.Fatalf("endpoint file still exists: err=%v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists: err=%v", err)
	}
}
