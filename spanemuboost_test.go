package spanemuboost

import (
	"sync"
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/go-cmp/cmp"
)

func TestNewEmulatorWithClients(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	ctx := t.Context()
	_, clients, teardown, err := NewEmulatorWithClients(ctx,
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
	want := []*row{
		{"bar", 2},
		{"foo", 1},
	}

	var got []*row
	err = spanner.SelectAll(clients.Client.Single().Query(ctx, stmt), &got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestNewEmulatorWithClientsPostgreSQL(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	ctx := t.Context()
	_, clients, teardown, err := NewEmulatorWithClients(ctx,
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk text PRIMARY KEY, col bigint)"}),
		WithSetupRawDMLs([]string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}),
		WithDatabaseDialect(databasepb.DatabaseDialect_POSTGRESQL),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer teardown()

	stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
	want := []*row{
		{"bar", 2},
		{"foo", 1},
	}

	var got []*row
	err = spanner.SelectAll(clients.Client.Single().Query(ctx, stmt), &got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestRunEmulatorWithClients(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	env := SetupEmulatorWithClients(t,
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}),
	)

	ctx := t.Context()
	stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
	want := []*row{
		{"bar", 2},
		{"foo", 1},
	}

	var got []*row
	err := spanner.SelectAll(env.Client.Single().Query(ctx, stmt), &got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestRunEmulatorWithClientsPostgreSQL(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	env := SetupEmulatorWithClients(t,
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk text PRIMARY KEY, col bigint)"}),
		WithSetupRawDMLs([]string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}),
		WithDatabaseDialect(databasepb.DatabaseDialect_POSTGRESQL),
	)

	ctx := t.Context()
	stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
	want := []*row{
		{"bar", 2},
		{"foo", 1},
	}

	var got []*row
	err := spanner.SelectAll(env.Client.Single().Query(ctx, stmt), &got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}

func TestSetupEmulatorAndSetupClients(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}
	dmls := []string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}

	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	t.Run("default inherits instance skip", func(t *testing.T) {
		clients := SetupClients(t, emu,
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
			WithSetupRawDMLs(dmls),
		)

		ctx := t.Context()
		stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
		want := []*row{
			{"bar", 2},
			{"foo", 1},
		}

		var got []*row
		err := spanner.SelectAll(clients.Client.Single().Query(ctx, stmt), &got)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("override with EnableAutoConfig", func(t *testing.T) {
		clients := SetupClients(t, emu,
			EnableAutoConfig(),
			WithRandomInstanceID(),
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
			WithSetupRawDMLs(dmls),
		)

		ctx := t.Context()
		stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
		want := []*row{
			{"bar", 2},
			{"foo", 1},
		}

		var got []*row
		err := spanner.SelectAll(clients.Client.Single().Query(ctx, stmt), &got)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestWithRandomIDImpliesCreation(t *testing.T) {
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}

	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	t.Run("nonexistent instance without creation fails", func(t *testing.T) {
		// OpenClients disables instance creation by default.
		// Using a non-existent instance ID without creation should fail.
		// On error OpenClients returns (nil, err), so no Close call is needed.
		_, err := OpenClients(t.Context(), emu,
			WithInstanceID("nonexistent"),
			WithSetupDDLs(ddls),
		)
		if err == nil {
			t.Fatal("expected error for nonexistent instance without creation enabled, but got nil")
		}
	})

	t.Run("nonexistent database without creation fails", func(t *testing.T) {
		// OpenClients allows database creation by default, so explicitly disable it.
		// DDLs are needed to trigger an operation against the nonexistent database;
		// without DDLs, bootstrap skips the database step entirely.
		// On error OpenClients returns (nil, err), so no Close call is needed.
		_, err := OpenClients(t.Context(), emu,
			DisableAutoConfig(),
			WithDatabaseID("nonexistent"),
			WithSetupDDLs(ddls),
		)
		if err == nil {
			t.Fatal("expected error for nonexistent database without creation enabled, but got nil")
		}
	})

	t.Run("random instance ID implies creation", func(t *testing.T) {
		clients := SetupClients(t, emu,
			WithRandomInstanceID(),
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)
		mustConsumeQuery(t, clients, "SELECT 1")
	})

	t.Run("random database ID implies creation", func(t *testing.T) {
		// DisableAutoConfig first so that WithRandomDatabaseID() must
		// re-enable database creation to succeed.
		clients := SetupClients(t, emu,
			DisableAutoConfig(),
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)
		mustConsumeQuery(t, clients, "SELECT 1")
	})

	t.Run("DisableAutoConfig after random ID overrides", func(t *testing.T) {
		// On error OpenClients returns (nil, err), so no Close call is needed.
		_, err := OpenClients(t.Context(), emu,
			WithRandomInstanceID(),
			WithRandomDatabaseID(),
			DisableAutoConfig(),
			WithSetupDDLs(ddls),
		)
		if err == nil {
			t.Fatal("expected error when DisableAutoConfig follows WithRandomInstanceID, but got nil")
		}
	})
}

func TestEmulatorInheritedOptionsReuseExistingDatabase(t *testing.T) {
	opts, err := applyOptions(WithDatabaseID("existing-database"))
	if err != nil {
		t.Fatal(err)
	}

	emu := &Emulator{opts: opts}
	inherited, err := emu.inheritedOptions()
	if err != nil {
		t.Fatal(err)
	}

	if !inherited.disableCreateInstance {
		t.Fatal("disableCreateInstance = false, want true")
	}
	if !inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = false, want true")
	}
	if inherited.databaseID != "existing-database" {
		t.Fatalf("databaseID = %q, want existing-database", inherited.databaseID)
	}
}

func TestEmulatorInheritedOptionsKeepReuseWhenDatabaseIsUnchanged(t *testing.T) {
	opts, err := applyOptions(WithDatabaseID("existing-database"))
	if err != nil {
		t.Fatal(err)
	}

	emu := &Emulator{opts: opts}
	inherited, err := emu.inheritedOptions(WithDatabaseID("existing-database"))
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseID != "existing-database" {
		t.Fatalf("databaseID = %q, want existing-database", inherited.databaseID)
	}
	if !inherited.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = false, want true")
	}
}

func TestEmulatorInheritedOptionsPreserveDatabaseDialect(t *testing.T) {
	opts, err := applyOptions(WithDatabaseDialect(databasepb.DatabaseDialect_POSTGRESQL))
	if err != nil {
		t.Fatal(err)
	}

	emu := &Emulator{opts: opts}
	inherited, err := emu.inheritedOptions(WithRandomDatabaseID())
	if err != nil {
		t.Fatal(err)
	}

	if inherited.databaseDialect != databasepb.DatabaseDialect_POSTGRESQL {
		t.Fatalf("databaseDialect = %v, want %v", inherited.databaseDialect, databasepb.DatabaseDialect_POSTGRESQL)
	}
}

func TestRuntimeEnvCloseZeroValue(t *testing.T) {
	var env RuntimeEnv
	if err := env.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	var nilEnv *RuntimeEnv
	if err := nilEnv.Close(); err != nil {
		t.Fatalf("nil Close() error = %v, want nil", err)
	}
}

func TestMinimalBootstrapClientConfig(t *testing.T) {
	original := spanner.ClientConfig{
		DisableNativeMetrics: true,
		SessionPoolConfig: spanner.SessionPoolConfig{
			MinOpened: 5,
			MaxOpened: 7,
		},
	}

	got := minimalBootstrapClientConfig(original)

	if got.MinOpened != 1 {
		t.Fatalf("MinOpened = %d, want 1", got.MinOpened)
	}
	if got.MaxOpened != 1 {
		t.Fatalf("MaxOpened = %d, want 1", got.MaxOpened)
	}
	if !got.DisableNativeMetrics {
		t.Fatal("DisableNativeMetrics = false, want true")
	}
	if original.MinOpened != 5 {
		t.Fatalf("original MinOpened = %d, want 5", original.MinOpened)
	}
	if original.MaxOpened != 7 {
		t.Fatalf("original MaxOpened = %d, want 7", original.MaxOpened)
	}
}

func TestOpenClientsRejectsNilRuntime(t *testing.T) {
	var nilEmulator *Emulator
	_, err := OpenClients(t.Context(), nilEmulator)
	if err == nil {
		t.Fatal("OpenClients(nil *Emulator) error = nil, want non-nil")
	}

	var nilLazy *LazyEmulator
	_, err = OpenClients(t.Context(), nilLazy)
	if err == nil {
		t.Fatal("OpenClients(nil *LazyEmulator) error = nil, want non-nil")
	}

	var nilRuntime Runtime = (*Emulator)(nil)
	_, err = OpenClients(t.Context(), nilRuntime)
	if err == nil {
		t.Fatal("OpenClients(nil Runtime) error = nil, want non-nil")
	}
}

func TestSchemaTeardown(t *testing.T) {
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}

	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	t.Run("fixed ID is dropped by default", func(t *testing.T) {
		// Create database in a subtest so it is closed (and torn down) on exit.
		t.Run("create", func(t *testing.T) {
			clients := SetupClients(t, emu,
				WithDatabaseID("fixed-teardown"),
				WithSetupDDLs(ddls),
			)
			mustConsumeQuery(t, clients, "SELECT 1")
		})
		// If teardown worked, re-creating with the same ID should succeed.
		t.Run("recreate", func(t *testing.T) {
			clients := SetupClients(t, emu,
				WithDatabaseID("fixed-teardown"),
				WithSetupDDLs(ddls),
			)
			mustConsumeQuery(t, clients, "SELECT 1")
		})
	})

	t.Run("SkipSchemaTeardown keeps database", func(t *testing.T) {
		// Create database with teardown skipped in a subtest.
		t.Run("create", func(t *testing.T) {
			clients := SetupClients(t, emu,
				WithDatabaseID("skip-teardown"),
				SkipSchemaTeardown(),
				WithSetupDDLs(ddls),
			)
			mustConsumeQuery(t, clients, "SELECT 1")
		})
		// Database still exists, so re-creating should fail.
		// On error OpenClients returns (nil, err), so no Close call is needed.
		_, err := OpenClients(t.Context(), emu,
			WithDatabaseID("skip-teardown"),
			WithSetupDDLs(ddls),
		)
		if err == nil {
			t.Fatal("expected 'already exists' error, but got nil")
		}
	})

	t.Run("random ID is not dropped by default", func(t *testing.T) {
		// Random IDs are not dropped by default. Verify by creating a
		// database with a random ID, closing the clients, and then
		// successfully reconnecting to it.
		var dbID string
		t.Run("create", func(t *testing.T) {
			clients := SetupClients(t, emu,
				WithRandomDatabaseID(),
				WithSetupDDLs(ddls),
			)
			dbID = clients.DatabaseID
			mustConsumeQuery(t, clients, "SELECT 1")
		})
		// After "create" subtest, clients are closed. The database should
		// still exist. Reconnect with auto-creation disabled to confirm.
		reconnectClients, err := OpenClients(t.Context(), emu,
			WithDatabaseID(dbID),
			DisableAutoConfig(),
		)
		if err != nil {
			t.Fatalf("expected to reconnect to existing random-ID database, but got error: %v", err)
		}
		if err := reconnectClients.Close(); err != nil {
			t.Errorf("failed to close reconnect clients: %v", err)
		}
	})
}

func TestEmulatorAccessors(t *testing.T) {
	emu := SetupEmulator(t, DisableAutoConfig())

	if got := emu.ProjectID(); got != DefaultProjectID {
		t.Errorf("ProjectID() = %q, want %q", got, DefaultProjectID)
	}
	if got := emu.InstanceID(); got != DefaultInstanceID {
		t.Errorf("InstanceID() = %q, want %q", got, DefaultInstanceID)
	}
	if got := emu.DatabaseID(); got != DefaultDatabaseID {
		t.Errorf("DatabaseID() = %q, want %q", got, DefaultDatabaseID)
	}
	if got := emu.URI(); got == "" {
		t.Error("URI() is empty")
	}
	if got := emu.Container(); got == nil {
		t.Error("Container() is nil")
	}
	if got := emu.ProjectPath(); got != "projects/"+DefaultProjectID {
		t.Errorf("ProjectPath() = %q", got)
	}
	if got := emu.InstancePath(); got != "projects/"+DefaultProjectID+"/instances/"+DefaultInstanceID {
		t.Errorf("InstancePath() = %q", got)
	}
	if got := emu.DatabasePath(); got != "projects/"+DefaultProjectID+"/instances/"+DefaultInstanceID+"/databases/"+DefaultDatabaseID {
		t.Errorf("DatabasePath() = %q", got)
	}
	if opts := emu.ClientOptions(); len(opts) != 4 {
		t.Errorf("ClientOptions() returned %d options, want 4", len(opts))
	}
}

func TestClientsAccessors(t *testing.T) {
	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	clients := SetupClients(t, emu,
		WithRandomDatabaseID(),
	)

	if opts := clients.ClientOptions(); len(opts) != 4 {
		t.Errorf("ClientOptions() returned %d options, want 4", len(opts))
	}
	if uri := clients.URI(); uri == "" {
		t.Error("URI() is empty")
	} else if uri != emu.URI() {
		t.Errorf("URI() = %q, want %q (from emulator)", uri, emu.URI())
	}
}

func TestLazyEmulatorWithSetupClients(t *testing.T) {
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}

	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy emulator: %v", err)
		}
	}()

	t.Run("first call starts emulator", func(t *testing.T) {
		clients := SetupClients(t, lazy,
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)
		mustConsumeQuery(t, clients, "SELECT 1")
	})

	t.Run("second call reuses emulator", func(t *testing.T) {
		clients := SetupClients(t, lazy,
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)
		mustConsumeQuery(t, clients, "SELECT 1")
	})
}

