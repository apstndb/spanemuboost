package spanemuboost_test

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/spanner"

	"github.com/apstndb/spanemuboost"
)

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
		// Output: {fields: [type:{code:INT64}], values: [string_value:"1"]}
		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}
}

func ExampleNewEmulatorAndNewClients() {
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
	for i := 0; i < 10; i++ {
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
