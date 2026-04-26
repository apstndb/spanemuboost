package spanemuboost

import (
	"cmp"
	"fmt"
	"math/rand/v2"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/api/option"
)

type emulatorOptions struct {
	emulatorImage                     string
	projectID, instanceID, databaseID string

	randomProjectID, randomInstanceID, randomDatabaseID bool

	disableCreateInstance    bool
	disableCreateDatabase    bool
	disableBackendGuardrails bool
	reuseExistingDatabase    bool
	schemaTeardown           *bool

	databaseDialect        databasepb.DatabaseDialect
	setupDDLs              []string
	setupDMLs              []spanner.Statement
	clientConfig           *spanner.ClientConfig // nil until finalizeOptions; guaranteed non-nil after
	containerCustomizers   []testcontainers.ContainerCustomizer
	clientOptionsForClient []option.ClientOption
}

// Option configures spanemuboost runtime bootstrap behavior.
// Use the package-provided With*, Without*, Enable*, Disable*, Force*, and
// Skip* helpers; external Option implementations are not supported.
type Option func(*emulatorOptions) error

// WithContainerCustomizers sets any testcontainers.ContainerCustomizer
func WithContainerCustomizers(containerCustomizers ...testcontainers.ContainerCustomizer) Option {
	return func(opts *emulatorOptions) error {
		opts.containerCustomizers = append(opts.containerCustomizers, containerCustomizers...)
		return nil
	}
}

// EnableFaultInjection enables fault injection of Cloud Spanner Emulator.
func EnableFaultInjection() Option {
	return func(opts *emulatorOptions) error {
		opts.containerCustomizers = append(opts.containerCustomizers, testcontainers.WithConfigModifier(func(config *container.Config) {
			config.Cmd = append(config.Cmd, "--enable_fault_injection")
		}))
		return nil
	}
}

// WithClientOptionsForClient configures ClientOption for Clients.Client.
func WithClientOptionsForClient(option ...option.ClientOption) Option {
	return func(opts *emulatorOptions) error {
		opts.clientOptionsForClient = append(opts.clientOptionsForClient, option...)
		return nil
	}
}

// WithProjectID configures the project ID.
// Empty string resets to default.
func WithProjectID(projectID string) Option {
	return func(opts *emulatorOptions) error {
		opts.projectID = projectID
		return nil
	}
}

// WithRandomProjectID enables the random project ID. Default is disabled.
// This clears any previously set project ID (including inherited values from [OpenClients]).
func WithRandomProjectID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomProjectID = true
		opts.projectID = ""
		return nil
	}
}

// WithoutRandomProjectID disables the random project ID. Default is disabled.
func WithoutRandomProjectID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomProjectID = false
		return nil
	}
}

// WithInstanceID configures the instance ID.
// Empty string resets to default.
func WithInstanceID(instanceID string) Option {
	return func(opts *emulatorOptions) error {
		opts.instanceID = instanceID
		return nil
	}
}

// WithRandomInstanceID enables the random instance ID. Default is disabled.
// This clears any previously set instance ID (including inherited values from [OpenClients]).
//
// Because a random ID will never match an existing instance, this option also
// enables instance auto-creation (sets disableCreateInstance to false).
// To disable creation again, call [DisableAutoConfig] after this option.
func WithRandomInstanceID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomInstanceID = true
		opts.instanceID = ""
		opts.disableCreateInstance = false
		return nil
	}
}

// WithoutRandomInstanceID disables the random instance ID. Default is disabled.
func WithoutRandomInstanceID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomInstanceID = false
		return nil
	}
}

// WithDatabaseID configures the database ID.
// Empty string resets to default.
func WithDatabaseID(databaseID string) Option {
	return func(opts *emulatorOptions) error {
		currentDatabaseID := cmp.Or(opts.databaseID, DefaultDatabaseID)
		targetDatabaseID := cmp.Or(databaseID, DefaultDatabaseID)
		if opts.reuseExistingDatabase && targetDatabaseID != currentDatabaseID {
			opts.disableCreateDatabase = false
		}
		opts.reuseExistingDatabase = false
		opts.databaseID = databaseID
		return nil
	}
}

// WithRandomDatabaseID enables the random database ID. Default is disabled.
// This clears any previously set database ID (including inherited values from [OpenClients]).
//
// Because a random ID will never match an existing database, this option also
// enables database auto-creation (sets disableCreateDatabase to false).
// To disable creation again, call [DisableAutoConfig] after this option.
func WithRandomDatabaseID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomDatabaseID = true
		opts.reuseExistingDatabase = false
		opts.databaseID = ""
		opts.disableCreateDatabase = false
		return nil
	}
}

