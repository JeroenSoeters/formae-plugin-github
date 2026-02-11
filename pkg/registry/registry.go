package registry

import (
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/prov"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// ProvisionerFactory creates a provisioner for a given config.
type ProvisionerFactory func(cfg *config.Config) prov.Provisioner

type registryEntry struct {
	operations map[resource.Operation]ProvisionerFactory
}

var entries = make(map[string]*registryEntry)

// Register registers a provisioner factory for a resource type and set of operations.
func Register(resourceType string, operations []resource.Operation, factory ProvisionerFactory) {
	entry, ok := entries[resourceType]
	if !ok {
		entry = &registryEntry{operations: make(map[resource.Operation]ProvisionerFactory)}
		entries[resourceType] = entry
	}
	for _, op := range operations {
		entry.operations[op] = factory
	}
}

// HasProvisioner returns true if a provisioner is registered for the given resource type and operation.
func HasProvisioner(resourceType string, op resource.Operation) bool {
	entry, ok := entries[resourceType]
	if !ok {
		return false
	}
	_, ok = entry.operations[op]
	return ok
}

// Get returns a provisioner for the given resource type, operation, and config.
func Get(resourceType string, op resource.Operation, cfg *config.Config) prov.Provisioner {
	return entries[resourceType].operations[op](cfg)
}