func TestLazyEmulatorWithOpenClients(t *testing.T) {
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}

	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy emulator: %v", err)
		}
	}()

	clients, err := OpenClients(t.Context(), lazy,
		WithRandomDatabaseID(),
		WithSetupDDLs(ddls),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := clients.Close(); err != nil {
			t.Errorf("failed to close clients: %v", err)
		}
	}()

	mustConsumeQuery(t, clients, "SELECT 1")
}

func TestLazyEmulatorClientsAccessors(t *testing.T) {
	// Verify that Clients.ClientOptions() and Clients.URI() work when
	// created via SetupClients with a LazyEmulator, so callers don't need
	// a separate *Emulator reference for connection info.
	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy emulator: %v", err)
		}
	}()

	clients := SetupClients(t, lazy,
		WithRandomDatabaseID(),
	)

	if opts := clients.ClientOptions(); len(opts) != 4 {
		t.Errorf("ClientOptions() returned %d options, want 4", len(opts))
	}
	if uri := clients.URI(); uri == "" {
		t.Error("URI() is empty")
	}
}

func TestLazyEmulatorConcurrentGet(t *testing.T) {
	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy emulator: %v", err)
		}
	}()

	const goroutines = 10
	emus := make([]*Emulator, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			emus[i], errs[i] = lazy.Get(t.Context())
		}()
	}
	wg.Wait()

	for i := range goroutines {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: Get() failed: %v", i, errs[i])
		}
		if emus[i] != emus[0] {
			t.Errorf("goroutine %d: Get() returned different instance", i)
		}
	}
}

