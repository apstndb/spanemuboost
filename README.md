# SPANner EMUlator BOOtSTrapper

[![Go Reference](https://pkg.go.dev/badge/github.com/apstndb/spanemuboost.svg)](https://pkg.go.dev/github.com/apstndb/spanemuboost)

spanemuboost bootstraps Cloud Spanner Emulator and client with no required configuration using [testcontainers-go](https://github.com/testcontainers/testcontainers-go).

It inspired by `autoConfigEmulator` of:

* [java-spanner-jdbc](https://github.com/googleapis/java-spanner-jdbc?tab=readme-ov-file#commonly-used-properties)
* [java-spanner](https://cloud.google.com/java/docs/reference/google-cloud-spanner/6.62.0/com.google.cloud.spanner.connection.ConnectionOptions.Builder#com_google_cloud_spanner_connection_ConnectionOptions_Builder_setUri_java_lang_String_)

This package doesn't have functionality of splitting statements and stripping comments.
Consider to use [memefish](https://github.com/cloudspannerecosystem/memefish) or helper packages.

## Examples

### Simple setup (test)

`spanemuboost.SetupEmulatorWithClients()` handles everything: starts the emulator, creates clients, and auto-cleans up when the test finishes.

```go
func TestFoo(t *testing.T) {
    env := spanemuboost.SetupEmulatorWithClients(t,
        spanemuboost.WithSetupDDLs(ddls),
    )
    // env.Client, env.DatabaseClient, env.InstanceClient available
    // cleanup is automatic via t.Cleanup
}
```

### Simple setup (non-test)

`spanemuboost.RunEmulatorWithClients()` can be used without required configurations.

```go
func ExampleRunEmulatorWithClients() {
    ctx := context.Background()

    env, err := spanemuboost.RunEmulatorWithClients(ctx)
    if err != nil {
        log.Fatalln(err)
        return
    }

    defer env.Close()

    err = env.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(r *spanner.Row) error {
        fmt.Println(r)
        return nil
    })
    if err != nil {
        log.Fatalln(err)
    }
}
```

### Recommended test setup

As recommended by [Cloud Spanner Emulator FAQ](https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/master/README.md#what-is-the-recommended-test-setup):

> What is the recommended test setup?
> Use a single emulator process and create a Cloud Spanner instance within it. Since creating databases is cheap in the emulator, we recommend that each test bring up and tear down its own database. This ensures hermetic testing and allows the test suite to run tests in parallel if needed.

Use `TestMain` to start a single emulator and share it across all test functions in the package. Each test function creates its own database via `SetupClients`.

Note: `testing.M` does NOT implement `testing.TB`, so `Setup*` functions cannot be used in `TestMain` itself. Use `RunEmulator` to start the emulator, then `SetupClients` in each test function for per-test database setup with automatic cleanup.

```go
var emulator *spanemuboost.Emulator

func TestMain(m *testing.M) {
    var err error
    emulator, err = spanemuboost.RunEmulator(context.Background(),
        spanemuboost.EnableInstanceAutoConfigOnly(),
    )
    if err != nil { log.Fatal(err) }
    code := m.Run()
    if err := emulator.Close(); err != nil {
        log.Printf("failed to close emulator: %v", err)
    }
    os.Exit(code)
}

func TestCreate(t *testing.T) {
    clients := spanemuboost.SetupClients(t, emulator,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs(ddls),
    )
    // use clients.Client...
}

func TestRead(t *testing.T) {
    clients := spanemuboost.SetupClients(t, emulator,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs(ddls),
    )
    // use clients.Client...
}
```

### Shared emulator with subtests

When tests are naturally related and don't need `TestMain`, you can share an emulator within subtests of a single parent test. Since each subtest creates its own database via `WithRandomDatabaseID()`, subtests can safely run in parallel with `t.Parallel()`.

```go
func TestSuite(t *testing.T) {
    emu := spanemuboost.SetupEmulator(t, spanemuboost.EnableInstanceAutoConfigOnly())

    t.Run("test1", func(t *testing.T) {
        t.Parallel()
        clients := spanemuboost.SetupClients(t, emu,
            spanemuboost.WithRandomDatabaseID(),
            spanemuboost.WithSetupDDLs(ddls),
        )
        // use clients.Client...
    })

    t.Run("test2", func(t *testing.T) {
        t.Parallel()
        clients := spanemuboost.SetupClients(t, emu,
            spanemuboost.WithRandomDatabaseID(),
            spanemuboost.WithSetupDDLs(otherDDLs),
        )
        // use clients.Client...
    })
}
```

### Using `SPANNER_EMULATOR_HOST` environment variable

For serial tests with code that reads `SPANNER_EMULATOR_HOST` directly instead of accepting a client, you can use `t.Setenv` to set the environment variable:

```go
func TestWithEnvVar(t *testing.T) {
    emu := spanemuboost.SetupEmulator(t, spanemuboost.EnableInstanceAutoConfigOnly())
    t.Setenv("SPANNER_EMULATOR_HOST", emu.URI())
    // Code under test that reads SPANNER_EMULATOR_HOST directly
}
```

**Caveats:**
- `t.Setenv` cannot be used with `t.Parallel()` or tests with parallel ancestors (it panics).
- The environment variable is process-global and doesn't scale to concurrent tests.
- Prefer passing `emu.ClientOptions()` or clients directly when possible.

### Multiple databases (non-test)

```go
func ExampleOpenClients() {
    ctx := context.Background()

    emu, err := spanemuboost.RunEmulator(ctx,
        spanemuboost.EnableInstanceAutoConfigOnly(),
    )
    if err != nil {
        log.Fatalln(err)
        return
    }

    defer emu.Close()

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

            defer clients.Close()

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
