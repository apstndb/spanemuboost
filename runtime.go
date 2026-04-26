package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/api/option"
)

// Backend identifies the runtime implementation to start.
// Callers should use the exported Backend* constants; other values are rejected.
type Backend string

const (
	// BackendEmulator starts the Cloud Spanner Emulator backend.
	BackendEmulator Backend = "emulator"
	// BackendOmni starts the experimental Spanner Omni backend.
	// Backend-specific behavior for Omni may change before v1.
	// Use [RecommendedOmniClientConfig] for external Go clients.
	BackendOmni Backend = "omni"
)

// RuntimeHandle is a package-provided runtime value accepted by [OpenClients]
// and [SetupClients].
//
// Supported handles are started [Runtime] values returned by [Run] or [Setup],
// as well as [*Emulator], [*LazyRuntime], and [*LazyEmulator]. External
// implementations are not supported.
type RuntimeHandle interface {
	spanemuboostRuntime()
}

// Runtime is a started backend-neutral Spanner-compatible test runtime returned
// by [Run] or [Setup].
//
// This backend-neutral API surface is intended to remain the primary public
// entry point. Backend-specific behavior may evolve independently, especially
// for the experimental [BackendOmni] backend.
//
// Implementations are provided by this package.
type Runtime interface {
	RuntimeHandle
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
		databaseDialect:       opts.databaseDialect,
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

// OpenClients and SetupClients intentionally accept either a started Runtime,
// a *LazyRuntime, or a *LazyEmulator without adding another startup method to
// the public Runtime API.
func resolveRuntime(ctx context.Context, runtime RuntimeHandle) (runtimeInstance, error) {
	if runtime == nil {
		return nil, errors.New("spanemuboost: runtime is nil; use *Emulator, *LazyRuntime, *LazyEmulator, or a Runtime returned by Run or Setup")
	}

	switch r := runtime.(type) {
	case *Emulator:
		if r == nil {
			return nil, errors.New("spanemuboost: runtime is a nil *Emulator")
		}
		return r, nil
	case *LazyRuntime:
		if r == nil {
			return nil, errors.New("spanemuboost: runtime is a nil *LazyRuntime")
		}
		return r.get(ctx)
	case *LazyEmulator:
		if r == nil {
			return nil, errors.New("spanemuboost: runtime is a nil *LazyEmulator")
		}
		return r.get(ctx)
	case Runtime:
		if isNilRuntimeValue(r) {
			return nil, fmt.Errorf("spanemuboost: runtime is a nil %T", r)
		}
		switch instance := r.(type) {
		case *Emulator:
			return instance, nil
		case *omniRuntime:
			return instance, nil
		default:
			return nil, fmt.Errorf("spanemuboost: unsupported runtime type %T; use *Emulator, *LazyRuntime, *LazyEmulator, or a Runtime returned by Run or Setup", runtime)
		}
	default:
		return nil, fmt.Errorf("spanemuboost: unsupported runtime type %T; use *Emulator, *LazyRuntime, *LazyEmulator, or a Runtime returned by Run or Setup", runtime)
	}
}

func isNilRuntimeValue(runtime Runtime) bool {
	value := reflect.ValueOf(runtime)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// RuntimeEnv combines a [Runtime] with [Clients] for backend-neutral startup.
// When created with [BackendOmni], backend-specific behavior remains
// experimental.
type RuntimeEnv struct {
	*Clients
	runtime Runtime

	// Pointer-backed to keep exported RuntimeEnv comparable as a value.
	closeState *closeState
}

// Runtime returns the started runtime behind this environment.
func (e *RuntimeEnv) Runtime() Runtime {
	return e.runtime
}

// Close closes the clients and then terminates the runtime.
// Close is nil-safe and idempotent. After the first call, subsequent calls
// return the result of that first call.
func (e *RuntimeEnv) Close() error {
	if e == nil {
		return nil
	}
	return ensureCloseState(&e.closeState).close(func() error {
		var errs []error
		if e.Clients != nil {
			errs = append(errs, e.Clients.Close())
		}
		if e.runtime != nil {
			errs = append(errs, e.runtime.Close())
		}
		return errors.Join(errs...)
	})
}

// Run starts the selected backend and returns it as a backend-neutral runtime.
// When backend is [BackendOmni], backend-specific behavior remains experimental.
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
// When backend is [BackendOmni], backend-specific behavior remains experimental.
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

// Setup starts the selected backend and registers cleanup with
// [testing.TB.Cleanup].
// When backend is [BackendOmni], backend-specific behavior remains experimental.
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
// When backend is [BackendOmni], backend-specific behavior remains experimental.
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
