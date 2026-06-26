package spanemuboost

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestWithSetupFileDescriptorSet(t *testing.T) {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{Name: proto.String("example.proto")},
		},
	}
	want, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	opts, err := applyOptions(WithSetupFileDescriptorSet(fds))
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, want) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, want)
	}
}

func TestWithSetupRawFileDescriptorSet(t *testing.T) {
	raw := []byte("serialized-file-descriptor-set")
	opts, err := applyOptions(WithSetupRawFileDescriptorSet(raw))
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, raw) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, raw)
	}

	raw[0] = 'X'
	if bytes.Equal(opts.setupFileDescriptorSet, raw) {
		t.Fatal("setupFileDescriptorSet aliases caller slice")
	}
}

func TestSetupFileDescriptorSetOptionsLastWins(t *testing.T) {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("typed.proto")}},
	}
	raw := []byte("raw-descriptor-set")

	opts, err := applyOptions(
		WithSetupFileDescriptorSet(fds),
		WithSetupRawFileDescriptorSet(raw),
	)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, raw) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, raw)
	}

	marshaled, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	opts, err = applyOptions(
		WithSetupRawFileDescriptorSet(raw),
		WithSetupFileDescriptorSet(fds),
	)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, marshaled) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, marshaled)
	}
}

func TestWithSetupFileDescriptorSetSnapshotsBeforeApply(t *testing.T) {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("before.proto")}},
	}
	opt := WithSetupFileDescriptorSet(fds)
	fds.File[0].Name = proto.String("after.proto")

	opts, err := applyOptions(opt)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}

	want, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{Name: proto.String("before.proto")}},
	})
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, want) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, want)
	}
}

func TestWithSetupRawFileDescriptorSetSnapshotsBeforeApply(t *testing.T) {
	raw := []byte("before")
	opt := WithSetupRawFileDescriptorSet(raw)
	raw[0] = 'X'

	opts, err := applyOptions(opt)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	if !bytes.Equal(opts.setupFileDescriptorSet, []byte("before")) {
		t.Fatalf("setupFileDescriptorSet = %q, want %q", opts.setupFileDescriptorSet, []byte("before"))
	}
}

func TestHasSetupDDLWork(t *testing.T) {
	t.Run("ddl only", func(t *testing.T) {
		opts, err := applyOptions(WithSetupDDLs([]string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"}))
		if err != nil {
			t.Fatalf("applyOptions: %v", err)
		}
		if !opts.hasSetupDDLWork() {
			t.Fatal("hasSetupDDLWork() = false, want true")
		}
	})
	t.Run("descriptor only", func(t *testing.T) {
		opts, err := applyOptions(WithSetupRawFileDescriptorSet([]byte("fds")))
		if err != nil {
			t.Fatalf("applyOptions: %v", err)
		}
		if opts.hasSetupDDLWork() {
			t.Fatal("hasSetupDDLWork() = true, want false for descriptor-only options")
		}
	})
	t.Run("descriptor without ddl when database auto-create disabled", func(t *testing.T) {
		_, err := applyOptions(
			DisableAutoConfig(),
			WithSetupRawFileDescriptorSet([]byte("fds")),
		)
		if err == nil {
			t.Fatal("applyOptions: want error for descriptor-only options with DisableAutoConfig")
		}
	})
	t.Run("empty", func(t *testing.T) {
		opts, err := applyOptions()
		if err != nil {
			t.Fatalf("applyOptions: %v", err)
		}
		if opts.hasSetupDDLWork() {
			t.Fatal("hasSetupDDLWork() = true, want false")
		}
	})
}
