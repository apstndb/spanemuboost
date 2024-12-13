package spanemuboost

import (
	"context"
	"testing"

	"cloud.google.com/go/spanner"
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
