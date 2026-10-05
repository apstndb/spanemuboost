package spanemuboost

import (
	"slices"
	"strings"
	"testing"
)

func TestParseServeArgsImageAndOmniStartMode(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		backend   Backend
		image     string
		startArgs []string
	}{
		{"legacy separated", []string{"omni", "--image", "r3-image", "--omni-start-mode", "legacy"}, BackendOmni, "r3-image", nil},
		{"legacy equals before backend", []string{"--image=r3-image", "--omni-start-mode=legacy", "omni"}, BackendOmni, "r3-image", nil},
		{"default separated", []string{"omni", "--image", "r4-image", "--omni-start-mode", "default"}, BackendOmni, "r4-image", []string{"--listen-addresses=0.0.0.0"}},
		{"default equals", []string{"omni", "--image=r4-image", "--omni-start-mode=default"}, BackendOmni, "r4-image", []string{"--listen-addresses=0.0.0.0"}},
		{"last mode wins", []string{"omni", "--omni-start-mode=legacy", "--omni-start-mode=default"}, BackendOmni, defaultOmniImage, []string{"--listen-addresses=0.0.0.0"}},
		{"image alone", []string{"omni", "--image=custom-image"}, BackendOmni, "custom-image", []string{"--listen-addresses=0.0.0.0"}},
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
			if cfg.Backend != tt.backend || opts.emulatorImage != tt.image || !slices.Equal(opts.omniStartArgs, tt.startArgs) {
				t.Fatalf("backend/image/startArgs = %q/%q/%q", cfg.Backend, opts.emulatorImage, opts.omniStartArgs)
			}
		})
	}
}

func TestParseServeArgsRejectsInvalidStartupFlags(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"omni", "--image"}, "requires a value"},
		{[]string{"omni", "--image", ""}, "non-empty"},
		{[]string{"omni", "--image="}, "non-empty"},
		{[]string{"omni", "--image", "--omni-start-mode=legacy"}, "requires a value"},
		{[]string{"omni", "--omni-start-mode"}, "requires a value"},
		{[]string{"omni", "--omni-start-mode="}, "non-empty"},
		{[]string{"omni", "--omni-start-mode", ""}, "non-empty"},
		{[]string{"omni", "--omni-start-mode=unknown"}, "default or legacy"},
		{[]string{"emulator", "--omni-start-mode=legacy"}, "only supported for Spanner Omni"},
		{[]string{"emulator", "--omni-start-mode=default"}, "only supported for Spanner Omni"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if _, err := ParseServeArgs(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