// WithoutRandomDatabaseID disables the random database ID. Default is disabled.
func WithoutRandomDatabaseID() Option {
	return func(opts *emulatorOptions) error {
		opts.randomDatabaseID = false
		return nil
	}
}

// WithDatabaseDialect configures the database dialect.
func WithDatabaseDialect(dialect databasepb.DatabaseDialect) Option {
	return func(opts *emulatorOptions) error {
		opts.databaseDialect = dialect
		return nil
	}
}

// WithContainerImage configures the container image used for the selected backend.
// Empty string will be ignored.
func WithContainerImage(image string) Option {
	return func(opts *emulatorOptions) error {
		if image != "" {
			opts.emulatorImage = image
		}
		return nil
	}
}

// Deprecated: WithEmulatorImage is a deprecated alias for [WithContainerImage].
// Empty string will be ignored.
func WithEmulatorImage(image string) Option {
	return WithContainerImage(image)
}

// DisableBackendGuardrails disables backend-specific validation and coercion.
//
// By default, spanemuboost rejects known-invalid backend configurations early
// with human-readable errors. Use this option only when trying a newer backend
// version whose constraints may have changed.
func DisableBackendGuardrails() Option {
	return func(opts *emulatorOptions) error {
		opts.disableBackendGuardrails = true
		return nil
	}
}

// WithClientConfig sets [spanner.ClientConfig] for managed data clients created by
// spanemuboost, including [OpenClients], [RunWithClients], and [SetupWithClients].
//
// When this option is not used, spanemuboost sets DisableNativeMetrics to true
// by default, since the Spanner native metrics infrastructure is unnecessary
// for emulator connections and can add overhead (metadata server lookups,
// monitoring exporter creation).
//
// For Omni managed clients with backend guardrails enabled, spanemuboost applies
// the recommended Omni defaults from [RecommendedOmniClientConfig], including
// DisableNativeMetrics and IsExperimentalHost, overriding those two fields even
// when they were set explicitly in the provided config. DisableBackendGuardrails
// keeps the provided config untouched. [RecommendedOmniClientConfig] remains the
// recommended base for external Go clients.
func WithClientConfig(config spanner.ClientConfig) Option {
	return func(opts *emulatorOptions) error {
		opts.clientConfig = &config
		return nil
	}
}

// WithSetupDDLs sets DDLs to be executed.
// Calling this multiple times replaces the previous value.
func WithSetupDDLs(ddls []string) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDDLs = ddls
		return nil
	}
}

// WithSetupRawDMLs sets string DMLs to be executed.
// Calling this multiple times replaces the previous value.
// This is mutually exclusive with WithSetupDMLs; the last one called wins.
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

// WithSetupDMLs sets DMLs in spanner.Statement to be executed.
// Calling this multiple times replaces the previous value.
// This is mutually exclusive with WithSetupRawDMLs; the last one called wins.
func WithSetupDMLs(dmls []spanner.Statement) Option {
	return func(opts *emulatorOptions) error {
		opts.setupDMLs = dmls
		return nil
	}
}

// DisableAutoConfig disables auto config.(default enable)
func DisableAutoConfig() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = true
		opts.disableCreateDatabase = true
		opts.reuseExistingDatabase = false
		return nil
	}
}

// EnableAutoConfig enables auto config.(default enable)
func EnableAutoConfig() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = false
		opts.disableCreateDatabase = false
		opts.reuseExistingDatabase = false
		return nil
	}
}

// EnableInstanceAutoConfigOnly enables only instance auto-creation and keeps
// database auto-creation disabled.
func EnableInstanceAutoConfigOnly() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = false
		opts.disableCreateDatabase = true
		opts.reuseExistingDatabase = false
		return nil
	}
}

// EnableDatabaseAutoConfigOnly enables only database auto-creation and keeps
// instance auto-creation disabled.
func EnableDatabaseAutoConfigOnly() Option {
	return func(opts *emulatorOptions) error {
		opts.disableCreateInstance = true
		opts.disableCreateDatabase = false
		opts.reuseExistingDatabase = false
		return nil
	}
}

// ForceSchemaTeardown forces schema resource cleanup on [Clients.Close],
// dropping any auto-created database or instance before closing the Go clients.
//
// By default, schema teardown is enabled for fixed IDs and disabled for random IDs.
// This option overrides that default for all resources.
func ForceSchemaTeardown() Option {
	return func(opts *emulatorOptions) error {
		opts.schemaTeardown = ptrOf(true)
		return nil
	}
}

