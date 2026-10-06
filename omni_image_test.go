package spanemuboost

import (
	"slices"
	"testing"
)

func TestDefaultOmniStartArgsForImage(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, tt := range []struct {
		image  string
		legacy bool
	}{
		{"us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta.1", true},
		{"us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r2-beta", true},
		{"us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r2.1-beta", true},
		{"us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta", true},
		{"mirror.invalid:5000/omni:2026.r3-beta.1", true},
		{"omni:2026.r3-beta", true},
		{"omni:2026.r3", true},
		{"omni:2026-r3", true},
		{"mirror.invalid:5000/omni:custom-2026-r2.1-build", true},
		{"omni:custom_2026.r1_release", true},
		{defaultOmniImage, false},
		{"omni:2026.r4-beta", false},
		{"omni:2026.r5-beta", false},
		{"omni:2027.r1-beta", false},
		{"omni:latest", false},
		{"omni:custom-beta", false},
		{"omni:2026.r3-beta-custom", true},
		{"omni:2026.r3-beta.1.2", true},
		{"omni:2026.r3.1.2-beta", true},
		{"omni:2026.r30-beta", false},
		{"omni:2026-r31-beta", false},
		{"omni:2026.r10-beta", false},
		{"omni:12026.r3-beta", false},
		{"omni:custom2026.r3-beta", false},
		{"omni:2026.r3custom", false},
		{"mirror.invalid:5000/2026.r3-beta/omni:2026.r4-lts", false},
		{"mirror.invalid:5000/2026.r3-beta/omni", false},
		{"omni", false},
		{"omni@" + digest, false},
		{"omni:2026.r3-beta@" + digest, true},
		{"omni:2026.r4-lts@" + digest, false},
	} {
		t.Run(tt.image, func(t *testing.T) {
			want := []string{"--listen-addresses=0.0.0.0"}
			if tt.legacy {
				want = nil
			}
			if got := defaultOmniStartArgs(tt.image); !slices.Equal(got, want) {
				t.Fatalf("startup args = %q, want %q", got, want)
			}
		})
	}
}
