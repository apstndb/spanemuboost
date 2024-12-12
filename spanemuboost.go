package spanemuboost

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type noopLogger struct{}

// Printf implements testcontainers.Logging.
func (n noopLogger) Printf(string, ...interface{}) {
}

const (
	DefaultEmulatorImage = "gcr.io/cloud-spanner-emulator/emulator:1.5.25"
	DefaultProjectID     = "emulator-project"
	DefaultInstanceID    = "emulator-instance"
	DefaultDatabaseID    = "emulator-database"
)

type emulatorOptions struct {
	emulatorImage                     string
	projectID, instanceID, databaseID string
	disableAutoConfig                 bool
	databaseDialect                   databasepb.DatabaseDialect
	setupDDLs                         []string
	setupDMLs                         []spanner.Statement
}

type Option func(*emulatorOptions) error

func WithProjectID(projectID string) Option {
	return func(opts *emulatorOptions) error {
		opts.projectID = projectID
		return nil
	}
}

func WithInstanceID(instanceID string) Option {
	return func(opts *emulatorOptions) error {
		opts.instanceID = instanceID
		return nil
	}
}

func WithDatabaseID(databaseID string) Option {
	return func(opts *emulatorOptions) error {
		opts.databaseID = databaseID
		return nil
	}
}

func WithDatabaseDialect(dialect databasepb.DatabaseDialect) Option {
	return func(opts *emulatorOptions) error {
		opts.databaseDialect = dialect
		return nil
	}
}

func WithEmulatorImage(image string) Option {
	return func(opts *emulatorOptions) error {
		opts.emulatorImage = image
		return nil
	}
}

func WithSetupDDLs(ddls []string) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDDLs = ddls
		return nil
	}
}

func WithSetupRawDMLs(rawDMLs []string) Option {
	return func(opts *emulatorOptions) error {
		dmlStmts := make([]spanner.Statement, 0, len(rawDMLs))
		for _, rawDML := range rawDMLs {
			dmlStmts = append(dmlStmts, spanner.NewStatement(rawDML))
		}

		opts.setupDMLs = dmlStmts
		return nil
	}
}

func WithSetupDMLs(dmls []spanner.Statement) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDMLs = dmls
		return nil
	}
}

func DisableAutoConfig(opts *emulatorOptions) Option {
	return func(opts *emulatorOptions) error {
		opts.disableAutoConfig = true
		return nil
	}
}

func (o *emulatorOptions) DatabasePath() string {
	return databasePath(o.projectID, o.instanceID, o.databaseID)
}

func (o *emulatorOptions) InstancePath() string {
	return instancePath(o.projectID, o.instanceID)
}

func (o *emulatorOptions) ProjectPath() string {
	return projectPath(o.projectID)
}

type Clients struct {
	InstanceClient *instance.InstanceAdminClient
	DatabaseClient *database.DatabaseAdminClient
	Client         *spanner.Client
}

func applyOptions(options ...Option) (*emulatorOptions, error) {
	opts := &emulatorOptions{
		emulatorImage:     DefaultEmulatorImage,
		projectID:         DefaultProjectID,
		instanceID:        DefaultInstanceID,
		databaseID:        DefaultDatabaseID,
		disableAutoConfig: false,
		databaseDialect:   databasepb.DatabaseDialect_DATABASE_DIALECT_UNSPECIFIED,
	}

	for _, opt := range options {
		if err := opt(opts); err != nil {
			return nil, err
		}
	}

	return opts, nil
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
	container, err = gcloud.RunSpanner(ctx,
		opts.emulatorImage,
		gcloud.WithProjectID(opts.projectID),
		testcontainers.WithLogger(&noopLogger{}),
	)
	if err != nil {
		return nil, nil, err
	}

	teardown = func() {
		err := container.Terminate(ctx)
		if err != nil {
			log.Printf("failed to terminate Cloud Spanner Emulator: %v", err)
		}
	}

	clientOpts := defaultClientOpts(container)

	if !opts.disableAutoConfig {
		if err = bootstrap(ctx, opts, clientOpts...); err != nil {
			teardown()
			return nil, nil, err
		}
	} else if len(opts.setupDDLs) > 0 {
		err = updateDDLs(ctx, opts, clientOpts...)
		if err != nil {
			teardown()
			return nil, nil, err
		}
	}

	if len(opts.setupDMLs) > 0 {
		if err = executeDMLs(ctx, opts, clientOpts...); err != nil {
			teardown()
			return nil, nil, err
		}
	}

	return container, teardown, nil
}

func executeDMLs(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(),
		spanner.ClientConfig{
			SessionPoolConfig: spanner.SessionPoolConfig{MinOpened: 1},
		},
		clientOpts...)
	if err != nil {
		return err
	}

	defer client.Close()

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err = txn.BatchUpdate(ctx, opts.setupDMLs)
		if err != nil {
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to apply DML, err:%w", err)
	}

	return nil
}

func updateDDLs(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	defer func(dbCli *database.DatabaseAdminClient) {
		if err := dbCli.Close(); err != nil {
			log.Println(err)
		}
	}(dbCli)

	op, err := dbCli.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   databasePath(opts.projectID, opts.instanceID, opts.databaseID),
		Statements: opts.setupDDLs,
	})
	if err != nil {
		return err
	}

	if err := op.Wait(ctx); err != nil {
		return err
	}
	return nil
}

func bootstrap(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) (err error) {
	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	defer instanceCli.Close()

	createInstanceOp, err := instanceCli.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     opts.ProjectPath(),
		InstanceId: opts.instanceID,
		Instance: &instancepb.Instance{
			Name:        opts.InstancePath(),
			Config:      "emulator-config",
			DisplayName: opts.instanceID,
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
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%v`", opts.databaseID),
		DatabaseDialect: opts.databaseDialect,
		ExtraStatements: opts.setupDDLs,
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

// NewEmulatorWithClients initializes Cloud Spanner Emulator with Spanner clients.
// The emulator and clients are closed when teardown is called. You should call it.
func NewEmulatorWithClients(ctx context.Context, options ...Option) (emulator *gcloud.GCloudContainer, clients *Clients, teardown func(), err error) {
	opts, err := applyOptions(options...)
	if err != nil {
		return nil, nil, nil, err
	}

	return newClients(ctx, opts)
}

func newClients(ctx context.Context, opts *emulatorOptions) (emulator *gcloud.GCloudContainer, clients *Clients, teardown func(), err error) {
	emulator, emulatorTeardown, err := newEmulator(ctx, opts)
	if err != nil {
		return nil, nil, nil, err
	}

	teardown = emulatorTeardown

	clientOpts := defaultClientOpts(emulator)
	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, nil, err
	}

	instanceCliTeardown := func() {
		if err := instanceCli.Close(); err != nil {
			log.Printf("failed to instanceAdminClient.Close(): %v", err)
		}
		emulatorTeardown()
	}
	teardown = instanceCliTeardown

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, nil, err
	}

	dbCliTeardown := func() {
		if err := dbCli.Close(); err != nil {
			log.Printf("failed to databaseAdminClient.Close(): %v", err)
		}
		instanceCliTeardown()
	}
	teardown = dbCliTeardown

	client, err := spanner.NewClient(ctx, opts.DatabasePath(), clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, nil, err
	}

	clientTeardown := func() {
		client.Close()
		dbCliTeardown()
	}
	teardown = clientTeardown

	return emulator, &Clients{
		InstanceClient: instanceCli,
		DatabaseClient: dbCli,
		Client:         client,
	}, teardown, nil
}

func defaultClientOpts(emulator *gcloud.GCloudContainer) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(emulator.URI),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}
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
