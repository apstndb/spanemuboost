package spanemuboost

import (
	"errors"
	"slices"
	"strings"
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
		{"explicit empty overrides digest default", []Option{WithContainerImage(customImage), WithOmniStartArgs()}, customImage, legacyCmd, nil},
		{"explicit GA overrides beta selection", []Option{WithContainerImage(oldImage), WithOmniStartArgs("--listen-addresses=0.0.0.0")}, oldImage, gaCmd, nil},
		{"custom argv stays literal", []Option{WithOmniStartArgs("--listen-addresses=0.0.0.0", "a b;$(false)'quoted'")}, ltsImage, []string{"start-single-server", "--listen-addresses=0.0.0.0", "a b;$(false)'quoted'"}, nil},
		{"empty last wins", []Option{WithOmniStartArgs("--log-errors-inline"), WithOmniStartArgs()}, ltsImage, legacyCmd, nil},
		{"nonempty last wins", []Option{WithOmniStartArgs(), WithOmniStartArgs("--listen-addresses=0.0.0.0")}, ltsImage, gaCmd, nil},
		{"image-only customizer selects beta", []Option{WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = oldImage
			return nil
		}))}, oldImage, legacyCmd, nil},
		{"image-only customizer selects GA", []Option{WithContainerImage(oldImage), WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = ltsImage
			return nil
		}))}, ltsImage, gaCmd, nil},
		{"manual args survive image-only customizer", []Option{WithOmniStartArgs("--listen-addresses=0.0.0.0"), WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
			r.Image = oldImage
			return nil
		}))}, oldImage, gaCmd, nil},
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

func TestWithOmniStartArgsCopiesSlices(t *testing.T) {
	args := []string{"--listen-addresses=0.0.0.0", "--log-errors-inline"}
	opt := WithOmniStartArgs(args...)
	args[0] = "caller mutation"
	first, err := applyOmniOptions(opt)
	if err != nil {
		t.Fatal(err)
	}
	first.omniStartArgs[0] = "applied option mutation"
	second, err := applyOmniOptions(opt)
	if err != nil {
		t.Fatal(err)
	}
	if second.omniStartArgs[0] != "--listen-addresses=0.0.0.0" {
		t.Fatalf("reused option aliases another slice: %q", second.omniStartArgs)
	}

	// Each request must own its argv even when customizers mutate it.
	captureErr := errors.New("mutated request")
	second.containerCustomizers = append(second.containerCustomizers, testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
		r.Cmd[1] = "request mutation"
		return captureErr
	}))
	if _, err := newOmni(t.Context(), second); !errors.Is(err, captureErr) {
		t.Fatalf("newOmni() error = %v", err)
	}
	if second.omniStartArgs[0] != "--listen-addresses=0.0.0.0" {
		t.Fatal("request aliases finalized option arguments")
	}
}

func TestWithOmniStartArgsRejectsInvalidUse(t *testing.T) {
	if _, err := applyOmniOptions(WithOmniStartArgs("")); err == nil {
		t.Fatal("empty individual argument accepted")
	}
	for _, options := range [][]Option{
		{WithOmniStartArgs()},
		{DisableBackendGuardrails(), WithOmniStartArgs("--flag")},
	} {
		if _, err := Run(t.Context(), BackendEmulator, options...); err == nil || !strings.Contains(err.Error(), "only supported for Spanner Omni") {
			t.Fatalf("Run(Emulator) error = %v", err)
		}
	}
}

func TestOmniStartArgsSurviveLazyStartup(t *testing.T) {
	args := []string{"legacy-custom-arg"}
	opt := WithOmniStartArgs(args...)
	captureErr := errors.New("lazy request captured")
	lazy := NewLazyRuntime(BackendOmni, opt, WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(r *testcontainers.GenericContainerRequest) error {
		if !slices.Equal(r.Cmd, []string{"start-single-server", "legacy-custom-arg"}) {
			t.Fatalf("lazy Cmd = %q", r.Cmd)
		}
		return captureErr
	})))
	args[0] = "mutation before lazy start"
	if _, err := lazy.Get(t.Context()); !errors.Is(err, captureErr) {
		t.Fatalf("lazy.Get() error = %v", err)
	}
	if err := lazy.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOmniStartupOptionsDoNotReconfigureAttachedRuntime(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend: BackendOmni, URI: "127.0.0.1:15000",
		ProjectID: "default", InstanceID: "default",
	}, WithContainerImage("custom-image"), WithOmniStartArgs(),
		WithContainerCustomizers(testcontainers.CustomizeRequestOption(func(*testcontainers.GenericContainerRequest) error {
			t.Fatal("attach must not start a container")
			return nil
		})))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.URI() != "127.0.0.1:15000" {
		t.Fatalf("URI = %q", runtime.URI())
	}
	// Startup options are not inherited when clients are reopened.
	opts, err := runtime.inheritedOptions()
	if err != nil {
		t.Fatal(err)
	}
	if opts.omniStartArgsSet || opts.emulatorImage != "" {
		t.Fatal("reopened client options inherited startup configuration")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}
