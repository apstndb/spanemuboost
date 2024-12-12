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
