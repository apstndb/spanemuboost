.PHONY: test lint emulator-smoke omni-smoke

# Keep this as an explicit test-name list rather than a compact grouped regex so
# CI selection stays easy to audit when smoke coverage changes. Match either the
# top-level test name or one of its subtests.
EMULATOR_SMOKE_TEST_PATTERN = ^(TestRunEmulatorWithClients|TestSetupEmulatorAndSetupClients|TestRuntimePlatformWithStartedRuntime|TestLazyRuntimeEmulatorWithSetupClients)($$|/)
OMNI_SMOKE_TEST_PATTERN = ^(TestRunOmni|TestRunOmniWithClients|TestOpenOmniClients|TestOpenOmniClientsReuseDefaultDatabase|TestOpenOmniClientsAllowDatabaseOverride|TestSetupClientsWithOmni|TestLazyRuntimeWithOmni)($$|/)

test:
	go test -v ./...
lint:
	golangci-lint run
emulator-smoke:
	go test -v -race -count=1 -shuffle=on -p=1 -parallel=1 -run '$(EMULATOR_SMOKE_TEST_PATTERN)' ./...
omni-smoke:
	SPANEMUBOOST_ENABLE_OMNI_TESTS=1 go test -v -race -count=1 -shuffle=on -p=1 -parallel=1 -run '$(OMNI_SMOKE_TEST_PATTERN)' ./...
