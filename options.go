package spanemuboost

import (
	"bytes"
	"cmp"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
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
	setupFileDescriptorSet []byte
	setupDMLs              []spanner.Statement
	clientConfig           *spanner.ClientConfig // nil until finalizeOptions; guaranteed non-nil after
	containerCustomizers   []testcontainers.ContainerCustomizer
	containerProviderSet   bool
	clientOptionsForClient []option.ClientOption

	// gatewayFlags accumulates extra arguments appended to the emulator
	// gateway_main command line. They are emulator-specific; finalizeOmniOptions
	// rejects them unless backend guardrails are disabled.
	gatewayFlags []string
}

// Option configures spanemuboost runtime bootstrap behavior.
// Use the package-provided With*, Without*, Enable*, Disable*, Force*, and
// Skip* helpers; external Option implementations are not supported.
type Option func(*emulatorOptions) error

const testcontainersProviderEnv = "SPANEMUBOOST_TESTCONTAINERS_PROVIDER"

// WithContainerCustomizers adds low-level testcontainers customizers to backend
// runtime containers.
//
// Prefer [WithContainerProvider] instead of passing testcontainers.WithProvider
// directly when selecting Docker or Podman. If a customizer does set the
// provider, it is applied after SPANEMUBOOST_TESTCONTAINERS_PROVIDER and can
// override that environment default.
func WithContainerCustomizers(containerCustomizers ...testcontainers.ContainerCustomizer) Option {
	return func(opts *emulatorOptions) error {
		opts.containerCustomizers = append(opts.containerCustomizers, containerCustomizers...)
		return nil
	}
}

// WithContainerProvider configures the testcontainers provider used to start
// backend runtime containers.
//
// Use [testcontainers.ProviderPodman] when running against Podman and
// Testcontainers-Go cannot auto-detect Podman from DOCKER_HOST. This is common
// with macOS Podman machine forwarded sockets whose host path does not contain
// "podman.sock".
//
// This option overrides SPANEMUBOOST_TESTCONTAINERS_PROVIDER. If several
// provider customizers are supplied explicitly, the last one applied to the
// Testcontainers request wins.
func WithContainerProvider(provider testcontainers.ProviderType) Option {
	return func(opts *emulatorOptions) error {
		if err := validateContainerProvider(provider, "WithContainerProvider"); err != nil {
			return err
		}
		opts.containerProviderSet = true
		opts.containerCustomizers = append(opts.containerCustomizers, testcontainers.WithProvider(provider))
		return nil
	}
}

// EnableFaultInjection enables fault injection of Cloud Spanner Emulator
// (the emulator's --enable_fault_injection flag). Emulator-only.
func EnableFaultInjection() Option {
	return appendGatewayFlag("--enable_fault_injection")
}

// EnableLogRequests enables gRPC request and response logging in the emulator
// gateway (the emulator's --log_requests flag). Useful when debugging test
// failures; output is written to the container's stdout. Emulator-only.
func EnableLogRequests() Option {
	return appendGatewayFlag("--log_requests")
}

// EnableEmulatorStdoutCopy enables copying the emulator backend's stdout to
// the gateway's stdout (the emulator's --copy_emulator_stdout flag). The
// gateway already copies the backend's stderr by default; this option adds
// the matching stdout stream for debugging. Emulator-only.
func EnableEmulatorStdoutCopy() Option {
	return appendGatewayFlag("--copy_emulator_stdout")
}

// DisableQueryNullFilteredIndexCheck disables the emulator's safeguard that
// rejects queries against NULL_FILTERED indexes (the emulator's
// --disable_query_null_filtered_index_check flag). Emulator-only.
//
// Production Spanner answers such queries; the emulator rejects them by
// default because the result set can legitimately differ from a base-table
// scan. Use this option only in tests that intentionally exercise reads
// against NULL_FILTERED indexes and have accounted for that difference.
func DisableQueryNullFilteredIndexCheck() Option {
	return appendGatewayFlag("--disable_query_null_filtered_index_check")
}

// WithMaxDatabasesPerInstance overrides the emulator's maximum number of
// databases per instance (the emulator's --override_max_databases_per_instance
// flag). n must be positive. Emulator-only.
//
// Per the upstream help text, the emulator only honors values greater than
// Spanner's default limit (100); smaller values are ignored. If the
// MAX_DATABASES_PER_INSTANCE environment variable is also set on the
// container, it takes precedence over this flag.
func WithMaxDatabasesPerInstance(n int) Option {
	return func(opts *emulatorOptions) error {
		if n <= 0 {
			return fmt.Errorf("WithMaxDatabasesPerInstance: n must be > 0, got %d", n)
		}
		opts.gatewayFlags = append(opts.gatewayFlags, fmt.Sprintf("--override_max_databases_per_instance=%d", n))
		return nil
	}
}

