package spanemuboost

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
)

const (
	DefaultEmulatorImage = "gcr.io/cloud-spanner-emulator/emulator:1.5.50"
	DefaultProjectID     = "emulator-project"
	DefaultInstanceID    = "emulator-instance"
	DefaultDatabaseID    = "emulator-database"
)

// Clients holds Spanner clients and manages the lifecycle of schema resources
// (instances and databases) auto-created during bootstrap.
//
// By default, auto-created resources with fixed IDs are dropped on [Clients.Close],
// while resources with random IDs are not (since they never collide).
// Use [ForceSchemaTeardown] or [SkipSchemaTeardown] to override.
//
// For [RunEmulatorWithClients]/[SetupEmulatorWithClients], teardown is disabled
// because the emulator container owns the resource lifecycle;
// use [ForceSchemaTeardown] to override.
type Clients struct {
	InstanceClient *instance.InstanceAdminClient
	DatabaseClient *database.DatabaseAdminClient
	Client         *spanner.Client

	ProjectID, InstanceID, DatabaseID string

	dropDatabase bool
	dropInstance bool
}

func (c *Clients) ProjectPath() string  { return projectPath(c.ProjectID) }
func (c *Clients) InstancePath() string { return instancePath(c.ProjectID, c.InstanceID) }
func (c *Clients) DatabasePath() string { return databasePath(c.ProjectID, c.InstanceID, c.DatabaseID) }

// Close closes all Spanner clients.
// By default, auto-created resources with fixed IDs are dropped before
// the clients are closed. See [ForceSchemaTeardown] and [SkipSchemaTeardown].
// [spanner.Client.Close] does not return an error, so only admin client and
// resource cleanup errors are returned.
func (c *Clients) Close() error {
	c.Client.Close()

	var dropErrs []error
	ctx := context.Background()
	if c.dropInstance {
		// Deleting the instance also removes all databases within it,
		// so there is no need to drop the database separately.
		if err := c.InstanceClient.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{
			Name: c.InstancePath(),
		}); err != nil {
			dropErrs = append(dropErrs, fmt.Errorf("delete instance %s: %w", c.InstancePath(), err))
		}
	} else if c.dropDatabase {
		if err := c.DatabaseClient.DropDatabase(ctx, &databasepb.DropDatabaseRequest{
			Database: c.DatabasePath(),
		}); err != nil {
			dropErrs = append(dropErrs, fmt.Errorf("drop database %s: %w", c.DatabasePath(), err))
		}
	}

	return errors.Join(append(dropErrs,
		c.DatabaseClient.Close(),
		c.InstanceClient.Close(),
	)...)
}

// RunEmulator starts a Cloud Spanner Emulator container and performs any
// configured bootstrap (instance/database creation, DDL, DML).
// Call [Emulator.Close] to terminate the container when done.
// In tests, prefer [SetupEmulator] which handles cleanup automatically.
// In TestMain, use this function since [testing.M] does not implement [testing.TB].
func RunEmulator(ctx context.Context, options ...Option) (*Emulator, error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, err
	}

	container, _, err := newEmulator(ctx, opts)
	if err != nil {
		return nil, err
	}

	emu := &Emulator{container: container, opts: opts}

	if err = bootstrap(ctx, opts, emu.ClientOptions()...); err != nil {
		_ = emu.Close()
		return nil, err
	}

	return emu, nil
}

// RunEmulatorWithClients starts a Cloud Spanner Emulator and opens Spanner clients.
// Call [Env.Close] to close clients and terminate the container.
// In tests, prefer [SetupEmulatorWithClients] which handles cleanup automatically.
func RunEmulatorWithClients(ctx context.Context, options ...Option) (*Env, error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, err
	}

	container, _, err := newEmulator(ctx, opts)
	if err != nil {
		return nil, err
	}

	emu := &Emulator{container: container, opts: opts}

	clients, err := bootstrapAndCreateClients(ctx, emu, opts)
	if err != nil {
		_ = emu.Close()
		return nil, err
	}

	// Env owns the emulator lifecycle — resources are cleaned up when the
	// container terminates, so disable schema teardown unless explicitly forced.
	forceTeardown := opts.schemaTeardown != nil && *opts.schemaTeardown
	if !forceTeardown {
		clients.dropDatabase = false
		clients.dropInstance = false
	}

	return &Env{Clients: clients, emulator: emu}, nil
}

// OpenClients connects to an existing emulator and opens Spanner clients.
// The emu parameter accepts both [*Emulator] and [*LazyEmulator].
// When a [*LazyEmulator] is passed, the emulator is started automatically on first use.
// Options inherit the emulator's projectID and instanceID; instance creation
// is disabled by default (use [EnableAutoConfig] to override).
// Call [Clients.Close] to close the clients when done.
// In tests, prefer [SetupClients] which handles cleanup automatically.
func OpenClients(ctx context.Context, emu abstractEmulator, options ...Option) (*Clients, error) {
	e, err := emu.get(ctx)
	if err != nil {
		return nil, err
	}

	base := &emulatorOptions{
		projectID:             e.opts.projectID,
		instanceID:            e.opts.instanceID,
		disableCreateInstance: true,
	}

	opts, err := applyOptionsWithBase(base, options...)
	if err != nil {
		return nil, err
	}

	return bootstrapAndCreateClients(ctx, e, opts)
}

// Deprecated: Use [SetupEmulator] (for tests) or [RunEmulator] instead.
//
// NewEmulator initializes Cloud Spanner Emulator.
// The emulator will be closed when teardown is called. You should call it.
func NewEmulator(ctx context.Context, options ...Option) (emulator *tcspanner.Container, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, err
	}

	emulator, teardown, err = newEmulator(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	if err = bootstrap(ctx, opts, defaultClientOpts(emulator)...); err != nil {
		teardown()
		return nil, nil, err
	}

	return emulator, teardown, nil
}

// Deprecated: Use [SetupEmulatorWithClients] (for tests) or [RunEmulatorWithClients] instead.
//
// NewEmulatorWithClients initializes Cloud Spanner Emulator with Spanner clients.
// The emulator and clients will be closed when teardown is called. You should call it.
func NewEmulatorWithClients(ctx context.Context, options ...Option) (emulator *tcspanner.Container, clients *Clients, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, nil, err
	}

	emulator, emulatorTeardown, err := newEmulator(ctx, opts)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = bootstrap(ctx, opts, defaultClientOpts(emulator)...); err != nil {
		emulatorTeardown()
		return nil, nil, nil, err
	}

	clients, clientsTeardown, err := newClients(ctx, emulator, opts)
	if err != nil {
		emulatorTeardown()
		return nil, nil, nil, err
	}

	return emulator, clients, func() {
		clientsTeardown()
		emulatorTeardown()
	}, nil
}

// Deprecated: Use [SetupClients] (for tests) or [OpenClients] instead.
//
// NewClients setup existing Cloud Spanner Emulator with Spanner clients.
// The clients will be closed when teardown is called. You should call it.
func NewClients(ctx context.Context, emulator *tcspanner.Container, options ...Option) (clients *Clients, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, err
	}

	if err := bootstrap(ctx, opts, defaultClientOpts(emulator)...); err != nil {
		return nil, nil, err
	}

	return newClients(ctx, emulator, opts)
}
