package spanemuboost

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AttachedRuntime is a [Runtime] connected to an already-running backend.
// [AttachedRuntime.Close] does not stop the remote process or container.
type AttachedRuntime struct {
	backend Backend
	opts    *emulatorOptions
	uri     string
}

func (*AttachedRuntime) spanemuboostRuntime() {}

// NewAttachedRuntime connects to endpoint without starting a container.
func NewAttachedRuntime(endpoint Endpoint) (*AttachedRuntime, error) {
	if err := endpoint.validate(); err != nil {
		return nil, err
	}
	opts, err := finalizeAttachedOptions(endpoint)
	if err != nil {
		return nil, err
	}
	return &AttachedRuntime{
		backend: endpoint.Backend,
		opts:    opts,
		uri:     endpoint.URI,
	}, nil
}

// NewAttachedRuntimeFromEnv is a convenience wrapper around [LoadEndpoint] and
// [NewAttachedRuntime].
func NewAttachedRuntimeFromEnv() (*AttachedRuntime, error) {
	endpoint, err := LoadEndpoint()
	if err != nil {
		return nil, err
	}
	return NewAttachedRuntime(endpoint)
}

// NewLazyRuntimeFromEnvOrStart returns a [LazyRuntime] that attaches to an
// external endpoint when one is configured in the environment. When no endpoint
// env vars are set, it defers container startup until first use. When endpoint
// env vars are set but [LoadEndpoint] fails, it returns an error instead of
// falling back to cold start.
func NewLazyRuntimeFromEnvOrStart(backend Backend, options ...Option) (*LazyRuntime, error) {
	lr := NewLazyRuntime(backend, options...)
	if !endpointEnvConfigured() {
		return lr, nil
	}
	endpoint, err := LoadEndpoint()
	if err != nil {
		return nil, err
	}
	if endpoint.Backend != backend {
		return nil, fmt.Errorf("spanemuboost: configured endpoint backend %q does not match requested backend %q", endpoint.Backend, backend)
	}
	lr.attachedEndpoint = &endpoint
	return lr, nil
}

// NewLazyRuntimeOptionalEndpoint returns a [LazyRuntime] that attaches to an
// external endpoint when one is configured in the environment. Otherwise it
// starts a container on first use, preserving the existing testcontainers path.
//
// Deprecated: use [NewLazyRuntimeFromEnvOrStart] to handle endpoint load errors.
func NewLazyRuntimeOptionalEndpoint(backend Backend, options ...Option) (*LazyRuntime, error) {
	return NewLazyRuntimeFromEnvOrStart(backend, options...)
}

func finalizeAttachedOptions(endpoint Endpoint) (*emulatorOptions, error) {
	base := &emulatorOptions{
		projectID:             endpoint.ProjectID,
		instanceID:            endpoint.InstanceID,
		disableCreateInstance: true,
	}
	switch endpoint.Backend {
	case BackendOmni:
		return finalizeOmniOptions(base)
	case BackendEmulator:
		return finalizeOptions(base)
	default:
		return nil, fmt.Errorf("unsupported backend %q", endpoint.Backend)
	}
}

func (a *AttachedRuntime) URI() string {
	if a == nil {
		return ""
	}
	return a.uri
}

func (a *AttachedRuntime) ClientOptions() []option.ClientOption {
	if a == nil {
		return nil
	}
	if a.opts != nil && len(a.opts.clientOptionsForClient) > 0 {
		return a.opts.clientOptionsForClient
	}
	switch a.backend {
	case BackendOmni:
		return []option.ClientOption{
			option.WithEndpoint(a.uri),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	default:
		return []option.ClientOption{
			option.WithEndpoint("passthrough:///" + a.uri),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
			internaloption.SkipDialSettingsValidation(),
		}
	}
}

// Close is a no-op for attached runtimes because this handle does not own the
// remote backend lifecycle.
func (a *AttachedRuntime) Close() error { return nil }

func (a *AttachedRuntime) ProjectID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.projectID
}
func (a *AttachedRuntime) InstanceID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.instanceID
}
func (a *AttachedRuntime) DatabaseID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.databaseID
}

func (a *AttachedRuntime) ProjectPath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.ProjectPath()
}
func (a *AttachedRuntime) InstancePath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.InstancePath()
}
func (a *AttachedRuntime) DatabasePath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.DatabasePath()
}

func (a *AttachedRuntime) inheritedOptions(options ...Option) (*emulatorOptions, error) {
	if a == nil || a.opts == nil {
		return nil, fmt.Errorf("spanemuboost: attached runtime or options is nil")
	}
	base := inheritedRuntimeOptions(a.opts)
	switch a.backend {
	case BackendOmni:
		return applyOmniOptionsWithBase(base, options...)
	default:
		return applyOptionsWithBase(base, options...)
	}
}

func (a *AttachedRuntime) runtimePlatform(context.Context) (string, error) {
	return "attached", nil
}
