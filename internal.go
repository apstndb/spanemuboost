package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	dcontainer "github.com/docker/docker/api/types/container"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/testcontainers/testcontainers-go"
	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type createdSchemaResources struct {
	instance bool
	database bool
}

const rollbackTimeout = 30 * time.Second

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
		if err := container.Terminate(context.Background()); err != nil {
			log.Printf("failed to terminate Cloud Spanner Emulator: %v", err)
		}
	}

	return container, teardown, nil
}

func containerPlatform(ctx context.Context, container testcontainers.Container) (string, error) {
	if container == nil {
		return "", errors.New("spanemuboost: container is nil")
	}
	info, err := container.Inspect(ctx)
	if err != nil {
		return "", fmt.Errorf("spanemuboost: inspect container platform: %w", err)
	}
	return inspectContainerPlatform(info)
}

func inspectContainerPlatform(info *dcontainer.InspectResponse) (string, error) {
	if info == nil {
		return "", errors.New("spanemuboost: container inspect response is nil")
	}
	if platform := descriptorPlatformString(info.ImageManifestDescriptor); platform != "" {
		return platform, nil
	}
	if info.ContainerJSONBase != nil && info.Platform != "" {
		return info.Platform, nil
	}
	return "", errors.New("spanemuboost: container platform metadata unavailable")
}

func descriptorPlatformString(desc *ocispec.Descriptor) string {
	if desc == nil || desc.Platform == nil {
		return ""
	}
	return ociPlatformString(desc.Platform)
}

func ociPlatformString(platform *ocispec.Platform) string {
	if platform == nil || platform.OS == "" || platform.Architecture == "" {
		return ""
	}
	if platform.Variant == "" {
		return platform.OS + "/" + platform.Architecture
	}
	return platform.OS + "/" + platform.Architecture + "/" + platform.Variant
}

