package spanemuboost

import (
	"context"
	"errors"
	"sync"

	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
	"google.golang.org/api/option"
)

// abstractEmulator is satisfied by both [*Emulator] and [*LazyEmulator].
// The unexported method prevents external implementations.
// It allows [OpenClients] and [SetupClients] to accept either type.
type abstractEmulator interface {
	get(context.Context) (*Emulator, error)
}

// Emulator wraps a Cloud Spanner Emulator container.
// Use [RunEmulator] or [SetupEmulator] to create one.
type Emulator struct {
	container *tcspanner.Container
	opts      *emulatorOptions
}

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
func (e *Emulator) Close() error {
	return e.container.Terminate(context.Background())
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

func (e *Emulator) get(_ context.Context) (*Emulator, error) {
	return e, nil
}

// LazyEmulator defers emulator startup until first use.
// Use [NewLazyEmulator] in a package-level var, then pass directly to [SetupClients]
// or [OpenClients]. Call [LazyEmulator.Setup] or [LazyEmulator.Get] for standalone access.
// [LazyEmulator.Close] is safe to call even if the emulator was never started (no-op).
type LazyEmulator struct {
	once sync.Once
	emu  *Emulator
	err  error
	opts []Option

	closeOnce sync.Once
	closeErr  error
}

// NewLazyEmulator creates a [LazyEmulator] that will start an emulator with the
// given options on first use. The emulator is not started until it is passed to
// [SetupClients], [OpenClients], or until [LazyEmulator.Setup] / [LazyEmulator.Get]
// is called directly.
func NewLazyEmulator(options ...Option) *LazyEmulator {
	return &LazyEmulator{opts: options}
}

func (le *LazyEmulator) get(ctx context.Context) (*Emulator, error) {
	le.once.Do(func() {
		le.emu, le.err = RunEmulator(ctx, le.opts...)
	})
	if le.emu == nil && le.err == nil {
		return nil, errors.New("spanemuboost: lazy emulator used after Close was called before initialization")
	}
	return le.emu, le.err
}

// Get starts the emulator on first call (thread-safe via [sync.Once]) and
// returns the cached [*Emulator] on subsequent calls.
func (le *LazyEmulator) Get(ctx context.Context) (*Emulator, error) {
	return le.get(ctx)
}

// Close terminates the emulator if it was started. No-op otherwise.
// Close is idempotent — subsequent calls return the result of the first call.
// Close waits for any in-progress initialization to complete before checking.
// If Close is called before any Get or Setup, the emulator will never be started.
func (le *LazyEmulator) Close() error {
	le.once.Do(func() {})
	le.closeOnce.Do(func() {
		if le.emu != nil {
			le.closeErr = le.emu.Close()
		}
	})
	return le.closeErr
}

// Env combines an [Emulator] with [Clients] for the single-call use case.
// Use [RunEmulatorWithClients] or [SetupEmulatorWithClients] to create one.
type Env struct {
	*Clients
	emulator *Emulator
}

// Emulator returns the underlying [Emulator].
func (e *Env) Emulator() *Emulator {
	return e.emulator
}

// Close closes the clients and then terminates the emulator.
func (e *Env) Close() error {
	return errors.Join(
		e.Clients.Close(),
		e.emulator.Close(),
	)
}