func TestLazyEmulatorGetReturnsSameInstance(t *testing.T) {
	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Errorf("failed to close lazy emulator: %v", err)
		}
	}()

	ctx := t.Context()
	emu1, err := lazy.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	emu2, err := lazy.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if emu1 != emu2 {
		t.Error("Get() returned different instances")
	}
}

func TestLazyEmulatorCloseWithoutStart(t *testing.T) {
	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	if err := lazy.Close(); err != nil {
		t.Fatalf("Close() on unused LazyEmulator should be no-op, got: %v", err)
	}
}

func TestLazyEmulatorGetAfterClose(t *testing.T) {
	lazy := NewLazyEmulator(EnableInstanceAutoConfigOnly())
	if err := lazy.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := lazy.Get(t.Context())
	if err == nil {
		t.Fatal("Get() after Close() should return an error")
	}
}

func sliceOf[T any](values ...T) []T {
	return values
}

// mustConsumeQuery executes a query and fails the test if it returns an error.
func mustConsumeQuery(t *testing.T, clients *Clients, sql string) {
	t.Helper()
	iter := clients.Client.Single().Query(t.Context(), spanner.NewStatement(sql))
	if err := iter.Do(func(*spanner.Row) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestNewEmulatorAndNewClientsWithDisableAutoConfig(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	// Use the same DDLs and DMLs for all tests.
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)"}
	dmls := []string{`INSERT INTO tbl (pk, col) VALUES ('foo', 1),('bar', 2)`}

	tests := []struct {
		desc            string
		newEmulatorOpts []Option
		newClientsOpts  []Option
	}{
		{
			"all config on NewEmulator",
			sliceOf(
				WithSetupDDLs(ddls),
				WithSetupRawDMLs(dmls),
			),
			sliceOf(
				DisableAutoConfig(),
			),
		},
		{
			"all config on NewClients",
			sliceOf(
				DisableAutoConfig(),
			),
			sliceOf(
				WithSetupDDLs(ddls),
				WithSetupRawDMLs(dmls),
			),
		},
		{
			"config instance on NewEmulator and config database on NewClients",
			sliceOf(
				EnableInstanceAutoConfigOnly(),
			),
			sliceOf(
				EnableDatabaseAutoConfigOnly(),
				WithSetupDDLs(ddls),
				WithSetupRawDMLs(dmls),
			),
		},
		{
			"config on NewEmulator and setup database on NewClients",
			nil,
			sliceOf(
				DisableAutoConfig(),
				WithSetupDDLs(ddls),
				WithSetupRawDMLs(dmls),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctx := t.Context()
			emulator, emuTeardown, err := NewEmulator(ctx, test.newEmulatorOpts...)
			if err != nil {
				t.Fatal(err)
			}
			defer emuTeardown()

			clients, clientTeardown, err := NewClients(ctx, emulator, test.newClientsOpts...)
			if err != nil {
				t.Fatal(err)
			}
			defer clientTeardown()

			stmt := spanner.NewStatement(`SELECT pk, col FROM tbl ORDER BY pk`)
			want := []*row{
				{"bar", 2},
				{"foo", 1},
			}

			var got []*row
			err = spanner.SelectAll(clients.Client.Single().Query(ctx, stmt), &got)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
