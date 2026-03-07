package spanemuboost

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newEmulator(ctx context.Context, opts *emulatorOptions) (container *tcspanner.Container, teardown func(), err error) {
	containerCustomizers := []testcontainers.ContainerCustomizer{
		tcspanner.WithProjectID(opts.projectID),
		testcontainers.WithConfigModifier(func(config *dcontainer.Config) {
			config.Cmd = []string{"./gateway_main", "--hostname", "0.0.0.0"}
		}),
	}
	containerCustomizers = append(containerCustomizers, opts.containerCustomizers...)

	container, err = tcspanner.Run(ctx,
		opts.emulatorImage,
		containerCustomizers...,
	)
	if err != nil {
		return nil, nil, err
	}

	teardown = func() {
		if err := container.Terminate(context.WithoutCancel(ctx)); err != nil {
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
		return fmt.Errorf("failed to apply DML: %w", err)
	}

	return nil
}

func executeDMLsWithClient(ctx context.Context, opts *emulatorOptions, client *spanner.Client) error {
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err := txn.BatchUpdate(ctx, opts.setupDMLs)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to apply DML: %w", err)
	}
	return nil
}

func updateDDLs(ctx context.Context, opts *emulatorOptions, dbCli *database.DatabaseAdminClient) error {
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

// bootstrapInstance creates the instance if auto-config is enabled.
func bootstrapInstance(ctx context.Context, opts *emulatorOptions, instanceCli *instance.InstanceAdminClient) error {
	if opts.disableCreateInstance {
		return nil
	}
	return createInstance(ctx, opts, instanceCli)
}

// bootstrapDatabase creates the database (with DDLs) or applies DDLs to an existing database.
func bootstrapDatabase(ctx context.Context, opts *emulatorOptions, dbCli *database.DatabaseAdminClient) error {
	if !opts.disableCreateDatabase {
		return createDatabase(ctx, opts, dbCli)
	}
	if len(opts.setupDDLs) > 0 {
		return updateDDLs(ctx, opts, dbCli)
	}
	return nil
}

func bootstrap(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	if !opts.disableCreateInstance {
		instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
		if err != nil {
			return err
		}
		defer func() {
			if err := instanceCli.Close(); err != nil {
				log.Printf("failed to close instance admin client: %v", err)
			}
		}()

		if err := createInstance(ctx, opts, instanceCli); err != nil {
			return err
		}
	}

	if !opts.disableCreateDatabase || len(opts.setupDDLs) > 0 {
		dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
		if err != nil {
			return err
		}
		defer func() {
			if err := dbCli.Close(); err != nil {
				log.Printf("failed to close database admin client: %v", err)
			}
		}()

		if err := bootstrapDatabase(ctx, opts, dbCli); err != nil {
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

func createDatabase(ctx context.Context, opts *emulatorOptions, dbCli *database.DatabaseAdminClient) error {
	var createStmt string
	if opts.databaseDialect != databasepb.DatabaseDialect_POSTGRESQL {
		createStmt = fmt.Sprintf("CREATE DATABASE `%v`", opts.databaseID)
	} else {
		createStmt = fmt.Sprintf(`CREATE DATABASE "%s"`, strings.ReplaceAll(opts.databaseID, `"`, `""`))
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

func createInstance(ctx context.Context, opts *emulatorOptions, instanceCli *instance.InstanceAdminClient) error {
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

// bootstrapAndCreateClients creates admin clients, runs bootstrap operations
// (instance/database creation, DDL, DML) using them, creates the data client,
// and returns all clients as [Clients]. This avoids creating admin clients twice
// (once for bootstrap, once for the caller).
func bootstrapAndCreateClients(ctx context.Context, emu *Emulator, opts *emulatorOptions) (_ *Clients, retErr error) {
	clientOpts := emu.ClientOptions()

	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := instanceCli.Close(); err != nil {
				log.Printf("failed to close instance admin client: %v", err)
			}
		}
	}()

	if err := bootstrapInstance(ctx, opts, instanceCli); err != nil {
		return nil, err
	}

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := dbCli.Close(); err != nil {
				log.Printf("failed to close database admin client: %v", err)
			}
		}
	}()

	if err := bootstrapDatabase(ctx, opts, dbCli); err != nil {
		return nil, err
	}

	// Create the data client before DML execution so that the same client
	// can be reused for both bootstrap DMLs and user operations.
	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(), opts.clientConfig, slices.Concat(clientOpts, opts.clientOptionsForClient)...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			client.Close()
		}
	}()

	if len(opts.setupDMLs) > 0 {
		if err := executeDMLsWithClient(ctx, opts, client); err != nil {
			return nil, err
		}
	}

	return &Clients{
		InstanceClient: instanceCli,
		DatabaseClient: dbCli,
		Client:         client,
		ProjectID:      opts.projectID,
		InstanceID:     opts.instanceID,
		DatabaseID:     opts.databaseID,
		dropDatabase:   opts.strictTeardown && !opts.disableCreateDatabase,
		dropInstance:   opts.strictTeardown && !opts.disableCreateInstance,
	}, nil
}

func newClients(ctx context.Context, emulator *tcspanner.Container, opts *emulatorOptions) (clients *Clients, teardown func(), err error) {
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

	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(), opts.clientConfig, slices.Concat(clientOpts, opts.clientOptionsForClient)...)
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

// defaultClientOpts returns client options for connecting to the emulator.
// It is the shared implementation for [Emulator.ClientOptions] and the deprecated
// newClients path. Once the deprecated path is removed, this function should be
// inlined into [Emulator.ClientOptions].
func defaultClientOpts(emulator *tcspanner.Container) []option.ClientOption {
	return []option.ClientOption{
		// passthrough:/// tells gRPC to use the address as-is without DNS resolution.
		option.WithEndpoint("passthrough:///" + emulator.URI()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
		// SkipDialSettingsValidation is required because the passthrough:/// prefix
		// fails the default endpoint validation. This is an internal option also used
		// by the Spanner, Bigtable, and Datastore client libraries for emulator paths.
		internaloption.SkipDialSettingsValidation(),
	}
}
