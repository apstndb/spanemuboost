package spanemuboost_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"

	"github.com/apstndb/spanemuboost"
)

func ExampleRunEmulatorWithClients() {
	ctx := context.Background()

	env, err := spanemuboost.RunEmulatorWithClients(ctx)
	if err != nil {
		log.Fatalln(err)
		return
	}

	defer env.Close() //nolint:errcheck

	err = env.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(r *spanner.Row) error {
		fmt.Println(r)
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}
	// Output: {fields: [type:{code:INT64}], values: [string_value:"1"]}
}

func ExampleOpenClients() {
	ctx := context.Background()

	emu, err := spanemuboost.RunEmulator(ctx,
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	if err != nil {
		log.Fatalln(err)
		return
	}

	defer emu.Close() //nolint:errcheck

	var pks []int64
	for i := range 10 {
		func() {
			clients, err := spanemuboost.OpenClients(ctx, emu,
				spanemuboost.WithRandomDatabaseID(),
				spanemuboost.WithSetupDDLs([]string{"CREATE TABLE tbl (PK INT64 PRIMARY KEY)"}),
				spanemuboost.WithSetupDMLs([]spanner.Statement{
					{SQL: "INSERT INTO tbl(PK) VALUES(@i)", Params: map[string]any{"i": i}},
				}),
			)
			if err != nil {
				log.Fatalln(err)
				return
			}

			defer clients.Close() //nolint:errcheck

			err = clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT PK FROM tbl")).Do(func(r *spanner.Row) error {
				var pk int64
				if err := r.ColumnByName("PK", &pk); err != nil {
					return err
				}
				pks = append(pks, pk)
				return nil
			})
			if err != nil {
				log.Fatalln(err)
			}
		}()
	}

	fmt.Println(pks)
	// Output: [0 1 2 3 4 5 6 7 8 9]
}

func ExampleLazyEmulator() {
	ctx := context.Background()

	lazy := spanemuboost.NewLazyEmulator(
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	defer func() {
		if err := lazy.Close(); err != nil {
			log.Printf("failed to close lazy emulator: %v", err)
		}
	}()

	// OpenClients accepts a *LazyEmulator directly and starts it on first use.
	clients, err := spanemuboost.OpenClients(ctx, lazy,
		spanemuboost.WithRandomDatabaseID(),
	)
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer func() {
		if err := clients.Close(); err != nil {
			log.Printf("failed to close clients: %v", err)
		}
	}()

	err = clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(r *spanner.Row) error {
		fmt.Println(r)
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}
	// Output: {fields: [type:{code:INT64}], values: [string_value:"1"]}
}

func ExampleLazyRuntime() {
	ctx := context.Background()

	lazy := spanemuboost.NewLazyRuntime(
		spanemuboost.BackendEmulator,
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	defer func() {
		if err := lazy.Close(); err != nil {
			log.Printf("failed to close lazy runtime: %v", err)
		}
	}()

	// OpenClients accepts a *LazyRuntime directly and starts it on first use.
	clients, err := spanemuboost.OpenClients(ctx, lazy,
		spanemuboost.WithRandomDatabaseID(),
	)
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer func() {
		if err := clients.Close(); err != nil {
			log.Printf("failed to close clients: %v", err)
		}
	}()

	err = clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(r *spanner.Row) error {
		fmt.Println(r)
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}
	// Output: {fields: [type:{code:INT64}], values: [string_value:"1"]}
}

func ExampleNewLazyRuntime_validationCases() {
	ctx := context.Background()
	// LazyRuntime shares one backend container across all cases. The same
	// database-per-case loop works with BackendOmni; omit
	// EnableInstanceAutoConfigOnly there because Omni uses its built-in instance.
	lazy := spanemuboost.NewLazyRuntime(
		spanemuboost.BackendEmulator,
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	defer func() {
		if err := lazy.Close(); err != nil {
			log.Printf("failed to close runtime: %v", err)
		}
	}()

	baseDDLs := []string{
		"CREATE TABLE Singers (SingerId INT64, Name STRING(MAX)) PRIMARY KEY (SingerId)",
	}
	baseDMLs := []string{
		"INSERT INTO Singers (SingerId, Name) VALUES (1, 'Marc Richards')",
	}
	cases := []struct {
		name      string
		setupDDLs []string
		ddl       string
		dml       spanner.Statement
		query     string
	}{
		{
			name: "ddl",
			ddl:  "CREATE TABLE Albums (SingerId INT64, AlbumId INT64) PRIMARY KEY (SingerId, AlbumId)",
		},
		{
			name: "dml",
			setupDDLs: []string{
				"CREATE TABLE Albums (SingerId INT64, AlbumId INT64, Title STRING(MAX)) PRIMARY KEY (SingerId, AlbumId)",
			},
			dml: spanner.NewStatement("INSERT INTO Albums (SingerId, AlbumId, Title) VALUES (1, 1, 'Total Junk')"),
		},
		{
			name:  "query",
			query: "SELECT Name FROM Singers WHERE SingerId = 1",
		},
	}

	for _, tc := range cases {
		// Setup options replace previous values, so compose shared setup and
		// case-specific setup before calling WithSetupDDLs.
		setupDDLs := append([]string{}, baseDDLs...)
		setupDDLs = append(setupDDLs, tc.setupDDLs...)

		// WithRandomDatabaseID isolates cases without starting another backend
		// container. For Omni, random project and instance IDs are not the
		// normal isolation mechanism.
		clients, err := spanemuboost.OpenClients(ctx, lazy,
			spanemuboost.WithRandomDatabaseID(),
			spanemuboost.WithSetupDDLs(setupDDLs),
			spanemuboost.WithSetupRawDMLs(baseDMLs),
		)
		if err != nil {
			log.Printf("%s: SETUP_INVALID: %v", tc.name, err)
			continue
		}

		func() {
			defer func() {
				if err := clients.Close(); err != nil {
					log.Printf("%s: close clients: %v", tc.name, err)
				}
			}()

			var err error
			switch {
			case tc.ddl != "":
				// Candidate DDL is executed after setup so setup failures and
				// candidate failures can be reported separately.
				op, opErr := clients.DatabaseClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
					Database:   clients.DatabasePath(),
					Statements: []string{tc.ddl},
				})
				if opErr != nil {
					err = opErr
				} else {
					err = op.Wait(ctx)
				}
			case tc.dml.SQL != "":
				_, err = clients.Client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
					_, err := txn.Update(ctx, tc.dml)
					return err
				})
			case tc.query != "":
				iter := clients.Client.Single().Query(ctx, spanner.NewStatement(tc.query))
				err = iter.Do(func(*spanner.Row) error { return nil })
			}
			if err != nil {
				log.Printf("%s: INVALID: %v", tc.name, err)
				return
			}
			log.Printf("%s: VALID", tc.name)
		}()
	}
}

