package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/option"
)

// Backend identifies the runtime implementation to start.
type Backend string

const (
	// BackendEmulator starts the Cloud Spanner Emulator backend.
	BackendEmulator Backend = "emulator"
	// BackendOmni starts the experimental Spanner Omni backend.
	// Use [RecommendedOmniClientConfig] for external Go clients.
	BackendOmni Backend = "omni"
)

// Runtime is a started Spanner-compatible test runtime.
type Runtime interface {
	URI() string
	ClientOptions() []option.ClientOption
	Close() error
	ProjectID() string
	InstanceID() string
	DatabaseID() string
	ProjectPath() string
	InstancePath() string
	DatabasePath() string
}

type runtimeInstance interface {
	Runtime
	inheritedOptions(...Option) (*emulatorOptions, error)
}

func inheritedRuntimeOptions(opts *emulatorOptions) *emulatorOptions {
	base := &emulatorOptions{
		projectID:             opts.projectID,
		instanceID:            opts.instanceID,
		databaseID:            opts.databaseID,
		disableCreateInstance: true,
		disableCreateDatabase: true,
		reuseExistingDatabase: true,
	}
	if opts.clientConfig != nil {
		config := *opts.clientConfig
		base.clientConfig = &config
	}
	return base
}

func disableSchemaTeardownUnlessForced(opts *emulatorOptions, clients *Clients) {
	forceTeardown := opts.schemaTeardown != nil && *opts.schemaTeardown
	if !forceTeardown {
		clients.dropDatabase = false
		clients.dropInstance = false
	}
}

func resolveRuntime(ctx context.Context, runtime any) (runtimeInstance, error) {
	switch r := runtime.(type) {
	case *Emulator:
		return r, nil
	case *LazyEmulator:
		return r.get(ctx)
	case Runtime:
		instance, ok := r.(runtimeInstance)
		if !ok {
			return nil, fmt.Errorf("spanemuboost: unsupported runtime type %T; use *Emulator, *LazyEmulator, or a Runtime returned by Run or Setup", runtime)
		}
		return instance, nil
	default:
		return nil, fmt.Errorf("spanemuboost: unsupported runtime type %T; use *Emulator, *LazyEmulator, or a Runtime returned by Run or Setup", runtime)
	}
}

// RuntimeEnv combines a [Runtime] with [Clients] for backend-neutral startup.
type RuntimeEnv struct {
	*Clients
	runtime Runtime
}

// Runtime returns the started runtime behind this environment.
func (e *RuntimeEnv) Runtime() Runtime {
	return e.runtime
}

// Close closes the clients and then terminates the runtime.
func (e *RuntimeEnv) Close() error {
	if e == nil {
		return nil
	}

	var errs []error
	if e.Clients != nil {
		errs = append(errs, e.Clients.Close())
	}
	if e.runtime != nil {
		errs = append(errs, e.runtime.Close())
	}
	return errors.Join(errs...)
}

// Run starts the selected backend and returns it as a backend-neutral runtime.
func Run(ctx context.Context, backend Backend, options ...Option) (Runtime, error) {
	switch backend {
	case BackendEmulator:
		return RunEmulator(ctx, options...)
	case BackendOmni:
		return runOmni(ctx, options...)
	default:
		return nil, fmt.Errorf("unsupported backend %q", backend)
	}
}

// RunWithClients starts the selected backend and returns managed clients.
func RunWithClients(ctx context.Context, backend Backend, options ...Option) (*RuntimeEnv, error) {
	switch backend {
	case BackendEmulator:
		env, err := RunEmulatorWithClients(ctx, options...)
		if err != nil {
			return nil, err
		}
		return &RuntimeEnv{Clients: env.Clients, runtime: env.Emulator()}, nil
	case BackendOmni:
		return runOmniWithClients(ctx, options...)
	default:
		return nil, fmt.Errorf("unsupported backend %q", backend)
	}
}

// Setup starts the selected backend and registers cleanup with [testing.TB.Cleanup].
func Setup(tb testing.TB, backend Backend, options ...Option) Runtime {
	tb.Helper()

	switch backend {
	case BackendEmulator:
		return SetupEmulator(tb, options...)
	case BackendOmni:
		return setupOmni(tb, options...)
	default:
		tb.Fatalf("unsupported backend %q", backend)
		return nil
	}
}

// SetupWithClients starts the selected backend with managed clients and
// registers cleanup with [testing.TB.Cleanup].
func SetupWithClients(tb testing.TB, backend Backend, options ...Option) *RuntimeEnv {
	tb.Helper()

	switch backend {
	case BackendEmulator:
		env := SetupEmulatorWithClients(tb, options...)
		return &RuntimeEnv{Clients: env.Clients, runtime: env.Emulator()}
	case BackendOmni:
		return setupOmniWithClients(tb, options...)
	default:
		tb.Fatalf("unsupported backend %q", backend)
		return nil
	}
}
