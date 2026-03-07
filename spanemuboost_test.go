package spanemuboost

import (
	"context"
	"fmt"
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

	ctx := context.Background()
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

	ctx := context.Background()
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

	ctx := context.Background()
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

	ctx := context.Background()
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

		ctx := context.Background()
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

		ctx := context.Background()
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

	// SetupEmulator with EnableInstanceAutoConfigOnly sets disableCreateInstance=false
	// but the base for SetupClients sets disableCreateInstance=true.
	// WithRandomInstanceID should implicitly re-enable instance creation,
	// so this must succeed without an explicit EnableAutoConfig call.
	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	t.Run("random instance ID implies creation", func(t *testing.T) {
		clients := SetupClients(t, emu,
			WithRandomInstanceID(),
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)

		ctx := context.Background()
		iter := clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1"))
		defer iter.Stop()
		_, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("random database ID implies creation", func(t *testing.T) {
		clients := SetupClients(t, emu,
			WithRandomDatabaseID(),
			WithSetupDDLs(ddls),
		)

		ctx := context.Background()
		iter := clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1"))
		defer iter.Stop()
		_, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("DisableAutoConfig after random ID overrides", func(t *testing.T) {
		// Use OpenClients instead of SetupClients because we need to inspect
		// the returned error: SetupClients would call t.Fatal internally.
		// WithRandomInstanceID enables creation, but DisableAutoConfig afterwards
		// should re-disable it, verifying sequential override behavior.
		// On error OpenClients returns (nil, err), so no Close call is needed.
		_, err := OpenClients(context.Background(), emu,
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

func TestWithStrictTeardown(t *testing.T) {
	ddls := []string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}

	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())

	// Run two sequential subtests with the same database ID and AutoConfig.
	// Without WithStrictTeardown, the second subtest would fail with "already exists".
	for i := range 2 {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			clients := SetupClients(t, emu,
				WithDatabaseID("strict-teardown-test"),
				WithStrictTeardown(),
				WithSetupDDLs(ddls),
			)

			ctx := context.Background()
			iter := clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1"))
			defer iter.Stop()
			_, err := iter.Next()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
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

func sliceOf[T any](values ...T) []T {
	return values
}

func TestNewEmulatorAndNewClientsWithDisableAutoConfig(t *testing.T) {
	type row struct {
		PK  string `spanner:"pk"`
		Col int64  `spanner:"col"`
	}

	ctx := context.Background()

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
