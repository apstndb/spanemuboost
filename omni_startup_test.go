package spanemuboost

import (
	"errors"
	"slices"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

func captureOmniRequest(t *testing.T, options ...Option) testcontainers.GenericContainerRequest {
	t.Helper()
	var req testcontainers.GenericContainerRequest
	captureErr := errors.New("capture request before Docker")
	options = append(options, WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
		req = *r
		return captureErr
	})))
	opts, err := applyOmniOptions(options...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newOmni(t.Context(), opts); !errors.Is(err, captureErr) {
		t.Fatalf("newOmni() error = %v, want capture error", err)
	}
	return req
}

func TestOmniStartupConfiguration(t *testing.T) {
	const ltsImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r4-lts"
	const oldImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r3-beta"
	const customImage = "example.invalid/omni@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	gaCmd := []string{"start-single-server", "--listen-addresses=0.0.0.0"}
	legacyCmd := []string{"start-single-server"}
	for _, tt := range []struct {
		name       string
		options    []Option
		image      string
		cmd        []string
		entrypoint []string
	}{
		{"LTS default", nil, ltsImage, gaCmd, nil},
		{"pre-GA beta selected automatically", []Option{WithContainerImage(oldImage)}, oldImage, legacyCmd, nil},
		{"pre-GA tagged digest selected automatically", []Option{WithContainerImage(oldImage + "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")}, oldImage + "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", legacyCmd, nil},
		{"digest defaults to GA", []Option{WithContainerImage(customImage)}, customImage, gaCmd, nil},
		{"image-only customizer selects beta", []Option{WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = oldImage
			return nil
		}))}, oldImage, legacyCmd, nil},
		{"image-only customizer selects GA", []Option{WithContainerImage(oldImage), WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = ltsImage
			return nil
		}))}, ltsImage, gaCmd, nil},
		{"in-place command customizer wins", []Option{WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = oldImage
			r.Cmd[1] = "--log-errors-inline"
			return nil
		}))}, oldImage, []string{"start-single-server", "--log-errors-inline"}, nil},
		{"manual command survives later image-only customizer", []Option{WithContainerCustomizers(
			testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
				r.Cmd = []string{"custom-subcommand", "custom-arg"}
				return nil
			}),
			testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
				r.Image = oldImage
				return nil
			}),
		)}, oldImage, []string{"custom-subcommand", "custom-arg"}, nil},
		{"command customizer wins", []Option{WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Cmd = []string{"custom-subcommand", "custom-arg"}
			return nil
		}))}, ltsImage, []string{"custom-subcommand", "custom-arg"}, nil},
		{"entrypoint customizer wins", []Option{WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Entrypoint = []string{"custom-entrypoint"}
			return nil
		}))}, ltsImage, gaCmd, []string{"custom-entrypoint"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := captureOmniRequest(t, tt.options...)
			if req.Image != tt.image || !slices.Equal(req.Cmd, tt.cmd) || !slices.Equal(req.Entrypoint, tt.entrypoint) {
				t.Fatalf("request image/cmd/entrypoint = %q/%q/%q, want %q/%q/%q", req.Image, req.Cmd, req.Entrypoint, tt.image, tt.cmd, tt.entrypoint)
			}
		})
	}
}
