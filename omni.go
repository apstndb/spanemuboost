package spanemuboost

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	defaultOmniImage      = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r1-beta"
	defaultOmniProjectID  = "default"
	defaultOmniInstanceID = "default"
)

var omniGRPCPort = nat.Port("15000/tcp")

type omniRuntime struct {
	container testcontainers.Container
	opts      *emulatorOptions
	uri       string

	closeState closeState
}

func (*omniRuntime) spanemuboostRuntime() {}

// URI returns the gRPC endpoint (host:port) of the Omni server.
func (o *omniRuntime) URI() string {
	return o.uri
}

// ClientOptions returns recommended [option.ClientOption] values for connecting
// to the Omni gRPC endpoint without authentication.
func (o *omniRuntime) ClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(o.URI()),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}

// RecommendedOmniClientConfig returns the recommended [spanner.ClientConfig]
// for a Go Spanner data client connecting to the experimental Omni backend.
// The helper remains part of the backend-neutral API surface, but its
// Omni-specific recommendations may evolve before v1.
func RecommendedOmniClientConfig() spanner.ClientConfig {
	return spanner.ClientConfig{
		DisableNativeMetrics: true,
		IsExperimentalHost:   true,
	}
}

func finalizeManagedOmniClientConfig(config *spanner.ClientConfig, disableBackendGuardrails bool) *spanner.ClientConfig {
	if config == nil {
		cfg := RecommendedOmniClientConfig()
		return &cfg
	}

	cfg := *config
	if !disableBackendGuardrails {
		// Guardrailed Omni managed clients must force the recommended transport
		// settings even when the caller supplied a custom ClientConfig.
		recommended := RecommendedOmniClientConfig()
		cfg.DisableNativeMetrics = recommended.DisableNativeMetrics
		cfg.IsExperimentalHost = recommended.IsExperimentalHost
	}
	return &cfg
}

// Close terminates the Omni container.
func (o *omniRuntime) Close() error {
	if o == nil {
		return nil
	}
	return o.closeState.close(func() error {
		if o.container == nil {
			return nil
		}
		ctx, cancel := newCloseContext()
		defer cancel()
		return o.container.Terminate(ctx)
	})
}

// ProjectID returns the fixed project ID used by the single-server Omni deployment.
func (o *omniRuntime) ProjectID() string { return o.opts.projectID }

// InstanceID returns the fixed instance ID used by the single-server Omni deployment.
func (o *omniRuntime) InstanceID() string { return o.opts.instanceID }

// DatabaseID returns the configured database ID.
func (o *omniRuntime) DatabaseID() string { return o.opts.databaseID }

// ProjectPath returns the project resource path.
func (o *omniRuntime) ProjectPath() string { return o.opts.ProjectPath() }

// InstancePath returns the instance resource path.
func (o *omniRuntime) InstancePath() string { return o.opts.InstancePath() }

// DatabasePath returns the database resource path.
func (o *omniRuntime) DatabasePath() string { return o.opts.DatabasePath() }

