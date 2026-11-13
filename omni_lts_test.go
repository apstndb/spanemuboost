package spanemuboost

import (
	"os"
	"testing"

	"cloud.google.com/go/spanner"
)

func TestRunOmniLatestLTS(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}
	env, err := RunWithClients(t.Context(), BackendOmni,
		WithContainerImage(LatestLTSOmniImage), WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE lts_smoke (pk INT64 NOT NULL, value STRING(MAX)) PRIMARY KEY (pk)"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Close(); err != nil {
			t.Error(err)
		}
	})
	assertObservedRuntimeProvenance(t, env.Runtime(), LatestLTSOmniImage)
	if _, err := env.Client.Apply(t.Context(), []*spanner.Mutation{
		spanner.Insert("lts_smoke", []string{"pk", "value"}, []any{int64(1), "lts"}),
	}); err != nil {
		t.Fatal(err)
	}
	clients, err := OpenClients(t.Context(), env.Runtime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := clients.Close(); err != nil {
			t.Error(err)
		}
	}()
	row, err := clients.Client.Single().ReadRow(t.Context(), "lts_smoke", spanner.Key{int64(1)}, []string{"value"})
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := row.Columns(&value); err != nil {
		t.Fatal(err)
	}
	if value != "lts" {
		t.Fatalf("value = %q, want lts", value)
	}
	if err := clients.Close(); err != nil {
		t.Fatal(err)
	}
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
}
