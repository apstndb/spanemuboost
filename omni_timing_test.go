//go:build omni_bench

package spanemuboost

import (
	"os"
	"testing"
	"time"
)

func TestOmniRuntimeAttachTiming(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1")
	}
	if !EndpointConfigured() {
		t.Skip("set SPANEMUBOOST_ENDPOINT_FILE or SPANEMUBOOST_OMNI_URI for attached timing")
	}

	start := time.Now()
	lazy, err := NewLazyRuntimeFromEnvOrStart(BackendOmni)
	if err != nil {
		t.Fatalf("NewLazyRuntimeFromEnvOrStart() error = %v", err)
	}
	clients := SetupClients(t, lazy,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}),
	)
	t.Logf("attached omni client setup: %s", time.Since(start))
	_ = clients
}

func TestOmniRuntimeColdStartTiming(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1")
	}
	if EndpointConfigured() {
		t.Skip("unset SPANEMUBOOST_ENDPOINT_FILE and SPANEMUBOOST_OMNI_URI for cold-start timing")
	}

	start := time.Now()
	lazy := NewLazyRuntime(BackendOmni)
	t.Cleanup(func() { _ = lazy.Close() })
	clients := SetupClients(t, lazy,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}),
	)
	t.Logf("cold omni container startup + client setup: %s", time.Since(start))
	_ = clients
}
