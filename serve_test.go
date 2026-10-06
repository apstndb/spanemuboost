package spanemuboost

import (
	"slices"
	"strings"
	"testing"
)

func TestParseServeArgsImageAndOmniStartMode(t *testing.T) {
	const oldImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"
	for _, tt := range []struct {
		name      string
		args      []string
		backend   Backend
		image     string
		startArgs []string
		explicit  bool
	}{
		{"old image separated", []string{"omni", "--image", oldImage}, BackendOmni, oldImage, nil, false},
		{"auto equals before backend", []string{"--image=" + oldImage, "--omni-start-mode=auto", "omni"}, BackendOmni, oldImage, nil, false},
		{"auto separated", []string{"omni", "--omni-start-mode", "auto"}, BackendOmni, defaultOmniImage, nil, false},
		{"default", []string{"omni"}, BackendOmni, defaultOmniImage, nil, false},
		{"last image wins", []string{"omni", "--image=" + oldImage, "--image", "r4-image"}, BackendOmni, "r4-image", nil, false},
		{"manual legacy separated", []string{"omni", "--image=custom-image", "--omni-start-mode", "legacy"}, BackendOmni, "custom-image", nil, true},
		{"manual GA equals", []string{"omni", "--image=" + oldImage, "--omni-start-mode=ga"}, BackendOmni, oldImage, []string{"--listen-addresses=0.0.0.0"}, true},
		{"last mode restores auto", []string{"omni", "--omni-start-mode=legacy", "--omni-start-mode=auto"}, BackendOmni, defaultOmniImage, nil, false},
		{"last manual mode wins", []string{"omni", "--omni-start-mode=ga", "--omni-start-mode=legacy"}, BackendOmni, defaultOmniImage, nil, true},
		{"emulator image", []string{"emulator", "--image=custom-emulator"}, BackendEmulator, "custom-emulator", nil, false},
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
			if cfg.Backend != tt.backend || opts.emulatorImage != tt.image || opts.omniStartArgsSet != tt.explicit || !slices.Equal(opts.omniStartArgs, tt.startArgs) {
				t.Fatalf("backend/image/startArgs/explicit = %q/%q/%q/%t", cfg.Backend, opts.emulatorImage, opts.omniStartArgs, opts.omniStartArgsSet)
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
		{[]string{"omni", "--image", "--endpoint-file"}, "requires a value"},
		{[]string{"omni", "--omni-start-mode"}, "requires a value"},
		{[]string{"omni", "--omni-start-mode="}, "non-empty"},
		{[]string{"omni", "--omni-start-mode", ""}, "non-empty"},
		{[]string{"omni", "--omni-start-mode=beta"}, "auto, ga or legacy"},
		{[]string{"emulator", "--omni-start-mode=auto"}, "only supported for Spanner Omni"},
		{[]string{"emulator", "--omni-start-mode=ga"}, "only supported for Spanner Omni"},
		{[]string{"emulator", "--omni-start-mode=legacy"}, "only supported for Spanner Omni"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if _, err := ParseServeArgs(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
