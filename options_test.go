package spanemuboost

import (
	"bytes"
	"strings"
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

func TestValidateResourceIDsRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		want    string
	}{
		{
			name:    "project slash",
			options: []Option{WithProjectID("bad/project")},
			want:    "project ID",
		},
		{
			name:    "project whitespace",
			options: []Option{WithProjectID(" \t ")},
			want:    "project ID",
		},
		{
			name:    "project too short",
			options: []Option{WithProjectID("abcde")},
			want:    "too short",
		},
		{
			name:    "project overlong",
			options: []Option{WithProjectID(strings.Repeat("a", maxProjectIDLength+1))},
			want:    "too long",
		},
		{
			name:    "instance too short",
			options: []Option{WithInstanceID("a")},
			want:    "too short",
		},
		{
			name:    "instance backtick",
			options: []Option{WithInstanceID("bad`instance")},
			want:    "instance ID",
		},
		{
			name:    "instance overlong",
			options: []Option{WithInstanceID(strings.Repeat("a", maxInstanceIDLength+1))},
			want:    "too long",
		},
		{
			name:    "database quote",
			options: []Option{WithDatabaseID(`bad"database`)},
			want:    "database ID",
		},
		{
			name:    "database backtick",
			options: []Option{WithDatabaseID("bad`database")},
			want:    "database ID",
		},
		{
			name:    "database uppercase",
			options: []Option{WithDatabaseID("Database")},
			want:    "database ID",
		},
		{
			name:    "database too short",
			options: []Option{WithDatabaseID("a")},
			want:    "too short",
		},
		{
			name:    "database overlong",
			options: []Option{WithDatabaseID(strings.Repeat("a", maxDatabaseIDLength+1))},
			want:    "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyOptions(tt.options...)
			if err == nil {
				t.Fatal("applyOptions() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("applyOptions() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateResourceIDsRejectsInvalidOmniOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		want    string
	}{
		{
			name:    "database slash",
			options: []Option{WithDatabaseID("bad/database")},
			want:    "database ID",
		},
		{
			name: "project slash with guardrails disabled",
			options: []Option{
				DisableBackendGuardrails(),
				WithProjectID("bad/project"),
			},
			want: "project ID",
		},
		{
			name: "instance backtick with guardrails disabled",
			options: []Option{
				DisableBackendGuardrails(),
				WithInstanceID("bad`instance"),
			},
			want: "instance ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyOmniOptions(tt.options...)
			if err == nil {
				t.Fatal("applyOmniOptions() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("applyOmniOptions() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateResourceIDRejectsEmptyFinalID(t *testing.T) {
	err := validateResourceID("database", "", minDatabaseIDLength, maxDatabaseIDLength)
	if err == nil {
		t.Fatal("validateResourceID() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "empty after option finalization") {
		t.Fatalf("validateResourceID() error = %q, want empty-finalization message", err)
	}
}

func TestValidateResourceIDsAcceptsDefaultsAndRandomIDs(t *testing.T) {
	t.Run("emulator defaults", func(t *testing.T) {
		opts, err := applyOptions()
		if err != nil {
			t.Fatalf("applyOptions() error = %v, want nil", err)
		}
		if opts.projectID != DefaultProjectID {
			t.Fatalf("projectID = %q, want %q", opts.projectID, DefaultProjectID)
		}
		if opts.instanceID != DefaultInstanceID {
			t.Fatalf("instanceID = %q, want %q", opts.instanceID, DefaultInstanceID)
		}
		if opts.databaseID != DefaultDatabaseID {
			t.Fatalf("databaseID = %q, want %q", opts.databaseID, DefaultDatabaseID)
		}
	})

	t.Run("emulator random IDs", func(t *testing.T) {
		opts, err := applyOptions(
			WithRandomProjectID(),
			WithRandomInstanceID(),
			WithRandomDatabaseID(),
		)
		if err != nil {
			t.Fatalf("applyOptions() error = %v, want nil", err)
		}
		for name, id := range map[string]string{
			"project":  opts.projectID,
			"instance": opts.instanceID,
			"database": opts.databaseID,
		} {
			if len(id) != idRange {
				t.Fatalf("%s ID length = %d, want %d", name, len(id), idRange)
			}
		}
	})

	t.Run("emulator database underscores", func(t *testing.T) {
		opts, err := applyOptions(WithDatabaseID("my_database"))
		if err != nil {
			t.Fatalf("applyOptions() error = %v, want nil", err)
		}
		if opts.databaseID != "my_database" {
			t.Fatalf("databaseID = %q, want %q", opts.databaseID, "my_database")
		}
	})

	t.Run("omni defaults", func(t *testing.T) {
		opts, err := applyOmniOptions()
		if err != nil {
			t.Fatalf("applyOmniOptions() error = %v, want nil", err)
		}
		if opts.projectID != defaultOmniProjectID {
			t.Fatalf("projectID = %q, want %q", opts.projectID, defaultOmniProjectID)
		}
		if opts.instanceID != defaultOmniInstanceID {
			t.Fatalf("instanceID = %q, want %q", opts.instanceID, defaultOmniInstanceID)
		}
		if opts.databaseID != DefaultDatabaseID {
			t.Fatalf("databaseID = %q, want %q", opts.databaseID, DefaultDatabaseID)
		}
	})

	t.Run("omni database underscores", func(t *testing.T) {
		opts, err := applyOmniOptions(WithDatabaseID("my_database"))
		if err != nil {
			t.Fatalf("applyOmniOptions() error = %v, want nil", err)
		}
		if opts.databaseID != "my_database" {
			t.Fatalf("databaseID = %q, want %q", opts.databaseID, "my_database")
		}
	})

	t.Run("omni random database", func(t *testing.T) {
		opts, err := applyOmniOptions(WithRandomDatabaseID())
		if err != nil {
			t.Fatalf("applyOmniOptions() error = %v, want nil", err)
		}
		if len(opts.databaseID) != idRange {
			t.Fatalf("databaseID length = %d, want %d", len(opts.databaseID), idRange)
		}
	})
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
