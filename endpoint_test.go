package spanemuboost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEndpointFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoint.json")
	endpoint := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}
	if err := SaveEndpoint(path, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}

	t.Setenv(endpointFileEnv, path)
	t.Setenv(omniURIEnv, "")

	got, err := LoadEndpoint()
	if err != nil {
		t.Fatalf("LoadEndpoint() error = %v", err)
	}
	if got != endpoint {
		t.Fatalf("LoadEndpoint() = %#v, want %#v", got, endpoint)
	}
}

func TestLoadEndpointFromOmniEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")
	t.Setenv(omniProjectIDEnv, "proj")
	t.Setenv(omniInstanceIDEnv, "inst")

	got, err := LoadEndpoint()
	if err != nil {
		t.Fatalf("LoadEndpoint() error = %v", err)
	}
	want := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  "proj",
		InstanceID: "inst",
	}
	if got != want {
		t.Fatalf("LoadEndpoint() = %#v, want %#v", got, want)
	}
}

func TestEndpointConfigured(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "")
	if EndpointConfigured() {
		t.Fatal("EndpointConfigured() = true, want false")
	}
	t.Setenv(omniURIEnv, "127.0.0.1:15000")
	if !EndpointConfigured() {
		t.Fatal("EndpointConfigured() = false, want true")
	}
}

func TestSaveEndpointRejectsInvalidBackend(t *testing.T) {
	err := SaveEndpoint(filepath.Join(t.TempDir(), "endpoint.json"), Endpoint{
		Backend:    Backend("bad"),
		URI:        "127.0.0.1:1",
		ProjectID:  "p",
		InstanceID: "i",
	})
	if err == nil {
		t.Fatal("SaveEndpoint() error = nil, want non-nil")
	}
}

func TestParseServeArgs(t *testing.T) {
	cfg, err := ParseServeArgs([]string{"omni", "--endpoint-file", "/tmp/omni.json"})
	if err != nil {
		t.Fatalf("ParseServeArgs() error = %v", err)
	}
	if cfg.Backend != BackendOmni || cfg.EndpointFile != "/tmp/omni.json" {
		t.Fatalf("ParseServeArgs() = %#v, want omni + /tmp/omni.json", cfg)
	}
}

func TestNewAttachedRuntimeCloseIsNoOp(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	})
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

func TestNewLazyRuntimeOptionalEndpointUsesEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")

	lazy := NewLazyRuntimeOptionalEndpoint(BackendOmni)
	if lazy.attachedEndpoint == nil {
		t.Fatal("attachedEndpoint = nil, want non-nil")
	}
	if lazy.attachedEndpoint.URI != "127.0.0.1:15000" {
		t.Fatalf("attachedEndpoint.URI = %q, want 127.0.0.1:15000", lazy.attachedEndpoint.URI)
	}
}

func TestReadEndpointFileMissing(t *testing.T) {
	_, err := ReadEndpointFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("ReadEndpointFile() error = nil, want non-nil")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "read endpoint file") {
		t.Fatalf("ReadEndpointFile() error = %v", err)
	}
}
