package spanemuboost

import (
	"cloud.google.com/go/spanner"
	"context"
	"fmt"
	"log"
)

import (
	"github.com/apstndb/lox"
	"github.com/testcontainers/testcontainers-go"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/samber/lo"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
)

type noopLogger struct{}

// Printf implements testcontainers.Logging.
func (n noopLogger) Printf(string, ...interface{}) {
}

const DefaultEmulatorImage = "gcr.io/cloud-spanner-emulator/emulator:1.5.25"

type emulatorOptions struct {
	EmulatorImage                     string
	ProjectID, InstanceID, DatabaseID string
	DisableAutoConfig                 bool
	DatabaseDialect                   databasepb.DatabaseDialect
}

type Option func(*emulatorOptions) error

func WithProjectID(projectID string) Option {
	return func(opts *emulatorOptions) error {
		opts.ProjectID = projectID
		return nil
	}
}

func WithInstanceID(instanceID string) Option {
	return func(opts *emulatorOptions) error {
		opts.InstanceID = instanceID
		return nil
	}
}

func WithDatabaseID(databaseID string) Option {
	return func(opts *emulatorOptions) error {
		opts.DatabaseID = databaseID
		return nil
	}
}

func WithDatabaseDialect(dialect databasepb.DatabaseDialect) Option {
	return func(opts *emulatorOptions) error {
		opts.DatabaseDialect = dialect
		return nil
	}
}

func WithEmulatorImage(image string) Option {
	return func(opts *emulatorOptions) error {
		opts.EmulatorImage = image
		return nil
	}
}

func DisableAutoConfig(opts *emulatorOptions) Option {
	return func(opts *emulatorOptions) error {
		opts.DisableAutoConfig = true
		return nil
	}
}

func (o *emulatorOptions) DatabasePath() string {
	return databasePath(o.ProjectID, o.InstanceID, o.DatabaseID)
}

func (o *emulatorOptions) InstancePath() string {
	return instancePath(o.ProjectID, o.InstanceID)
}

func (o *emulatorOptions) ProjectPath() string {
	return projectPath(o.ProjectID)
}

type Clients struct {
	InstanceClient *instance.InstanceAdminClient
	DatabaseClient *database.DatabaseAdminClient
	Client         *spanner.Client
}

func applyOptions(options ...Option) (*emulatorOptions, error) {
	var opts emulatorOptions
	for _, opt := range options {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	return &opts, nil
}

func NewEmulator(ctx context.Context, options ...Option) (container *gcloud.GCloudContainer, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, err
	}
	return newEmulator(ctx, opts)
}

func newEmulator(ctx context.Context, opts *emulatorOptions) (container *gcloud.GCloudContainer, teardown func(), err error) {
	// Workaround to suppress log output with `-v`.
	testcontainers.Logger = &noopLogger{}
	container, err = gcloud.RunSpanner(ctx, lo.CoalesceOrEmpty(opts.EmulatorImage, DefaultEmulatorImage), testcontainers.WithLogger(&noopLogger{}))
	if err != nil {
		return nil, nil, err
	}

	teardown = func() {
		err := container.Terminate(ctx)
		if err != nil {
			log.Printf("failed to terminate Cloud Spanner Emulator: %v", err)
		}
	}

	if !opts.DisableAutoConfig {
		clientOpts := defaultClientOpts(container)

		if err = bootstrap(ctx, opts, clientOpts...); err != nil {
			teardown()
			return nil, nil, err
		}
	}

	return container, teardown, nil
}

func bootstrap(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) (err error) {
	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	defer instanceCli.Close()

	createInstanceOp, err := instanceCli.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     opts.ProjectPath(),
		InstanceId: opts.InstanceID,
		Instance: &instancepb.Instance{
			Name:        opts.InstancePath(),
			Config:      "emulator-config",
			DisplayName: opts.InstanceID,
		},
	})
	if err != nil {
		return err
	}

	_, err = createInstanceOp.Wait(ctx)
	if err != nil {
		return err
	}

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	defer dbCli.Close()

	createDBOp, err := dbCli.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          opts.InstancePath(),
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%v`", opts.DatabaseID),
		DatabaseDialect: opts.DatabaseDialect,
	})
	if err != nil {
		return err
	}

	_, err = createDBOp.Wait(ctx)
	if err != nil {
		return err
	}
	return nil
}

func NewClients(ctx context.Context, options ...Option) (clients *Clients, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, err
	}

	return newClients(ctx, opts)
}

func newClients(ctx context.Context, opts *emulatorOptions) (clients *Clients, teardown func(), err error) {
	emulator, teardown, err := newEmulator(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	clientOpts := defaultClientOpts(emulator)

	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, err
	}

	teardown = func() {
		teardown := teardown
		if err := instanceCli.Close(); err != nil {
			log.Printf("failed to instanceAdminClient.Close(): %v", err)
		}
		teardown()
	}

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, err
	}

	teardown = func() {
		teardown := teardown
		if err := dbCli.Close(); err != nil {
			log.Printf("failed to databaseAdminClient.Close(): %v", err)
		}
		teardown()
	}

	client, err := spanner.NewClient(ctx, opts.DatabasePath(), clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, err
	}

	return &Clients{
			InstanceClient: instanceCli,
			DatabaseClient: dbCli,
			Client:         client,
		},
		func() {
			teardown := teardown
			client.Close()
			teardown()
		}, nil
}

func defaultClientOpts(emulator *gcloud.GCloudContainer) []option.ClientOption {
	clientOpts := lox.SliceOf(
		option.WithEndpoint(emulator.URI),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	return clientOpts
}

func projectPath(projectID string) string {
	return fmt.Sprintf("projects/%v", projectID)
}

func instancePath(projectID, instanceID string) string {
	return fmt.Sprintf("projects/%v/instances/%v", projectID, instanceID)
}

func databasePath(projectID, instanceID, databaseID string) string {
	return fmt.Sprintf("projects/%v/instances/%v/databases/%v", projectID, instanceID, databaseID)
}
