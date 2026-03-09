# SPANner EMUlator BOOtSTrapper

[![Go Reference](https://pkg.go.dev/badge/github.com/apstndb/spanemuboost.svg)](https://pkg.go.dev/github.com/apstndb/spanemuboost)

spanemuboost bootstraps Cloud Spanner Emulator and client with no required configuration using [testcontainers-go](https://github.com/testcontainers/testcontainers-go).

It inspired by `autoConfigEmulator` of:

* [java-spanner-jdbc](https://github.com/googleapis/java-spanner-jdbc?tab=readme-ov-file#commonly-used-properties)
* [java-spanner](https://cloud.google.com/java/docs/reference/google-cloud-spanner/6.62.0/com.google.cloud.spanner.connection.ConnectionOptions.Builder#com_google_cloud_spanner_connection_ConnectionOptions_Builder_setUri_java_lang_String_)

This package doesn't have functionality of splitting statements and stripping comments.
Consider to use [memefish](https://github.com/cloudspannerecosystem/memefish) or helper packages.

## Examples

### Quick start

```go
func TestFoo(t *testing.T) {
    env := spanemuboost.SetupEmulatorWithClients(t,
        spanemuboost.WithSetupDDLs(ddls),
    )
    // env.Client, env.DatabaseClient, env.InstanceClient available
    // cleanup is automatic via t.Cleanup
}
```

For non-test usage (e.g. embedding the emulator in an application where the `testing` package is unavailable), see runnable examples on [pkg.go.dev](https://pkg.go.dev/github.com/apstndb/spanemuboost#pkg-examples).

### Shared emulator patterns

As [recommended by the Cloud Spanner Emulator FAQ](https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/master/README.md#what-is-the-recommended-test-setup):

> What is the recommended test setup?
> Use a single emulator process and create a Cloud Spanner instance within it. Since creating databases is cheap in the emulator, we recommend that each test bring up and tear down its own database. This ensures hermetic testing and allows the test suite to run tests in parallel if needed.

| Pattern | Emulator lifetime | Best for |
|---|---|---|
| **Lazy** (`NewLazyEmulator` + `SetupClients`) | First `SetupClients` call &rarr; `TestMain` cleanup | Packages mixing emulator and non-emulator tests; skips startup when unused |
| **Eager** (`RunEmulator` + `SetupClients`) | `TestMain` start &rarr; `TestMain` cleanup | All tests need the emulator; fail fast on startup errors |
| **Subtests** (`SetupEmulator` + `SetupClients`) | Parent test &rarr; `t.Cleanup` | Related tests grouped under one function; supports `t.Parallel()` in subtests |

#### Lazy shared emulator (recommended)

The emulator starts only when the first test calls `SetupClients` with the `LazyEmulator`. If `go test -run TestUnit` matches only tests that never use it, the container is never started.

```go
var lazyEmu = spanemuboost.NewLazyEmulator(spanemuboost.EnableInstanceAutoConfigOnly())

func TestMain(m *testing.M) {
    code := m.Run()
    if err := lazyEmu.Close(); err != nil { // no-op if no test used the emulator
        log.Printf("failed to close emulator: %v", err)
    }
    os.Exit(code)
}

func TestCreate(t *testing.T) {
    clients := spanemuboost.SetupClients(t, lazyEmu,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs(ddls),
    )
    // use clients.Client...
}

func TestUnit(t *testing.T) {
    // Does NOT use lazyEmu — emulator never starts
}
```

#### Eager shared emulator

`testing.M` does NOT implement `testing.TB`, so use `RunEmulator` directly in `TestMain`.

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
```

#### Shared emulator with subtests

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
}
```

### `SPANNER_EMULATOR_HOST` environment variable

For serial tests with code that reads `SPANNER_EMULATOR_HOST` directly:

```go
func TestWithEnvVar(t *testing.T) {
    emu := spanemuboost.SetupEmulator(t, spanemuboost.EnableInstanceAutoConfigOnly())
    t.Setenv("SPANNER_EMULATOR_HOST", emu.URI())
    // Code under test that reads SPANNER_EMULATOR_HOST directly
}
```

| Caveat | Detail |
|---|---|
| No `t.Parallel()` | `t.Setenv` panics if the test or an ancestor called `t.Parallel()` |
| Process-global | The env var doesn't scale to concurrent tests |
| Prefer `ClientOptions()` | Pass `emu.ClientOptions()` or clients directly when possible |
