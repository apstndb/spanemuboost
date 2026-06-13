package spanemuboost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
