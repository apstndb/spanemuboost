.PHONY: test lint omni-smoke

# Keep this as an explicit test-name list rather than a compact grouped regex so
# CI selection stays easy to audit when Omni smoke coverage changes.
OMNI_SMOKE_TEST_PATTERN = ^(TestRunOmni|TestRunOmniWithClients|TestOpenOmniClients|TestOpenOmniClientsReuseDefaultDatabase|TestOpenOmniClientsAllowDatabaseOverride|TestSetupClientsWithOmni|TestLazyRuntimeWithOmni)$$

test:
	go test -v ./...
lint:
	golangci-lint run
omni-smoke:
	SPANEMUBOOST_ENABLE_OMNI_TESTS=1 go test -v -race -count=1 -shuffle=on -run '$(OMNI_SMOKE_TEST_PATTERN)' ./...
