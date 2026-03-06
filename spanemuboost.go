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

// Clients struct is container of Spanner clients.
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
// If [WithStrictTeardown] was used, any auto-created database or instance is
// dropped before the clients are closed.
// [spanner.Client.Close] does not return an error, so only admin client and
// resource cleanup errors are returned.
func (c *Clients) Close() error {
	c.Client.Close()

	var dropErrs []error
	ctx := context.Background()
	if c.dropDatabase {
		if err := c.DatabaseClient.DropDatabase(ctx, &databasepb.DropDatabaseRequest{
			Database: c.DatabasePath(),
		}); err != nil {
			dropErrs = append(dropErrs, fmt.Errorf("drop database %s: %w", c.DatabasePath(), err))
		}
	}
	if c.dropInstance {
		if err := c.InstanceClient.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{
			Name: c.InstancePath(),
		}); err != nil {
			dropErrs = append(dropErrs, fmt.Errorf("delete instance %s: %w", c.InstancePath(), err))
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

	if err = bootstrap(ctx, opts, emu.ClientOptions()...); err != nil {
		_ = emu.Close()
		return nil, err
	}

	clients, err := newClientsFromEmulator(ctx, emu, opts)
	if err != nil {
		_ = emu.Close()
		return nil, err
	}

	return &Env{Clients: clients, emulator: emu}, nil
}

// OpenClients connects to an existing [Emulator] and opens Spanner clients.
// Options inherit the emulator's projectID and instanceID; instance creation
// is disabled by default (use [EnableAutoConfig] to override).
// Call [Clients.Close] to close the clients when done.
func OpenClients(ctx context.Context, emu *Emulator, options ...Option) (*Clients, error) {
	base := &emulatorOptions{
		projectID:             emu.opts.projectID,
		instanceID:            emu.opts.instanceID,
		disableCreateInstance: true,
	}

	opts, err := applyOptionsWithBase(base, options...)
	if err != nil {
		return nil, err
	}

	if err := bootstrap(ctx, opts, emu.ClientOptions()...); err != nil {
		return nil, err
	}

	return newClientsFromEmulator(ctx, emu, opts)
}

// Deprecated: Use [RunEmulator] instead.
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

// Deprecated: Use [RunEmulatorWithClients] instead.
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

// Deprecated: Use [OpenClients] instead.
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
