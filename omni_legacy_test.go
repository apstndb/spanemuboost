package spanemuboost

import (
	"os"
	"testing"

	"cloud.google.com/go/spanner"
)

func TestRunOmniLegacyImages(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}
	// Keep these serial: every runtime owns a memory-heavy container.
	for _, version := range []string{"2026.r2.1-beta", "2026.r3-beta"} {
		t.Run(version, func(t *testing.T) {
			image := "us-docker.pkg.dev/spanner-omni/images/spanner-omni:" + version
			env, err := RunWithClients(t.Context(), BackendOmni,
				WithContainerImage(image), WithOmniStartArgs(),
				WithRandomDatabaseID(),
				WithSetupDDLs([]string{"CREATE TABLE tbl (pk INT64, col STRING(MAX)) PRIMARY KEY (pk)"}),
				WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES (1, 'legacy')"}),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := env.Close(); err != nil {
					t.Error(err)
				}
			})
			assertObservedRuntimeProvenance(t, env.Runtime(), image)
			rows := 0
			err = env.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 1")).Do(func(row *spanner.Row) error {
				rows++
				var value string
				if err := row.Column(0, &value); err != nil {
					return err
				}
				if value != "legacy" {
					t.Fatalf("col = %q, want legacy", value)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if rows != 1 {
				t.Fatalf("query returned %d rows, want 1", rows)
			}
			clients, err := OpenClients(t.Context(), env.Runtime())
			if err != nil {
				t.Fatal(err)
			}
			mustConsumeQuery(t, clients, "SELECT COUNT(*) FROM tbl")
			if err := clients.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
