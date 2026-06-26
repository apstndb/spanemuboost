package spanemuboost

import (
	"bytes"
	"testing"
)

func TestCreateDatabaseRequestIncludesFileDescriptorSet(t *testing.T) {
	raw := []byte("proto-descriptors")
	opts, err := applyOptions(
		WithSetupDDLs([]string{"CREATE PROTO BUNDLE (/* ... */)"}),
		WithSetupRawFileDescriptorSet(raw),
	)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	req := createDatabaseRequest(opts, opts.InstancePath(), "CREATE DATABASE `test`")
	if !bytes.Equal(req.ProtoDescriptors, raw) {
		t.Fatalf("ProtoDescriptors = %q, want %q", req.ProtoDescriptors, raw)
	}
	if len(req.ExtraStatements) != 1 {
		t.Fatalf("len(ExtraStatements) = %d, want 1", len(req.ExtraStatements))
	}
}

func TestUpdateDatabaseDdlRequestIncludesFileDescriptorSet(t *testing.T) {
	raw := []byte("proto-descriptors")
	opts, err := applyOptions(
		WithSetupDDLs([]string{"ALTER PROTO BUNDLE INSERT (/* ... */)"}),
		WithSetupRawFileDescriptorSet(raw),
	)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	req := updateDatabaseDdlRequest(opts)
	if !bytes.Equal(req.ProtoDescriptors, raw) {
		t.Fatalf("ProtoDescriptors = %q, want %q", req.ProtoDescriptors, raw)
	}
	if len(req.Statements) != 1 {
		t.Fatalf("len(Statements) = %d, want 1", len(req.Statements))
	}
}