// executeDMLs creates a short-lived internal client solely for bootstrap DML
// execution in the deprecated path. It uses a minimal config intentionally:
// the user-provided ClientConfig is not applied here because this client is
// discarded immediately after DMLs complete. In the new API path,
// executeDMLsWithClient is used instead with the user-facing client.
func executeDMLs(ctx context.Context, opts *emulatorOptions, clientOpts ...option.ClientOption) error {
	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(),
		minimalBootstrapClientConfig(spanner.ClientConfig{
			// This deprecated bootstrap path creates its own throwaway client and
			// bypasses the managed-client config finalization used elsewhere.
			// Keep native metrics disabled here explicitly rather than assuming
			// minimalBootstrapClientConfig will do it for us.
			DisableNativeMetrics: true,
		}),
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

func minimalBootstrapClientConfig(config spanner.ClientConfig) spanner.ClientConfig {
	cfg := config
	cfg.MinOpened = 1
	cfg.MaxOpened = 1
	return cfg
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
func bootstrapInstance(ctx context.Context, opts *emulatorOptions, instanceCli *instance.InstanceAdminClient) (bool, error) {
	if opts.disableCreateInstance {
		return false, nil
	}
	if err := createInstance(ctx, opts, instanceCli); err != nil {
		return false, err
	}
	return true, nil
}

// bootstrapDatabase creates the database (with DDLs) or applies DDLs to an existing database.
func bootstrapDatabase(ctx context.Context, opts *emulatorOptions, dbCli *database.DatabaseAdminClient) (bool, error) {
	if !opts.disableCreateDatabase {
		if err := createDatabase(ctx, opts, dbCli); err != nil {
			return false, err
		}
		return true, nil
	}
	if len(opts.setupDDLs) > 0 {
		return false, updateDDLs(ctx, opts, dbCli)
	}
	return false, nil
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

		if _, err := bootstrapInstance(ctx, opts, instanceCli); err != nil {
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

		if _, err := bootstrapDatabase(ctx, opts, dbCli); err != nil {
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

func bootstrapWithManagedClientConfig(ctx context.Context, opts *emulatorOptions, clientOpts []option.ClientOption) error {
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

		if _, err := bootstrapInstance(ctx, opts, instanceCli); err != nil {
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

		if _, err := bootstrapDatabase(ctx, opts, dbCli); err != nil {
			return err
		}
	}
	if len(opts.setupDMLs) == 0 {
		return nil
	}

	clientConfig := minimalBootstrapClientConfig(*opts.clientConfig)
	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(), clientConfig, slices.Concat(clientOpts, opts.clientOptionsForClient)...)
	if err != nil {
		return err
	}
	defer client.Close()

	return executeDMLsWithClient(ctx, opts, client)
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
	return bootstrapAndCreateClientsWithOptions(ctx, emu.URI(), opts, emu.ClientOptions())
}

func bootstrapAndCreateClientsWithOptions(ctx context.Context, uri string, opts *emulatorOptions, clientOpts []option.ClientOption) (_ *Clients, retErr error) {
	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}
	var (
		dbCli            *database.DatabaseAdminClient
		client           *spanner.Client
		createdResources createdSchemaResources
	)
	defer func() {
		if retErr != nil {
			if client != nil {
				client.Close()
			}
			if err := rollbackCreatedResources(instanceCli, dbCli, opts, createdResources); err != nil {
				retErr = errors.Join(retErr, err)
			}
			if dbCli != nil {
				logCloseError("close database admin client", dbCli.Close())
			}
			logCloseError("close instance admin client", instanceCli.Close())
		}
	}()

	createdResources.instance, err = bootstrapInstance(ctx, opts, instanceCli)
	if err != nil {
		return nil, err
	}

	dbCli, err = database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}

	createdResources.database, err = bootstrapDatabase(ctx, opts, dbCli)
	if err != nil {
		return nil, err
	}

	client, err = spanner.NewClientWithConfig(ctx, opts.DatabasePath(), *opts.clientConfig, slices.Concat(clientOpts, opts.clientOptionsForClient)...)
	if err != nil {
		return nil, err
	}

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
		clientOpts:     clientOpts,
		uri:            uri,
		dropDatabase:   opts.shouldDropDatabase(),
		dropInstance:   opts.shouldDropInstance(),
	}, nil
}

func rollbackCreatedResources(instanceCli *instance.InstanceAdminClient, dbCli *database.DatabaseAdminClient, opts *emulatorOptions, resources createdSchemaResources) error {
	// Rollback must still run if the setup context timed out or was canceled, but
	// it should remain bounded so OpenClients failure handling cannot hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
	defer cancel()
	var errs []error

	if resources.instance {
		// DeleteInstance removes child databases on success, so only fall back to
		// DropDatabase when instance cleanup could not run or when a follow-up
		// GetInstance confirms the instance still exists after the delete error.
		dropDatabase := false
		if instanceCli == nil {
			errs = append(errs, fmt.Errorf("rollback delete instance %s: instance admin client is nil", opts.InstancePath()))
			dropDatabase = resources.database
		} else {
			err := instanceCli.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{Name: opts.InstancePath()})
			if err != nil {
				errs = append(errs, fmt.Errorf("rollback delete instance %s: %w", opts.InstancePath(), err))
				if resources.database {
					exists, existsErr := instanceStillExists(ctx, instanceCli, opts.InstancePath())
					if existsErr != nil {
						errs = append(errs, fmt.Errorf("rollback get instance %s: %w", opts.InstancePath(), existsErr))
					} else if exists {
						dropDatabase = true
					}
				}
			}
		}
		if dropDatabase {
			if dbCli == nil {
				errs = append(errs, fmt.Errorf("rollback drop database %s: database admin client is nil", opts.DatabasePath()))
			} else {
				err := dbCli.DropDatabase(ctx, &databasepb.DropDatabaseRequest{Database: opts.DatabasePath()})
				if err != nil {
					errs = append(errs, fmt.Errorf("rollback drop database %s: %w", opts.DatabasePath(), err))
				}
			}
		}
		return errors.Join(errs...)
	}

	if resources.database {
		if dbCli == nil {
			errs = append(errs, fmt.Errorf("rollback drop database %s: database admin client is nil", opts.DatabasePath()))
			return errors.Join(errs...)
		}
		err := dbCli.DropDatabase(ctx, &databasepb.DropDatabaseRequest{Database: opts.DatabasePath()})
		if err != nil {
			errs = append(errs, fmt.Errorf("rollback drop database %s: %w", opts.DatabasePath(), err))
		}
	}
	return errors.Join(errs...)
}

func instanceStillExists(ctx context.Context, instanceCli *instance.InstanceAdminClient, instancePath string) (bool, error) {
	_, err := instanceCli.GetInstance(ctx, &instancepb.GetInstanceRequest{Name: instancePath})
	if err == nil {
		return true, nil
	}
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}

func logCloseError(action string, err error) {
	if err != nil {
		log.Printf("spanemuboost: failed to %s: %v", action, err)
	}
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

	client, err := spanner.NewClientWithConfig(ctx, opts.DatabasePath(), *opts.clientConfig, slices.Concat(clientOpts, opts.clientOptionsForClient)...)
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
		clientOpts:     clientOpts,
		uri:            emulator.URI(),
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
