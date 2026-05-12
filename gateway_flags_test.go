package spanemuboost

import (
	"slices"
	"strings"
	"testing"
)

func TestGatewayFlagOptionsAppendExpectedFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{name: "EnableFaultInjection", opt: EnableFaultInjection(), want: "--enable_fault_injection"},
		{name: "EnableLogRequests", opt: EnableLogRequests(), want: "--log_requests"},
		{name: "EnableEmulatorStdoutCopy", opt: EnableEmulatorStdoutCopy(), want: "--copy_emulator_stdout"},
		{name: "DisableQueryNullFilteredIndexCheck", opt: DisableQueryNullFilteredIndexCheck(), want: "--disable_query_null_filtered_index_check"},
		{name: "WithMaxDatabasesPerInstance", opt: WithMaxDatabasesPerInstance(500), want: "--override_max_databases_per_instance=500"},
		{name: "WithChangeStreamPartitionTokenAliveSeconds", opt: WithChangeStreamPartitionTokenAliveSeconds(7), want: "--override_change_stream_partition_token_alive_seconds=7"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, err := applyOptions(tc.opt)
			if err != nil {
				t.Fatalf("applyOptions: %v", err)
			}
			if !slices.Contains(opts.gatewayFlags, tc.want) {
				t.Fatalf("gatewayFlags = %v, want to contain %q", opts.gatewayFlags, tc.want)
			}
		})
	}
}

func TestGatewayFlagOptionsAccumulate(t *testing.T) {
	t.Parallel()
	opts, err := applyOptions(
		EnableFaultInjection(),
		EnableLogRequests(),
		WithMaxDatabasesPerInstance(200),
	)
	if err != nil {
		t.Fatalf("applyOptions: %v", err)
	}
	want := []string{
		"--enable_fault_injection",
		"--log_requests",
		"--override_max_databases_per_instance=200",
	}
	if !slices.Equal(opts.gatewayFlags, want) {
		t.Fatalf("gatewayFlags = %v, want %v", opts.gatewayFlags, want)
	}
}

func TestGatewayFlagOptionsRejectNonPositive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opt  Option
	}{
		{name: "WithMaxDatabasesPerInstance/zero", opt: WithMaxDatabasesPerInstance(0)},
		{name: "WithMaxDatabasesPerInstance/negative", opt: WithMaxDatabasesPerInstance(-1)},
		{name: "WithChangeStreamPartitionTokenAliveSeconds/zero", opt: WithChangeStreamPartitionTokenAliveSeconds(0)},
		{name: "WithChangeStreamPartitionTokenAliveSeconds/negative", opt: WithChangeStreamPartitionTokenAliveSeconds(-5)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := applyOptions(tc.opt); err == nil {
				t.Fatal("applyOptions: want error, got nil")
			}
		})
	}
}

// TestOmniGuardrailRejectsGatewayFlags ensures the gateway-flag Option helpers
// do not silently leak into Omni's start command. The guardrail must fire
// unless DisableBackendGuardrails is set.
func TestOmniGuardrailRejectsGatewayFlags(t *testing.T) {
	t.Parallel()

	t.Run("rejected by default", func(t *testing.T) {
		t.Parallel()
		_, err := applyOmniOptions(EnableFaultInjection())
		if err == nil {
			t.Fatal("applyOmniOptions: want error, got nil")
		}
		if !strings.Contains(err.Error(), "--enable_fault_injection") {
			t.Fatalf("error %q does not mention the rejected flag", err.Error())
		}
	})

	t.Run("allowed when guardrails disabled", func(t *testing.T) {
		t.Parallel()
		opts, err := applyOmniOptions(EnableFaultInjection(), DisableBackendGuardrails())
		if err != nil {
			t.Fatalf("applyOmniOptions: %v", err)
		}
		if !slices.Contains(opts.gatewayFlags, "--enable_fault_injection") {
			t.Fatalf("gatewayFlags = %v, want to contain --enable_fault_injection", opts.gatewayFlags)
		}
	})
}