// SkipSchemaTeardown disables schema resource cleanup on [Clients.Close].
// Auto-created databases and instances will not be dropped on close.
//
// By default, schema teardown is enabled for fixed IDs;
// use this option to opt out.
func SkipSchemaTeardown() Option {
	return func(opts *emulatorOptions) error {
		opts.schemaTeardown = ptrOf(false)
		return nil
	}
}

// Deprecated: Use [ForceSchemaTeardown] instead.
func WithStrictTeardown() Option {
	return ForceSchemaTeardown()
}

// shouldDropInstance returns whether the instance should be dropped on Close.
func (o *emulatorOptions) shouldDropInstance() bool {
	return o.shouldDropResource(o.disableCreateInstance, o.randomInstanceID)
}

// shouldDropDatabase returns whether the database should be dropped on Close.
func (o *emulatorOptions) shouldDropDatabase() bool {
	return o.shouldDropResource(o.disableCreateDatabase, o.randomDatabaseID)
}

// shouldDropResource returns whether a resource should be dropped on Close.
// If schemaTeardown is explicitly set, it takes precedence.
// Otherwise, a non-random (fixed) ID implies teardown.
func (o *emulatorOptions) shouldDropResource(disableCreate, isRandomID bool) bool {
	if disableCreate {
		return false
	}
	if o.schemaTeardown != nil {
		return *o.schemaTeardown
	}
	return !isRandomID
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

// applyOptionsWithBase applies options starting from a pre-configured base.
// Used by [OpenClients] to inherit emulator settings.
func applyOptionsWithBase(base *emulatorOptions, options ...Option) (*emulatorOptions, error) {
	opts := *base
	for _, opt := range options {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}
	return finalizeOptions(&opts)
}

func applyOptions(options ...Option) (*emulatorOptions, error) {
	opts := &emulatorOptions{}

	for _, opt := range options {
		if err := opt(opts); err != nil {
			return nil, err
		}
	}

	return finalizeOptions(opts)
}

func finalizeOptions(opts *emulatorOptions) (*emulatorOptions, error) {
	if opts.randomProjectID && opts.projectID != "" {
		return nil, fmt.Errorf("WithRandomProjectID() and WithProjectID() are mutually exclusive")
	}

	if opts.randomInstanceID && opts.instanceID != "" {
		return nil, fmt.Errorf("WithRandomInstanceID() and WithInstanceID() are mutually exclusive")
	}

	if opts.randomDatabaseID && opts.databaseID != "" {
		return nil, fmt.Errorf("WithRandomDatabaseID() and WithDatabaseID() are mutually exclusive")
	}

	if opts.randomProjectID {
		opts.projectID = generateRandomID()
	}

	if opts.randomInstanceID {
		opts.instanceID = generateRandomID()
	}

	if opts.randomDatabaseID {
		opts.databaseID = generateRandomID()
	}

	opts.emulatorImage = cmp.Or(opts.emulatorImage, DefaultEmulatorImage)
	opts.projectID = cmp.Or(opts.projectID, DefaultProjectID)
	opts.instanceID = cmp.Or(opts.instanceID, DefaultInstanceID)
	opts.databaseID = cmp.Or(opts.databaseID, DefaultDatabaseID)

	// Disable native metrics by default for emulator connections.
	// Without SPANNER_EMULATOR_HOST, the Spanner client tries to create a real
	// Cloud Monitoring exporter and contacts the GCP metadata server, adding
	// unnecessary latency and errors. See #9.
	if opts.clientConfig == nil {
		opts.clientConfig = &spanner.ClientConfig{DisableNativeMetrics: true}
	}

	return opts, nil
}

const (
	databaseIDFirstChars = "abcdefghijklmnopqrstuvwxyz"
	databaseIDChars      = databaseIDFirstChars + "0123456789"
	idRange              = 30
)

func ptrOf[T any](v T) *T { return &v }

// generateRandomID generates a random database ID.
// Generated ID will be this format: [a-z][a-z0-9]{29}
func generateRandomID() string {
	b := make([]byte, idRange)
	b[0] = databaseIDFirstChars[rand.N(len(databaseIDFirstChars))]
	for i := range idRange - 1 {
		b[i+1] = databaseIDChars[rand.N(len(databaseIDChars))]
	}
	return string(b)
}
