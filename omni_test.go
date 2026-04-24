package spanemuboost

import (
	"context"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
)

func TestRecommendedOmniClientConfig(t *testing.T) {
	config := RecommendedOmniClientConfig()
	if !config.DisableNativeMetrics {
		t.Fatal("DisableNativeMetrics should be true for Omni")
	}
	if !config.IsExperimentalHost {
		t.Fatal("IsExperimentalHost should be true for Omni")
	}
}

func TestRunOmniRejectsUnsupportedProjectOptions(t *testing.T) {
	t.Run("custom project", func(t *testing.T) {
		_, err := Run(context.Background(), BackendOmni, WithProjectID("custom"))
		if err == nil {
			t.Fatal("expected error for custom project ID")
		}
	})

	t.Run("random project", func(t *testing.T) {
		_, err := Run(context.Background(), BackendOmni, WithRandomProjectID())
		if err == nil {
			t.Fatal("expected error for random project ID")
		}
	})

	t.Run("custom instance", func(t *testing.T) {
		_, err := Run(context.Background(), BackendOmni, WithInstanceID("custom"))
		if err == nil {
			t.Fatal("expected error for custom instance ID")
		}
	})
}

func TestRunOmniRejectsUnsupportedInstanceAutoconfig(t *testing.T) {
	_, err := applyOmniOptions(EnableInstanceAutoConfigOnly())
	if err == nil {
		t.Fatal("expected error for instance auto-config")
	}
	if !strings.Contains(err.Error(), "EnableDatabaseAutoConfigOnly()") {
		t.Fatalf("error %q does not mention EnableDatabaseAutoConfigOnly()", err)
	}
	if !strings.Contains(err.Error(), "DisableBackendGuardrails()") {
		t.Fatalf("error %q does not mention DisableBackendGuardrails()", err)
	}
}

func TestRunOmniRejectsIncompatibleClientConfig(t *testing.T) {
	_, err := applyOmniOptions(WithClientConfig(spanner.ClientConfig{}))
	if err == nil {
		t.Fatal("expected error for incompatible client config")
	}
	if !strings.Contains(err.Error(), "IsExperimentalHost=true") {
		t.Fatalf("error %q does not mention IsExperimentalHost=true", err)
	}
}

func TestRunOmniDisableBackendGuardrails(t *testing.T) {
	opts, err := applyOmniOptions(
		DisableBackendGuardrails(),
		WithProjectID("custom-project"),
		WithInstanceID("custom-instance"),
		EnableAutoConfig(),
		WithClientConfig(spanner.ClientConfig{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if opts.projectID != "custom-project" {
		t.Fatalf("projectID = %q, want custom-project", opts.projectID)
	}
	if opts.instanceID != "custom-instance" {
		t.Fatalf("instanceID = %q, want custom-instance", opts.instanceID)
	}
	if opts.disableCreateInstance {
		t.Fatal("disableCreateInstance = true, want false")
	}
	if opts.clientConfig == nil {
		t.Fatal("clientConfig is nil")
	}
	if opts.clientConfig.IsExperimentalHost {
		t.Fatal("IsExperimentalHost = true, want false")
	}
}

func TestRunOmni(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	omni := Setup(t, BackendOmni,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('foo', 1), ('bar', 2)"}),
	)

	if omni.ProjectID() != defaultOmniProjectID {
		t.Fatalf("ProjectID() = %q, want %q", omni.ProjectID(), defaultOmniProjectID)
	}
	if omni.InstanceID() != defaultOmniInstanceID {
		t.Fatalf("InstanceID() = %q, want %q", omni.InstanceID(), defaultOmniInstanceID)
	}
	if omni.DatabaseID() == "" {
		t.Fatal("DatabaseID() is empty")
	}
	if len(omni.ClientOptions()) != 3 {
		t.Fatalf("ClientOptions() returned %d options, want 3", len(omni.ClientOptions()))
	}

	client, err := spanner.NewClientWithConfig(t.Context(), omni.DatabasePath(), RecommendedOmniClientConfig(), omni.ClientOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	iter := client.Single().Query(t.Context(), spanner.NewStatement("SELECT COUNT(*) FROM tbl"))
	err = iter.Do(func(r *spanner.Row) error {
		var count int64
		if err := r.Column(0, &count); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("COUNT(*) = %d, want 2", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunOmniWithClients(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	env := SetupWithClients(t, BackendOmni,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('foo', 1), ('bar', 2)"}),
	)

	if env.Runtime() == nil {
		t.Fatal("Runtime() returned nil")
	}

	iter := env.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'bar'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 2 {
			t.Fatalf("col = %d, want 2", col)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenOmniClients(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	omni := Setup(t, BackendOmni)

	clients := SetupClients(t, omni,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('baz', 3)"}),
	)

	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'baz'"))
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

func TestSetupClientsWithOmni(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	runtime := Setup(t, BackendOmni)
	clients := SetupClients(t, runtime,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('shared', 4)"}),
	)

	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'shared'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 4 {
			t.Fatalf("col = %d, want 4", col)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunInvalidBackend(t *testing.T) {
	_, err := Run(context.Background(), Backend("invalid"))
	if err == nil {
		t.Fatal("expected invalid backend error")
	}
}
