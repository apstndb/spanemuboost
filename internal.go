package spanemuboost

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/admin/instance/apiv1"
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
		if err := container.Terminate(ctx); err != nil {
			log.Printf("failed to terminate Cloud Spanner Emulator: %v", err)
		}
	}

	return container, teardown, nil
}

func executeDMLs(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(),
		spanner.ClientConfig{
			SessionPoolConfig: spanner.SessionPoolConfig{MinOpened: 1, MaxOpened: 1},
		},
		clientOpts...)
	if err != nil {
		return err
	}

	defer client.Close()

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err = txn.BatchUpdate(ctx, opts.setupDMLs)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to apply DML:%w", err)
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
		return fmt.Errorf("failed to apply DDL: %w", err)
	}
	return nil
}

func bootstrap(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	if !opts.disableCreateInstance {
		err := createInstance(ctx, opts, clientOpts)
		if err != nil {
			return err
		}
	}

	if !opts.disableCreateDatabase {
		if err := createDatabase(ctx, opts, clientOpts); err != nil {
			return err
		}
	} else if len(opts.setupDDLs) > 0 {
		if err := updateDDLs(ctx, opts, clientOpts...); err != nil {
			return err
		}
	}

	if len(opts.setupDMLs) > 0 {
		if err := executeDMLs(ctx, opts, clientOpts...); err != nil {
			return err
		}
	}

	return nil
}

func createDatabase(ctx context.Context, opts *emulatorOptions, clientOpts []option.ClientOption) error {
	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	defer dbCli.Close()

	var createStmt string
	if opts.databaseDialect != databasepb.DatabaseDialect_POSTGRESQL {
		createStmt = fmt.Sprintf("CREATE DATABASE `%v`", opts.databaseID)
	} else {
		createStmt = fmt.Sprintf("CREATE DATABASE %q", opts.databaseID)
	}
	createDBOp, err := dbCli.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          opts.InstancePath(),
		CreateStatement: createStmt,
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

func createInstance(ctx context.Context, opts *emulatorOptions, clientOpts []option.ClientOption) error {
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
	return nil
}

func newClients(ctx context.Context, emulator *gcloud.GCloudContainer, opts *emulatorOptions) (clients *Clients, teardown func(), err error) {
	clientOpts := defaultClientOpts(emulator)
	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return nil, nil, err
	}

	instanceCliTeardown := func() {
		if err := instanceCli.Close(); err != nil {
			log.Printf("failed to instanceAdminClient.Close(): %v", err)
		}
	}
	teardown = instanceCliTeardown

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, err
	}

	dbCliTeardown := func() {
		if err := dbCli.Close(); err != nil {
			log.Printf("failed to databaseAdminClient.Close(): %v", err)
		}
		instanceCliTeardown()
	}
	teardown = dbCliTeardown

	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(), opts.clientConfig, clientOpts...)
	if err != nil {
		teardown()
		return nil, nil, err
	}

	teardown = func() {
		client.Close()
		dbCliTeardown()
	}

	return &Clients{
		InstanceClient: instanceCli,
		DatabaseClient: dbCli,
		Client:         client,
		ProjectID:      opts.projectID,
		InstanceID:     opts.instanceID,
		DatabaseID:     opts.databaseID,
	}, teardown, nil
}

func defaultClientOpts(emulator *gcloud.GCloudContainer) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(emulator.URI),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}
}
