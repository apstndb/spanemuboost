package spanemuboost

import (
	"context"
	"fmt"
	"os"
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

// ServeConfig configures [ServeFromArgs].
type ServeConfig struct {
	Backend      Backend
	EndpointFile string
	Options      []Option
}

// ServeFromConfig starts a backend and blocks until interrupted.
func ServeFromConfig(ctx context.Context, cfg ServeConfig) error {
	return Serve(ctx, cfg.Backend, cfg.EndpointFile, cfg.Options...)
}

// ParseServeArgs parses `spanemuboost serve <emulator|omni> [--endpoint-file path]`.
func ParseServeArgs(args []string) (ServeConfig, error) {
	cfg := ServeConfig{}
	var backend string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint-file", "-o":
			if i+1 >= len(args) {
				return ServeConfig{}, fmt.Errorf("--endpoint-file requires a value")
			}
			cfg.EndpointFile = args[i+1]
			i++
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
		return ServeConfig{}, fmt.Errorf("usage: spanemuboost serve <emulator|omni> [--endpoint-file path]")
	}
	switch Backend(backend) {
	case BackendEmulator:
		cfg.Backend = BackendEmulator
	case BackendOmni:
		cfg.Backend = BackendOmni
	default:
		return ServeConfig{}, fmt.Errorf("unsupported serve backend %q; supported values are emulator and omni", backend)
	}
	return cfg, nil
}
