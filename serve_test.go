package spanemuboost

import (
	"slices"
	"strings"
	"testing"
)

func TestParseServeArgsImage(t *testing.T) {
	const oldImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"
	gaCmd := []string{"start-single-server", "--listen-addresses=0.0.0.0"}
	legacyCmd := []string{"start-single-server"}
	for _, tt := range []struct {
		name    string
		args    []string
		backend Backend
		image   string
		cmd     []string
	}{
		{"old image separated", []string{"omni", "--image", oldImage}, BackendOmni, oldImage, legacyCmd},
		{"old image equals before backend", []string{"--image=" + oldImage, "omni"}, BackendOmni, oldImage, legacyCmd},
		{"default", []string{"omni"}, BackendOmni, defaultOmniImage, gaCmd},
		{"last image wins", []string{"omni", "--image=" + oldImage, "--image", "r4-image"}, BackendOmni, "r4-image", gaCmd},
		{"emulator image", []string{"emulator", "--image=custom-emulator"}, BackendEmulator, "custom-emulator", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseServeArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			var opts *emulatorOptions
			if cfg.Backend == BackendOmni {
				opts, err = applyOmniOptions(cfg.Options...)
			} else {
				opts, err = applyOptions(cfg.Options...)
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Backend != tt.backend || opts.emulatorImage != tt.image {
				t.Fatalf("backend/image = %q/%q, want %q/%q", cfg.Backend, opts.emulatorImage, tt.backend, tt.image)
			}
			if cfg.Backend == BackendOmni {
				req := captureOmniRequest(t, cfg.Options...)
				if !slices.Equal(req.Cmd, tt.cmd) {
					t.Fatalf("command = %q, want %q", req.Cmd, tt.cmd)
				}
			}
		})
	}
}

func TestParseServeArgsRejectsInvalidImageFlag(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"omni", "--image"}, "requires a value"},
		{[]string{"omni", "--image", ""}, "non-empty"},
		{[]string{"omni", "--image="}, "non-empty"},
		{[]string{"omni", "--image", "--endpoint-file"}, "requires a value"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if _, err := ParseServeArgs(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