// WithChangeStreamPartitionTokenAliveSeconds overrides the alive time of
// change stream partition tokens (the emulator's
// --override_change_stream_partition_token_alive_seconds flag). seconds must
// be positive. Emulator-only.
//
// Per the upstream help text, the effective alive time becomes
// seconds..2*seconds (the emulator's default is 20..40s, which differs from
// production Spanner). This flag is emulator-only and has no effect on
// production Spanner.
func WithChangeStreamPartitionTokenAliveSeconds(seconds int) Option {
	return func(opts *emulatorOptions) error {
		if seconds <= 0 {
			return fmt.Errorf("WithChangeStreamPartitionTokenAliveSeconds: seconds must be > 0, got %d", seconds)
		}
		opts.gatewayFlags = append(opts.gatewayFlags, fmt.Sprintf("--override_change_stream_partition_token_alive_seconds=%d", seconds))
		return nil
	}
}

func appendGatewayFlag(flag string) Option {
	return func(opts *emulatorOptions) error {
		opts.gatewayFlags = append(opts.gatewayFlags, flag)
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

// WithSetupFileDescriptorSet sets proto descriptors for CREATE/ALTER PROTO BUNDLE
// statements in [WithSetupDDLs]. Use this option together with setup DDLs that
// reference proto bundles; the value is serialized for CreateDatabase and
// UpdateDatabaseDdl requests at bootstrap time.
// Calling this multiple times replaces the previous value.
func WithSetupFileDescriptorSet(fds *descriptorpb.FileDescriptorSet) Option {
	var raw []byte
	var err error
	if fds != nil {
		raw, err = proto.Marshal(fds)
	}
	return func(opts *emulatorOptions) error {
		if err != nil {
			return fmt.Errorf("marshal file descriptor set: %w", err)
		}
		opts.setupFileDescriptorSet = raw
		return nil
	}
}

// WithSetupRawFileDescriptorSet sets pre-serialized proto descriptors for
// CREATE/ALTER PROTO BUNDLE statements in [WithSetupDDLs]. Use this option
// together with setup DDLs that reference proto bundles.
// Calling this multiple times replaces the previous value.
// This is mutually exclusive with [WithSetupFileDescriptorSet]; the last one
// called wins.
func WithSetupRawFileDescriptorSet(raw []byte) Option {
	cloned := bytes.Clone(raw)
	return func(opts *emulatorOptions) error {
		opts.setupFileDescriptorSet = cloned
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

func (o *emulatorOptions) hasSetupDDLWork() bool {
	return len(o.setupDDLs) > 0
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

	if err := applyContainerProviderEnv(opts); err != nil {
		return nil, err
	}

	if err := validateSetupFileDescriptorSet(opts); err != nil {
		return nil, err
	}

	return opts, nil
}

func validateSetupFileDescriptorSet(opts *emulatorOptions) error {
	if len(opts.setupFileDescriptorSet) == 0 || len(opts.setupDDLs) > 0 {
		return nil
	}
	if !opts.disableCreateDatabase {
		return nil
	}
	return fmt.Errorf("setup file descriptor set requires WithSetupDDLs when database auto-creation is disabled")
}

func applyContainerProviderEnv(opts *emulatorOptions) error {
	if opts.containerProviderSet {
		return nil
	}

	raw := strings.TrimSpace(os.Getenv(testcontainersProviderEnv))
	if raw == "" {
		return nil
	}

	provider, err := parseContainerProvider(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", testcontainersProviderEnv, err)
	}
	if provider == testcontainers.ProviderDefault {
		return nil
	}

	opts.containerProviderSet = true
	opts.containerCustomizers = append([]testcontainers.ContainerCustomizer{
		testcontainers.WithProvider(provider),
	}, opts.containerCustomizers...)
	return nil
}

func parseContainerProvider(raw string) (testcontainers.ProviderType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "default":
		return testcontainers.ProviderDefault, nil
	case "docker":
		return testcontainers.ProviderDocker, nil
	case "podman":
		return testcontainers.ProviderPodman, nil
	default:
		return testcontainers.ProviderDefault, fmt.Errorf("unsupported testcontainers provider %q; supported values are default, docker, podman", raw)
	}
}

func validateContainerProvider(provider testcontainers.ProviderType, source string) error {
	switch provider {
	case testcontainers.ProviderDefault, testcontainers.ProviderDocker, testcontainers.ProviderPodman:
		return nil
	default:
		return fmt.Errorf("%s: unsupported testcontainers provider %d", source, provider)
	}
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
