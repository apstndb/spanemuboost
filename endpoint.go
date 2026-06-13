package spanemuboost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	endpointFileEnv     = "SPANEMUBOOST_ENDPOINT_FILE"
	omniURIEnv          = "SPANEMUBOOST_OMNI_URI"
	omniProjectIDEnv    = "SPANEMUBOOST_OMNI_PROJECT_ID"
	omniInstanceIDEnv   = "SPANEMUBOOST_OMNI_INSTANCE_ID"
	emulatorURIEnv      = "SPANEMUBOOST_EMULATOR_URI"
	emulatorProjectEnv  = "SPANEMUBOOST_EMULATOR_PROJECT_ID"
	emulatorInstanceEnv = "SPANEMUBOOST_EMULATOR_INSTANCE_ID"
)

// Endpoint describes a running spanemuboost-compatible Spanner backend that
// clients can attach to without starting a new container.
type Endpoint struct {
	Backend    Backend `json:"backend"`
	URI        string  `json:"uri"`
	ProjectID  string  `json:"project_id"`
	InstanceID string  `json:"instance_id"`

	// Lifecycle metadata is populated by spanemuboost serve and used by stop.
	ManagedBy string `json:"managed_by,omitempty"`
	PID       int    `json:"pid,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

// EndpointFromRuntime builds an [Endpoint] from a started [Runtime].
func EndpointFromRuntime(runtime Runtime) (Endpoint, error) {
	if runtime == nil || isNilRuntimeValue(runtime) {
		return Endpoint{}, errors.New("spanemuboost: runtime is nil")
	}
	uri := strings.TrimSpace(runtime.URI())
	if uri == "" {
		return Endpoint{}, errors.New("spanemuboost: runtime URI is empty")
	}
	backend := backendForRuntime(runtime)
	endpoint := Endpoint{
		Backend:    backend,
		URI:        uri,
		ProjectID:  runtime.ProjectID(),
		InstanceID: runtime.InstanceID(),
	}
	if err := endpoint.validate(); err != nil {
		return Endpoint{}, err
	}
	return endpoint, nil
}

func backendForRuntime(runtime Runtime) Backend {
	switch r := runtime.(type) {
	case *omniRuntime:
		return BackendOmni
	case *AttachedRuntime:
		return r.backend
	default:
		return BackendEmulator
	}
}

// EndpointConfigured reports whether external endpoint env vars are set in the
// current process environment. It does not validate that [LoadEndpoint] succeeds.
func EndpointConfigured() bool {
	return endpointEnvConfigured()
}

// EndpointConfiguredForBackend reports whether an endpoint for the requested
// backend is configured. Unlike [EndpointConfigured], this ignores unrelated
// backend URI env vars so callers can distinguish Omni from emulator endpoints.
func EndpointConfiguredForBackend(backend Backend) bool {
	if strings.TrimSpace(os.Getenv(endpointFileEnv)) != "" {
		return true
	}
	switch backend {
	case BackendOmni:
		return strings.TrimSpace(os.Getenv(omniURIEnv)) != ""
	case BackendEmulator:
		return strings.TrimSpace(os.Getenv(emulatorURIEnv)) != ""
	default:
		return false
	}
}

func endpointEnvConfigured() bool {
	if strings.TrimSpace(os.Getenv(endpointFileEnv)) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv(omniURIEnv)) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv(emulatorURIEnv)) != "" {
		return true
	}
	return false
}

// LoadEndpoint reads connection metadata from SPANEMUBOOST_ENDPOINT_FILE or
// backend-specific URI env vars.
//
// When SPANEMUBOOST_ENDPOINT_FILE is set, the JSON file takes precedence.
// Otherwise Omni is selected when SPANEMUBOOST_OMNI_URI is set, and the emulator
// path is selected when SPANEMUBOOST_EMULATOR_URI is set.
func LoadEndpoint() (Endpoint, error) {
	if path := strings.TrimSpace(os.Getenv(endpointFileEnv)); path != "" {
		return ReadEndpointFile(path)
	}
	if uri := strings.TrimSpace(os.Getenv(omniURIEnv)); uri != "" {
		endpoint := Endpoint{
			Backend:    BackendOmni,
			URI:        uri,
			ProjectID:  cmpOrEnv(omniProjectIDEnv, defaultOmniProjectID),
			InstanceID: cmpOrEnv(omniInstanceIDEnv, defaultOmniInstanceID),
		}
		return endpoint, endpoint.validate()
	}
	if uri := strings.TrimSpace(os.Getenv(emulatorURIEnv)); uri != "" {
		endpoint := Endpoint{
			Backend:    BackendEmulator,
			URI:        uri,
			ProjectID:  cmpOrEnv(emulatorProjectEnv, DefaultProjectID),
			InstanceID: cmpOrEnv(emulatorInstanceEnv, DefaultInstanceID),
		}
		return endpoint, endpoint.validate()
	}
	return Endpoint{}, fmt.Errorf("spanemuboost: no external endpoint configured; set %s, %s, or %s", endpointFileEnv, omniURIEnv, emulatorURIEnv)
}

// LoadEndpointForBackend reads endpoint metadata for the requested backend.
// When SPANEMUBOOST_ENDPOINT_FILE is set, the file backend must match.
// Otherwise only the URI env var for the requested backend is considered.
func LoadEndpointForBackend(backend Backend) (Endpoint, error) {
	if path := strings.TrimSpace(os.Getenv(endpointFileEnv)); path != "" {
		endpoint, err := ReadEndpointFile(path)
		if err != nil {
			return Endpoint{}, err
		}
		if endpoint.Backend != backend {
			return Endpoint{}, fmt.Errorf("spanemuboost: configured endpoint backend %q does not match requested backend %q", endpoint.Backend, backend)
		}
		return endpoint, nil
	}
	switch backend {
	case BackendOmni:
		if uri := strings.TrimSpace(os.Getenv(omniURIEnv)); uri != "" {
			endpoint := Endpoint{
				Backend:    BackendOmni,
				URI:        uri,
				ProjectID:  cmpOrEnv(omniProjectIDEnv, defaultOmniProjectID),
				InstanceID: cmpOrEnv(omniInstanceIDEnv, defaultOmniInstanceID),
			}
			return endpoint, endpoint.validate()
		}
	case BackendEmulator:
		if uri := strings.TrimSpace(os.Getenv(emulatorURIEnv)); uri != "" {
			endpoint := Endpoint{
				Backend:    BackendEmulator,
				URI:        uri,
				ProjectID:  cmpOrEnv(emulatorProjectEnv, DefaultProjectID),
				InstanceID: cmpOrEnv(emulatorInstanceEnv, DefaultInstanceID),
			}
			return endpoint, endpoint.validate()
		}
	default:
		return Endpoint{}, fmt.Errorf("spanemuboost: unsupported backend %q", backend)
	}
	return Endpoint{}, fmt.Errorf("spanemuboost: no external endpoint configured for backend %q", backend)
}

func cmpOrEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// ReadEndpointFile loads an [Endpoint] from a JSON file written by [SaveEndpoint]
// or `spanemuboost serve`.
func ReadEndpointFile(path string) (Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Endpoint{}, fmt.Errorf("spanemuboost: read endpoint file %q: %w", path, err)
	}
	var endpoint Endpoint
	if err := json.Unmarshal(data, &endpoint); err != nil {
		return Endpoint{}, fmt.Errorf("spanemuboost: parse endpoint file %q: %w", path, err)
	}
	return endpoint, endpoint.validate()
}

// SaveEndpoint writes endpoint metadata as JSON with mode 0600.
func SaveEndpoint(path string, endpoint Endpoint) error {
	if err := endpoint.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(endpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("spanemuboost: marshal endpoint: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("spanemuboost: create endpoint directory %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".endpoint-*.json")
	if err != nil {
		return fmt.Errorf("spanemuboost: create temp endpoint file in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("spanemuboost: write temp endpoint file %q: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("spanemuboost: chmod temp endpoint file %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("spanemuboost: close temp endpoint file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("spanemuboost: write endpoint file %q: %w", path, err)
	}
	cleanup = false
	return nil
}

func (e Endpoint) validate() error {
	if e.Backend != BackendEmulator && e.Backend != BackendOmni {
		return fmt.Errorf("spanemuboost: endpoint backend %q is unsupported", e.Backend)
	}
	if strings.TrimSpace(e.URI) == "" {
		return errors.New("spanemuboost: endpoint URI is required")
	}
	if strings.TrimSpace(e.ProjectID) == "" {
		return errors.New("spanemuboost: endpoint project_id is required")
	}
	if strings.TrimSpace(e.InstanceID) == "" {
		return errors.New("spanemuboost: endpoint instance_id is required")
	}
	return nil
}
