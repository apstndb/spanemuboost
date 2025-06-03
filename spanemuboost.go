package spanemuboost

import (
	"context"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
)

const (
	DefaultEmulatorImage = "gcr.io/cloud-spanner-emulator/emulator:1.5.34"
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
}

func (c *Clients) ProjectPath() string  { return projectPath(c.ProjectID) }
func (c *Clients) InstancePath() string { return instancePath(c.ProjectID, c.InstanceID) }
func (c *Clients) DatabasePath() string { return databasePath(c.ProjectID, c.InstanceID, c.DatabaseID) }

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
