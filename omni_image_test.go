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
		{defaultOmniImage, false},
		{"omni:2026.r4-beta", false},
		{"omni:2026.r5-beta", false},
		{"omni:2027.r1-beta", false},
		{"omni:latest", false},
		{"omni:custom-beta", false},
		{"omni:2026.r3-beta-custom", false},
		{"omni:2026.r3-beta.1.2", false},
		{"omni:2026.r3.1.2-beta", false},
		{"mirror.invalid:5000/2026.r3-beta/omni", false},
		{"omni", false},
		{"omni@" + digest, false},
		{"omni:2026.r3-beta@" + digest, false},
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