func ExampleRecommendedOmniClientConfig_externalClient() {
	if os.Getenv("SPANEMUBOOST_ENABLE_OMNI_TESTS") != "1" {
		return
	}

	ctx := context.Background()
	runtime, err := spanemuboost.Run(ctx, spanemuboost.BackendOmni,
		spanemuboost.WithRandomDatabaseID(),
	)
	if err != nil {
		log.Printf("failed to start Omni runtime: %v", err)
		return
	}
	defer runtime.Close() //nolint:errcheck

	client, err := spanner.NewClientWithConfig(
		ctx,
		runtime.DatabasePath(),
		spanemuboost.RecommendedOmniClientConfig(),
		runtime.ClientOptions()...,
	)
	if err != nil {
		log.Printf("failed to create Omni client: %v", err)
		return
	}
	defer client.Close()

	_ = client
}

// Deprecated examples kept for backward compatibility testing.

func ExampleNewEmulatorWithClients() {
	ctx := context.Background()

	_, clients, teardown, err := spanemuboost.NewEmulatorWithClients(ctx)
	if err != nil {
		log.Fatalln(err)
		return
	}

	defer teardown()

	err = clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(r *spanner.Row) error {
		fmt.Println(r)
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}
	// Output: {fields: [type:{code:INT64}], values: [string_value:"1"]}
}

func ExampleNewClients() {
	ctx := context.Background()

	emulator, emulatorTeardown, err := spanemuboost.NewEmulator(ctx,
		spanemuboost.EnableInstanceAutoConfigOnly(),
	)
	if err != nil {
		log.Fatalln(err)
		return
	}

	defer emulatorTeardown()

	var pks []int64
	for i := range 10 {
		func() {
			clients, clientsTeardown, err := spanemuboost.NewClients(ctx, emulator,
				spanemuboost.EnableDatabaseAutoConfigOnly(),
				spanemuboost.WithRandomDatabaseID(),
				spanemuboost.WithSetupDDLs([]string{"CREATE TABLE tbl (PK INT64 PRIMARY KEY)"}),
				spanemuboost.WithSetupDMLs([]spanner.Statement{
					{SQL: "INSERT INTO tbl(PK) VALUES(@i)", Params: map[string]any{"i": i}},
				}),
			)
			if err != nil {
				log.Fatalln(err)
				return
			}

			defer clientsTeardown()

			err = clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT PK FROM tbl")).Do(func(r *spanner.Row) error {
				var pk int64
				if err := r.ColumnByName("PK", &pk); err != nil {
					return err
				}
				pks = append(pks, pk)
				return nil
			})
			if err != nil {
				log.Fatalln(err)
			}
		}()
	}

	fmt.Println(pks)
	// Output: [0 1 2 3 4 5 6 7 8 9]
}
