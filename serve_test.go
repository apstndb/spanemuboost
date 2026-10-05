package spanemuboost

import (
	"strings"
	"testing"
)

func TestParseServeArgsImage(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		backend Backend
		image   string
	}{
		{"old image separated", []string{"omni", "--image", "r3-image"}, BackendOmni, "r3-image"},
		{"old image equals before backend", []string{"--image=r3-image", "omni"}, BackendOmni, "r3-image"},
		{"default", []string{"omni"}, BackendOmni, defaultOmniImage},
		{"last image wins", []string{"omni", "--image=r3-image", "--image", "r4-image"}, BackendOmni, "r4-image"},
		{"emulator image", []string{"emulator", "--image=custom-emulator"}, BackendEmulator, "custom-emulator"},
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
			if cfg.Backend != tt.backend || opts.emulatorImage != tt.image || opts.omniStartArgsSet {
				t.Fatalf("backend/image/explicit start args = %q/%q/%t", cfg.Backend, opts.emulatorImage, opts.omniStartArgsSet)
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
		{[]string{"omni", "--omni-start-mode=legacy"}, "unknown argument"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			if _, err := ParseServeArgs(tt.args); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