func runOmni(ctx context.Context, options ...Option) (Runtime, error) {
	opts, err := applyOmniOptions(options...)
	if err != nil {
		return nil, err
	}

	omni, err := startOmni(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := bootstrapOmni(ctx, omni, opts); err != nil {
		logCloseError("close omni after bootstrap failure", omni.Close())
		return nil, wrapOmniBootstrapError(err)
	}
	return omni, nil
}

func runOmniWithClients(ctx context.Context, options ...Option) (*RuntimeEnv, error) {
	opts, err := applyOmniOptions(options...)
	if err != nil {
		return nil, err
	}

	omni, err := startOmni(ctx, opts)
	if err != nil {
		return nil, err
	}
	clients, err := bootstrapAndCreateClientsWithOptions(ctx, omni.URI(), opts, omni.ClientOptions())
	if err != nil {
		logCloseError("close omni after client creation failure", omni.Close())
		return nil, wrapOmniBootstrapError(err)
	}

	disableSchemaTeardownUnlessForced(opts, clients)

	return &RuntimeEnv{Clients: clients, runtime: omni}, nil
}

func (o *omniRuntime) inheritedOptions(options ...Option) (*emulatorOptions, error) {
	base := inheritedRuntimeOptions(o.opts)
	base.disableBackendGuardrails = o.opts.disableBackendGuardrails
	return applyOmniOptionsWithBase(base, options...)
}

func (o *omniRuntime) runtimePlatform(ctx context.Context) (string, error) {
	return containerPlatform(ctx, o.container)
}

func applyOmniOptions(options ...Option) (*emulatorOptions, error) {
	opts := &emulatorOptions{disableCreateInstance: true}
	for _, opt := range options {
		if err := opt(opts); err != nil {
			return nil, err
		}
	}
	return finalizeOmniOptions(opts)
}

func applyOmniOptionsWithBase(base *emulatorOptions, options ...Option) (*emulatorOptions, error) {
	opts := *base
	for _, opt := range options {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}
	return finalizeOmniOptions(&opts)
}

func finalizeOmniOptions(opts *emulatorOptions) (*emulatorOptions, error) {
	if opts.randomProjectID && opts.projectID != "" {
		return nil, fmt.Errorf("WithRandomProjectID() and WithProjectID() are mutually exclusive")
	}
	if opts.randomInstanceID && opts.instanceID != "" {
		return nil, fmt.Errorf("WithRandomInstanceID() and WithInstanceID() are mutually exclusive")
	}
	if opts.randomProjectID {
		if !opts.disableBackendGuardrails {
			return nil, omniGuardrailError("WithRandomProjectID() is unsupported for Spanner Omni single-server because the deployment uses a fixed project", "use the built-in default project, or DisableBackendGuardrails() to bypass this validation")
		}
		opts.projectID = generateRandomID()
	}
	if opts.randomInstanceID {
		if !opts.disableBackendGuardrails {
			return nil, omniGuardrailError("WithRandomInstanceID() is unsupported for Spanner Omni single-server because the deployment uses a fixed instance", "use the built-in default instance, or DisableBackendGuardrails() to bypass this validation")
		}
		opts.instanceID = generateRandomID()
	}
	if !opts.disableBackendGuardrails && opts.projectID != "" && opts.projectID != defaultOmniProjectID {
		return nil, omniGuardrailError(fmt.Sprintf("WithProjectID(%q) is unsupported for Spanner Omni single-server; only %q is known to work", opts.projectID, defaultOmniProjectID), "use the built-in default project, or DisableBackendGuardrails() to bypass this validation")
	}
	if !opts.disableBackendGuardrails && opts.instanceID != "" && opts.instanceID != defaultOmniInstanceID {
		return nil, omniGuardrailError(fmt.Sprintf("WithInstanceID(%q) is unsupported for Spanner Omni single-server; only %q is known to work", opts.instanceID, defaultOmniInstanceID), "use the built-in default instance, or DisableBackendGuardrails() to bypass this validation")
	}
	if !opts.disableCreateInstance && !opts.disableBackendGuardrails {
		return nil, omniGuardrailError("instance auto-configuration is unsupported for Spanner Omni single-server because the built-in default instance cannot be created, updated, or deleted", "use the default behavior or EnableDatabaseAutoConfigOnly(), or DisableBackendGuardrails() to bypass this validation")
	}
	if opts.randomDatabaseID && opts.databaseID != "" {
		return nil, fmt.Errorf("WithRandomDatabaseID() and WithDatabaseID() are mutually exclusive")
	}
	if opts.randomDatabaseID {
		opts.databaseID = generateRandomID()
	}

	opts.emulatorImage = cmp.Or(opts.emulatorImage, defaultOmniImage)
	opts.projectID = cmp.Or(opts.projectID, defaultOmniProjectID)
	opts.instanceID = cmp.Or(opts.instanceID, defaultOmniInstanceID)
	opts.databaseID = cmp.Or(opts.databaseID, DefaultDatabaseID)
	opts.clientConfig = finalizeManagedOmniClientConfig(opts.clientConfig, opts.disableBackendGuardrails)
	return opts, nil
}

func omniGuardrailError(problem, suggestion string) error {
	return fmt.Errorf("spanemuboost: %s; %s", problem, suggestion)
}

func startOmni(ctx context.Context, opts *emulatorOptions) (*omniRuntime, error) {
	container, err := newOmni(ctx, opts)
	if err != nil {
		return nil, err
	}

	uri, err := container.PortEndpoint(ctx, omniGRPCPort, "")
	if err != nil {
		// Cleanup must still run if setup failed because ctx was canceled or
		// timed out, so don't reuse the possibly-dead setup context here.
		logCloseError("terminate omni container after endpoint lookup failure", container.Terminate(context.Background()))
		return nil, err
	}

	return &omniRuntime{
		container: container,
		opts:      opts,
		uri:       uri,
	}, nil
}

func wrapOmniBootstrapError(err error) error {
	message := "spanemuboost: bootstrap omni backend: %w"
	switch status.Code(err) {
	case codes.DeadlineExceeded, codes.Unavailable:
		return fmt.Errorf(message+"; verify that the environment satisfies the Spanner Omni software requirements: https://docs.cloud.google.com/spanner-omni/system-requirements#software-requirements", err)
	default:
		return fmt.Errorf(message, err)
	}
}

func newOmni(ctx context.Context, opts *emulatorOptions) (testcontainers.Container, error) {
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        opts.emulatorImage,
			ExposedPorts: []string{string(omniGRPCPort)},
			Cmd:          []string{"start-single-server"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Spanner is ready"),
				wait.ForExposedPort().SkipInternalCheck(),
			).WithDeadline(5 * time.Minute),
		},
		Started: true,
	}
	for _, customizer := range opts.containerCustomizers {
		if err := customizer.Customize(&req); err != nil {
			return nil, err
		}
	}
	return testcontainers.GenericContainer(ctx, req)
}

func bootstrapOmni(ctx context.Context, omni *omniRuntime, opts *emulatorOptions) error {
	return bootstrapWithManagedClientConfig(ctx, opts, omni.ClientOptions())
}
