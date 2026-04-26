// Package mocks contains generated mocks for interfaces consumed (but not
// defined) by internal/cli.
//
// LAYOUT NOTE: By project convention, mocks live next to the interface they
// mock — for example, internal/registry/mocks/mock_store.go mocks
// internal/registry.Store. The mocks in this package are an EXCEPTION: they
// mock interfaces defined in internal/deploy (ServiceDeployer, Lifecycle)
// because internal/cli is the sole consumer of those interfaces. Co-locating
// the mock with the only consumer beats co-locating with the interface when
// the interface has exactly one consumer outside its own package.
//
// If a second consumer of internal/deploy.ServiceDeployer or
// internal/deploy.Lifecycle ever appears (e.g., a future "decloud bootstrap"
// command in another package), MOVE these mocks to internal/deploy/mocks/.
package mocks
