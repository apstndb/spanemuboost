package spanemuboost

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Serve starts a backend runtime, writes its [Endpoint] metadata when endpointPath
// is non-empty, and blocks until ctx is canceled. The runtime is closed before
// Serve returns. When an endpoint file was written, it is removed on exit so
// stale metadata is not left behind.
func Serve(ctx context.Context, backend Backend, endpointPath string, options ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime, err := Run(ctx, backend, options...)
	if err != nil {
		return err
	}
	defer func() {
		logCloseError("close runtime after serve", runtime.Close())
	}()

	endpoint, err := EndpointFromRuntime(runtime)
	if err != nil {
		return err
	}
	endpoint.ManagedBy = "spanemuboost serve"
	endpoint.PID = os.Getpid()
	endpoint.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if endpointPath != "" {
		if err := SaveEndpoint(endpointPath, endpoint); err != nil {
			return err
		}
		defer func() {
			if err := os.Remove(endpointPath); err != nil && !os.IsNotExist(err) {
				logCloseError("remove endpoint file after serve", err)
			}
		}()
	}

	<-ctx.Done()
	if err := ctx.Err(); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

// ServeConfig configures [ServeFromConfig].
type ServeConfig struct {
	Backend      Backend
	EndpointFile string
	PIDFile      string
	Options      []Option
}

// ServeFromConfig starts a backend and blocks until interrupted.
func ServeFromConfig(ctx context.Context, cfg ServeConfig) error {
	if cfg.PIDFile != "" {
		if err := os.WriteFile(cfg.PIDFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
			return fmt.Errorf("spanemuboost: write pid file %q: %w", cfg.PIDFile, err)
		}
		defer func() {
			if err := os.Remove(cfg.PIDFile); err != nil && !os.IsNotExist(err) {
				logCloseError("remove pid file after serve", err)
			}
		}()
	}
	return Serve(ctx, cfg.Backend, cfg.EndpointFile, cfg.Options...)
}

// ParseServeArgs parses the arguments to spanemuboost serve.
// --image selects a container image and accepts both --image value and
// --image=value forms. Omni startup detects supported listen-address flags.
func ParseServeArgs(args []string) (ServeConfig, error) {
	cfg := ServeConfig{}
	var backend string
	withDefaultDatabase := false
	var image string
	for i := 0; i < len(args); i++ {
		arg, value, hasValue := strings.Cut(args[i], "=")
		if arg != "--image" {
			arg = args[i]
		}
		switch arg {
		case "--image":
			if !hasValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return ServeConfig{}, fmt.Errorf("%s requires a value", arg)
				}
				i++
				value = args[i]
			}
			if value == "" {
				return ServeConfig{}, fmt.Errorf("%s requires a non-empty value", arg)
			}
			image = value
		case "--endpoint-file", "-o":
			if i+1 >= len(args) {
				return ServeConfig{}, fmt.Errorf("--endpoint-file requires a value")
			}
			cfg.EndpointFile = args[i+1]
			i++
		case "--pid-file":
			if i+1 >= len(args) {
				return ServeConfig{}, fmt.Errorf("--pid-file requires a value")
			}
			cfg.PIDFile = args[i+1]
			i++
		case "--with-default-database":
			withDefaultDatabase = true
		case "emulator", "omni":
			if backend != "" {
				return ServeConfig{}, fmt.Errorf("multiple backends specified: %q and %q", backend, args[i])
			}
			backend = args[i]
		default:
			return ServeConfig{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if backend == "" {
		return ServeConfig{}, fmt.Errorf("usage: spanemuboost serve <emulator|omni> --endpoint-file path [--pid-file path] [--with-default-database] [--image image]")
	}
	switch Backend(backend) {
	case BackendEmulator:
		cfg.Backend = BackendEmulator
	case BackendOmni:
		cfg.Backend = BackendOmni
		if !withDefaultDatabase {
			cfg.Options = append(cfg.Options, DisableAutoConfig())
		}
	default:
		return ServeConfig{}, fmt.Errorf("unsupported serve backend %q; supported values are emulator and omni", backend)
	}
	if image != "" {
		cfg.Options = append(cfg.Options, WithContainerImage(image))
	}
	return cfg, nil
}
