// Package integration contains build-tagged integration tests that exercise
// real Docker. Run with:
//
//	DECLOUD_INTEGRATION=1 go test -tags integration ./internal/integration/...
//
// Tests skip if DECLOUD_INTEGRATION is not set to "1".

//go:build integration

package integration_test
