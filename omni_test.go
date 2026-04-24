package spanemuboost

import (
	"context"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
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

func TestOmniRuntimeCloseZeroValue(t *testing.T) {
	var nilRuntime *omniRuntime
	if err := nilRuntime.Close(); err != nil {
		t.Fatalf("nil Close() error = %v, want nil", err)
	}

	runtime := &omniRuntime{}
	if err := runtime.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
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

func TestRunOmniAppliesRecommendedClientConfigForManagedClients(t *testing.T) {
	opts, err := applyOmniOptions(WithClientConfig(spanner.ClientConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	if opts.clientConfig == nil {
		t.Fatal("clientConfig is nil")
	}
	if !opts.clientConfig.DisableNativeMetrics {
		t.Fatal("DisableNativeMetrics = false, want true")
	}
	if !opts.clientConfig.IsExperimentalHost {
		t.Fatal("IsExperimentalHost = false, want true")
	}
}

func TestRunOmniDisableBackendGuardrails(t *testing.T) {
	opts, err := applyOmniOptions(
		DisableBackendGuardrails(),
		WithRandomProjectID(),
		WithRandomInstanceID(),
		EnableAutoConfig(),
		WithClientConfig(spanner.ClientConfig{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if opts.projectID == "" || opts.projectID == defaultOmniProjectID {
		t.Fatalf("projectID = %q, want non-default random ID", opts.projectID)
	}
	if opts.instanceID == "" || opts.instanceID == defaultOmniInstanceID {
		t.Fatalf("instanceID = %q, want non-default random ID", opts.instanceID)
	}
	if opts.disableCreateInstance {
		t.Fatal("disableCreateInstance = true, want false")
	}
	if opts.clientConfig == nil {
		t.Fatal("clientConfig is nil")
	}
	if opts.clientConfig.DisableNativeMetrics {
		t.Fatal("DisableNativeMetrics = true, want false")
	}
	if opts.clientConfig.IsExperimentalHost {
		t.Fatal("IsExperimentalHost = true, want false")
	}
}

func TestOmniInheritedOptionsReuseExistingDatabase(t *testing.T) {
	opts, err := applyOmniOptions()
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions()
	if err != nil {
		t.Fatal(err)
	}

	if inherited.projectID != opts.projectID {
		t.Fatalf("projectID = %q, want %q", inherited.projectID, opts.projectID)
	}
	if inherited.instanceID != opts.instanceID {
		t.Fatalf("instanceID = %q, want %q", inherited.instanceID, opts.instanceID)
	}
	if inherited.databaseID != opts.databaseID {
		t.Fatalf("databaseID = %q, want %q", inherited.databaseID, opts.databaseID)
	}
	if !inherited.disableCreateInstance {
		t.Fatal("disableCreateInstance = false, want true")
	}
	if !inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = false, want true")
	}
}

func TestOmniInheritedOptionsAllowDatabaseOverride(t *testing.T) {
	opts, err := applyOmniOptions()
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions(WithDatabaseID("override-database"))
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseID != "override-database" {
		t.Fatalf("databaseID = %q, want override-database", inherited.databaseID)
	}
	if inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = true, want false")
	}
}

func TestOmniInheritedOptionsKeepReuseWhenDatabaseIsUnchanged(t *testing.T) {
	opts, err := applyOmniOptions()
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions(WithDatabaseID(opts.databaseID))
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseID != opts.databaseID {
		t.Fatalf("databaseID = %q, want %q", inherited.databaseID, opts.databaseID)
	}
	if !inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = false, want true")
	}
}

func TestOmniInheritedOptionsPreserveDatabaseDialect(t *testing.T) {
	opts, err := applyOmniOptions(WithDatabaseDialect(databasepb.DatabaseDialect_POSTGRESQL))
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions(WithRandomDatabaseID())
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseDialect != databasepb.DatabaseDialect_POSTGRESQL {
		t.Fatalf("databaseDialect = %v, want %v", inherited.databaseDialect, databasepb.DatabaseDialect_POSTGRESQL)
	}
}

func TestOmniInheritedOptionsRespectDisableAutoConfigForDatabaseOverride(t *testing.T) {
	opts, err := applyOmniOptions()
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions(
		DisableAutoConfig(),
		WithDatabaseID("override-database"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseID != "override-database" {
		t.Fatalf("databaseID = %q, want override-database", inherited.databaseID)
	}
	if !inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = false, want true")
	}
}

func TestOmniInheritedOptionsPreserveDisabledGuardrails(t *testing.T) {
	opts, err := applyOmniOptions(
		DisableBackendGuardrails(),
		WithProjectID("custom-project"),
		WithInstanceID("custom-instance"),
		WithDatabaseID("custom-database"),
		WithClientConfig(spanner.ClientConfig{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	omni := &omniRuntime{opts: opts}
	inherited, err := omni.inheritedOptions()
	if err != nil {
		t.Fatal(err)
	}

	if !inherited.disableBackendGuardrails {
		t.Fatal("disableBackendGuardrails = false, want true")
	}
	if inherited.projectID != "custom-project" {
		t.Fatalf("projectID = %q, want custom-project", inherited.projectID)
	}
	if inherited.instanceID != "custom-instance" {
		t.Fatalf("instanceID = %q, want custom-instance", inherited.instanceID)
	}
	if inherited.databaseID != "custom-database" {
		t.Fatalf("databaseID = %q, want custom-database", inherited.databaseID)
	}
	if inherited.clientConfig == nil {
		t.Fatal("clientConfig is nil")
	}
	if inherited.clientConfig.IsExperimentalHost {
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
	if len(omni.ClientOptions()) < 3 {
		t.Fatalf("ClientOptions() returned %d options, want at least 3", len(omni.ClientOptions()))
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

func TestOpenOmniClientsReuseDefaultDatabase(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	omni := Setup(t, BackendOmni,
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('default', 5)"}),
	)

	clients := SetupClients(t, omni)
	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'default'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 5 {
			t.Fatalf("col = %d, want 5", col)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenOmniClientsAllowDatabaseOverride(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	omni := Setup(t, BackendOmni)
	clients := SetupClients(t, omni,
		WithDatabaseID("override-database"),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('override', 6)"}),
	)

	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'override'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 6 {
			t.Fatalf("col = %d, want 6", col)
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

func TestLazyRuntimeWithOmni(t *testing.T) {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") == "" {
		t.Skip("set SPANEMUBOOST_ENABLE_OMNI_TESTS=1 to run Spanner Omni tests")
	}

	lazy := NewLazyRuntime(BackendOmni)
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy runtime: %v", err)
		}
	}()

	clients := SetupClients(t, lazy,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk, col) VALUES ('lazy', 7)"}),
	)

	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement("SELECT col FROM tbl WHERE pk = 'lazy'"))
	err := iter.Do(func(r *spanner.Row) error {
		var col int64
		if err := r.Column(0, &col); err != nil {
			return err
		}
		if col != 7 {
			t.Fatalf("col = %d, want 7", col)
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
