package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFailedStartupTerminatesContainer(t *testing.T) {
	for _, backend := range []Backend{BackendEmulator, BackendOmni} {
		t.Run(string(backend), func(t *testing.T) {
			if backend == BackendOmni {
				// Lifecycle probe only: the container image is the Cloud Spanner
				// emulator, not a Spanner Omni readiness check.
				t.Log("Omni startup path uses the emulator image to force a readiness failure")
			}
			label := startupLabel(t)
			t.Cleanup(func() { removeLabeledContainers(t, label) })
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			_, err := Run(ctx, backend, failedStartupOptions(label, 500*time.Millisecond)...)
			if err == nil {
				t.Fatal("Run() error = nil, want startup failure")
			}
			if ids := labeledContainerIDs(t, label); len(ids) != 0 {
				t.Fatalf("containers remaining after failed startup: %d (%v)", len(ids), ids)
			}
		})
	}
}

func TestCanceledReadinessTerminatesCreatedContainer(t *testing.T) {
	for _, backend := range []Backend{BackendEmulator, BackendOmni} {
		t.Run(string(backend), func(t *testing.T) {
			if backend == BackendOmni {
				t.Log("Omni startup path uses the emulator image; this is not Omni readiness evidence")
			}
			label := startupLabel(t)
			t.Cleanup(func() { removeLabeledContainers(t, label) })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			seen := make(chan struct{})
			stopPoll := make(chan struct{})
			defer close(stopPoll)
			go func() {
				deadline := time.Now().Add(20 * time.Second)
				for time.Now().Before(deadline) {
					select {
					case <-stopPoll:
						return
					default:
					}
					ids, err := listLabeledContainerIDs(label)
					if err == nil && len(ids) > 0 {
						close(seen)
						cancel()
						return
					}
					time.Sleep(50 * time.Millisecond)
				}
			}()

			_, err := Run(ctx, backend, failedStartupOptions(label, 30*time.Second)...)
			if err == nil {
				t.Fatal("Run() error = nil, want cancellation during readiness")
			}
			if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("Run() error = %v, want the startup context cancellation preserved", err)
			}
			select {
			case <-seen:
			case <-time.After(5 * time.Second):
				t.Fatal("startup returned before a created container was observed")
			}
			if ids := labeledContainerIDs(t, label); len(ids) != 0 {
				t.Fatalf("containers remaining after canceled readiness: %d (%v)", len(ids), ids)
			}
		})
	}
}

func startupLabel(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("spanemuboost-f3-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
}

func failedStartupOptions(label string, waitTimeout time.Duration) []Option {
	return []Option{
		WithContainerImage(DefaultEmulatorImage),
		DisableAutoConfig(),
		WithContainerCustomizers(
			testcontainers.WithLabels(map[string]string{"spanemuboost.review": label}),
			testcontainers.CustomizeRequestOption(func(req *testcontainers.GenericContainerRequest) error {
				req.Cmd = []string{"./gateway_main", "--hostname", "0.0.0.0"}
				return nil
			}),
			testcontainers.WithWaitStrategy(wait.ForLog("THIS_STARTUP_MARKER_IS_NEVER_EMITTED").WithStartupTimeout(waitTimeout)),
		),
	}
}

func labeledContainerIDs(t *testing.T, label string) []string {
	t.Helper()
	ids, err := listLabeledContainerIDs(label)
	if err != nil {
		t.Fatalf("list labeled containers: %v", err)
	}
	return ids
}

// listLabeledContainerIDs uses the Testcontainers Docker provider client, which
// resolves the same daemon endpoint as container startup. It does not invoke
// the docker CLI or a Docker context.
func listLabeledContainerIDs(label string) ([]string, error) {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return nil, err
	}
	defer func() { _ = provider.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	listed, err := provider.Client().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", "spanemuboost.review="+label),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(listed.Items))
	for _, item := range listed.Items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func removeLabeledContainers(t *testing.T, label string) {
	t.Helper()
	ids, err := listLabeledContainerIDs(label)
	if err != nil {
		t.Errorf("list labeled containers: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Errorf("provider: %v", err)
		return
	}
	defer func() { _ = provider.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, id := range ids {
		if _, err := provider.Client().ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil {
			t.Errorf("remove container %s: %v", id, err)
		}
	}
}
