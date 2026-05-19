package spanemuboost

import (
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

func TestWithContainerProvider(t *testing.T) {
	opts, err := applyOptions(WithContainerProvider(testcontainers.ProviderPodman))
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if got := customizedProvider(t, opts); got != testcontainers.ProviderPodman {
		t.Fatalf("provider = %v, want %v", got, testcontainers.ProviderPodman)
	}
}

func TestWithContainerProviderRejectsUnsupportedProvider(t *testing.T) {
	_, err := applyOptions(WithContainerProvider(testcontainers.ProviderType(99)))
	if err == nil {
		t.Fatal("applyOptions: want error, got nil")
	}
	if !strings.Contains(err.Error(), "WithContainerProvider: unsupported testcontainers provider 99") {
		t.Fatalf("error = %q, want unsupported provider message", err)
	}
}

func TestContainerProviderEnvAppliesToEmulator(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "podman")

	opts, err := applyOptions()
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if got := customizedProvider(t, opts); got != testcontainers.ProviderPodman {
		t.Fatalf("provider = %v, want %v", got, testcontainers.ProviderPodman)
	}
}

func TestContainerProviderEnvAppliesToOmni(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "podman")

	opts, err := applyOmniOptions()
	if err != nil {
		t.Fatalf("applyOmniOptions: %v", err)
	}

	if got := customizedProvider(t, opts); got != testcontainers.ProviderPodman {
		t.Fatalf("provider = %v, want %v", got, testcontainers.ProviderPodman)
	}
}

func TestContainerProviderEnvExplicitOptionWins(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "podman")

	opts, err := applyOptions(WithContainerProvider(testcontainers.ProviderDocker))
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if got := customizedProvider(t, opts); got != testcontainers.ProviderDocker {
		t.Fatalf("provider = %v, want %v", got, testcontainers.ProviderDocker)
	}
}

func TestContainerProviderEnvDefaultMatchesUnset(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "default")

	opts, err := applyOptions()
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if len(opts.containerCustomizers) != 0 {
		t.Fatalf("containerCustomizers length = %d, want 0", len(opts.containerCustomizers))
	}
}

func TestContainerProviderEnvLowLevelCustomizerCanOverride(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "podman")

	opts, err := applyOptions(WithContainerCustomizers(testcontainers.WithProvider(testcontainers.ProviderDocker)))
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	if got := customizedProvider(t, opts); got != testcontainers.ProviderDocker {
		t.Fatalf("provider = %v, want %v", got, testcontainers.ProviderDocker)
	}
}

func TestContainerProviderEnvValidationDoesNotPreemptOptionConflicts(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "containerd")

	_, err := applyOptions(WithRandomProjectID(), WithProjectID("custom"))
	if err == nil {
		t.Fatal("applyOptions: want error, got nil")
	}
	if !strings.Contains(err.Error(), "WithRandomProjectID() and WithProjectID() are mutually exclusive") {
		t.Fatalf("error = %q, want option conflict message", err)
	}
}

func TestContainerProviderEnvRejectsUnsupportedProvider(t *testing.T) {
	t.Setenv(testcontainersProviderEnv, "containerd")

	_, err := applyOptions()
	if err == nil {
		t.Fatal("applyOptions: want error, got nil")
	}
	if !strings.Contains(err.Error(), testcontainersProviderEnv) ||
		!strings.Contains(err.Error(), `unsupported testcontainers provider "containerd"`) {
		t.Fatalf("error = %q, want env provider message", err)
	}
}

func customizedProvider(t *testing.T, opts *emulatorOptions) testcontainers.ProviderType {
	t.Helper()

	req := testcontainers.GenericContainerRequest{}
	for _, customizer := range opts.containerCustomizers {
		if err := customizer.Customize(&req); err != nil {
			t.Fatalf("customizer.Customize: %v", err)
		}
	}
	return req.ProviderType
}
