package spanemuboost

import (
	"context"
	"errors"

	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
	"google.golang.org/api/option"
)

// Emulator wraps a Cloud Spanner Emulator container.
// Use [RunEmulator] or [SetupEmulator] to create one.
type Emulator struct {
	container *tcspanner.Container
	opts      *emulatorOptions

	closed   bool
	closeErr error
}

func (*Emulator) spanemuboostRuntime() {}

// URI returns the gRPC endpoint (host:port) of the emulator,
// suitable for use as SPANNER_EMULATOR_HOST.
//
// In serial tests, you can use [testing.T.Setenv] to set the environment variable:
//
//	t.Setenv("SPANNER_EMULATOR_HOST", emu.URI())
//
// Note that [testing.T.Setenv] panics if the test or an ancestor has called [testing.T.Parallel].
// Prefer [Emulator.ClientOptions] when possible.
func (e *Emulator) URI() string {
	return e.container.URI()
}

// ClientOptions returns [option.ClientOption] values configured for connecting
// to this emulator (endpoint, insecure credentials, no authentication).
//
// The endpoint uses the passthrough:/// scheme to bypass gRPC name resolution
// and avoid the slow authentication code path that would otherwise be triggered
// when grpc.NewClient (dns resolver by default) is used by the auth layer.
// This mirrors the approach used by the Spanner client library's
// SPANNER_EMULATOR_HOST handling (googleapis/google-cloud-go#10947), as well as
// the Bigtable and Datastore SDKs for their emulator paths.
//
// Currently the auth layer uses grpc.DialContext (passthrough by default), so
// this is a defensive measure for the planned migration to grpc.NewClient.
func (e *Emulator) ClientOptions() []option.ClientOption {
	return defaultClientOpts(e.container)
}

// Close terminates the emulator container.
// Close is nil-safe and idempotent. After the first call, subsequent calls
// return the result of that first call.
func (e *Emulator) Close() error {
	if e == nil {
		return nil
	}
	if e.closed {
		return e.closeErr
	}
	e.closed = true
	if e.container != nil {
		e.closeErr = e.container.Terminate(context.Background())
	}
	return e.closeErr
}

// Container returns the underlying [*tcspanner.Container] for direct access.
// Most users should use [Emulator.URI] or [Emulator.ClientOptions] instead.
func (e *Emulator) Container() *tcspanner.Container {
	return e.container
}

// ProjectID returns the project ID configured for this emulator.
func (e *Emulator) ProjectID() string { return e.opts.projectID }

// InstanceID returns the instance ID configured for this emulator.
func (e *Emulator) InstanceID() string { return e.opts.instanceID }

// DatabaseID returns the database ID configured for this emulator.
func (e *Emulator) DatabaseID() string { return e.opts.databaseID }

// ProjectPath returns the project resource path.
func (e *Emulator) ProjectPath() string { return projectPath(e.opts.projectID) }

// InstancePath returns the instance resource path.
func (e *Emulator) InstancePath() string { return instancePath(e.opts.projectID, e.opts.instanceID) }

// DatabasePath returns the database resource path.
func (e *Emulator) DatabasePath() string {
	return databasePath(e.opts.projectID, e.opts.instanceID, e.opts.databaseID)
}

func (e *Emulator) inheritedOptions(options ...Option) (*emulatorOptions, error) {
	base := inheritedRuntimeOptions(e.opts)
	return applyOptionsWithBase(base, options...)
}

// LazyEmulator defers emulator startup until first use.
// Use [NewLazyEmulator] in a package-level var, then pass directly to [SetupClients]
// or [OpenClients]. Call [LazyEmulator.Setup] or [LazyEmulator.Get] for standalone access.
// [LazyEmulator.Close] is safe to call even if the emulator was never started (no-op).
type LazyEmulator struct {
	state lazyRuntimeState
	opts  []Option
}

func (*LazyEmulator) spanemuboostRuntime() {}

// NewLazyEmulator creates a [LazyEmulator] that will start an emulator with the
// given options on first use. The emulator is not started until it is passed to
// [SetupClients], [OpenClients], or until [LazyEmulator.Setup] / [LazyEmulator.Get]
// is called directly.
func NewLazyEmulator(options ...Option) *LazyEmulator {
	return &LazyEmulator{opts: options}
}

func (le *LazyEmulator) get(ctx context.Context) (runtimeInstance, error) {
	return le.state.get(ctx, func(ctx context.Context) (runtimeInstance, error) {
		return RunEmulator(ctx, le.opts...)
	}, "spanemuboost: lazy emulator used after Close was called before initialization")
}

// Get starts the emulator on first call (thread-safe via [sync.Once]) and
// returns the cached [*Emulator] on subsequent calls.
func (le *LazyEmulator) Get(ctx context.Context) (*Emulator, error) {
	runtime, err := le.get(ctx)
	if err != nil {
		return nil, err
	}
	emu, ok := runtime.(*Emulator)
	if !ok {
		return nil, errors.New("spanemuboost: lazy emulator returned unexpected runtime type")
	}
	return emu, nil
}

// Close terminates the emulator if it was started. No-op otherwise.
// Close is nil-safe and idempotent — subsequent calls return the result of the first call.
// Close waits for any in-progress initialization to complete before checking.
// If Close is called before any Get or Setup, the emulator will never be started.
func (le *LazyEmulator) Close() error {
	if le == nil {
		return nil
	}
	return le.state.close()
}

// Env combines an [Emulator] with [Clients] for the single-call use case.
// Use [RunEmulatorWithClients] or [SetupEmulatorWithClients] to create one.
type Env struct {
	*Clients
	emulator *Emulator

	closed   bool
	closeErr error
}

// Emulator returns the underlying [Emulator].
func (e *Env) Emulator() *Emulator {
	return e.emulator
}

// Close closes the clients and then terminates the emulator.
// Close is nil-safe and idempotent. After the first call, subsequent calls
// return the result of that first call.
func (e *Env) Close() error {
	if e == nil {
		return nil
	}
	if e.closed {
		return e.closeErr
	}
	e.closed = true

	var errs []error
	if e.Clients != nil {
		errs = append(errs, e.Clients.Close())
	}
	if e.emulator != nil {
		errs = append(errs, e.emulator.Close())
	}
	e.closeErr = errors.Join(errs...)
	return e.closeErr
}
