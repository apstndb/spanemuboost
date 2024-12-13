# SPANner EMUlator BOOtSTrapper

[![Go Reference](https://pkg.go.dev/badge/github.com/apstndb/spanemuboost.svg)](https://pkg.go.dev/github.com/apstndb/spanemuboost)

spanemuboost bootstraps Cloud Spanner Emulator and client with no required configuration using [testcontainers-go](https://github.com/testcontainers/testcontainers-go).

It inspired by `autoConfigEmulator` of:

* [java-spanner-jdbc](https://github.com/googleapis/java-spanner-jdbc?tab=readme-ov-file#commonly-used-properties)
* [java-spanner](https://cloud.google.com/java/docs/reference/google-cloud-spanner/6.62.0/com.google.cloud.spanner.connection.ConnectionOptions.Builder#com_google_cloud_spanner_connection_ConnectionOptions_Builder_setUri_java_lang_String_)

This package doesn't have functionality of splitting statements and stripping comments.
Consider to use [memefish](https://github.com/cloudspannerecosystem/memefish) or helper packages.

## Examples

### Simple setup

[`spanemuboost.NewEmulatorWithclients()`](https://pkg.go.dev/github.com/apstndb/spanemuboost#NewEmulatorWithClients) can be used without required configurations.

```go
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
```

### Multiple databases setup

spanemuboost supports more practical setup as recommended by [Cloud Spanner Emulator FAQ](https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/master/README.md#what-is-the-recommended-test-setup)

> What is the recommended test setup?
> Use a single emulator process and create a Cloud Spanner instance within it. Since creating databases is cheap in the emulator, we recommend that each test bring up and tear down its own database. This ensures hermetic testing and allows the test suite to run tests in parallel if needed.

```go
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
```