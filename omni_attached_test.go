package spanemuboost

import (
	"os"
	"testing"

	"cloud.google.com/go/spanner"
)

func TestAttachedOmniClientsFromEnv(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}
	if !EndpointConfigured() {
		t.Skip("set SPANEMUBOOST_ENDPOINT_FILE or SPANEMUBOOST_OMNI_URI to run attached Omni tests")
	}

	lazy := NewLazyRuntimeOptionalEndpoint(BackendOmni)
	clients := SetupClients(t, lazy,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('attached', 3)"}),
	)

	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'attached'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 3 {
			t.Fatalf("col = %d, want 3", col)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
