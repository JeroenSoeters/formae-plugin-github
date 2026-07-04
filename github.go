package main

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/registry"
	pkgmodel "github.com/platform-engineering-labs/formae/pkg/model"
	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	// Register all resource provisioners via init().
	_ "github.com/platform-engineering-labs/formae-plugin-github/pkg/resources"
)

// Plugin implements the Formae ResourcePlugin interface for GitHub.
type Plugin struct{}

var _ plugin.ResourcePlugin = &Plugin{}

// RateLimit returns rate limiting config. GitHub allows 5,000 req/hour (~1.4/sec).
// We set 2/sec with namespace scope to stay comfortably under limits.
func (p *Plugin) RateLimit() pkgmodel.RateLimitConfig {
	return pkgmodel.RateLimitConfig{
		Scope:                            pkgmodel.RateLimitScopeNamespace,
		MaxRequestsPerSecondForNamespace: 2,
	}
}

// DiscoveryFilters returns filters to exclude certain resources from discovery.
func (p *Plugin) DiscoveryFilters() []pkgmodel.MatchFilter {
	return nil
}

// LabelConfig returns the configuration for extracting human-readable labels.
func (p *Plugin) LabelConfig() pkgmodel.LabelConfig {
	return pkgmodel.LabelConfig{
		DefaultQuery: "$.name",
		ResourceOverrides: map[string]string{
			"GitHub::Repos::Repository":       "$.fullName",
			"GitHub::Actions::WorkflowRun":    "$.workflow",
			"GitHub::Repos::BranchProtection": "$.branch",
			"GitHub::Teams::Membership":       "$.username",
			"GitHub::Teams::RepositoryAccess": "$.repository",
			"GitHub::Actions::Secret":         "$.name",
			"GitHub::Actions::Variable":       "$.name",
		},
	}
}

func (p *Plugin) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationCreate) {
		return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
	}
	return registry.Get(req.ResourceType, resource.OperationCreate, cfg).Create(ctx, req)
}

func (p *Plugin) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationRead) {
		return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
	}
	return registry.Get(req.ResourceType, resource.OperationRead, cfg).Read(ctx, req)
}

func (p *Plugin) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationUpdate) {
		return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
	}
	return registry.Get(req.ResourceType, resource.OperationUpdate, cfg).Update(ctx, req)
}

func (p *Plugin) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationDelete) {
		return nil, fmt.Errorf("unsupported resource type: %s", req.ResourceType)
	}
	return registry.Get(req.ResourceType, resource.OperationDelete, cfg).Delete(ctx, req)
}

func (p *Plugin) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	plugin.LoggerFromContext(ctx).Info("plugin.Status invoked", "resourceType", req.ResourceType, "hasProvisioner", registry.HasProvisioner(req.ResourceType, resource.OperationCheckStatus))
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationCheckStatus) {
		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationCheckStatus,
				OperationStatus: resource.OperationStatusSuccess,
			},
		}, nil
	}
	return registry.Get(req.ResourceType, resource.OperationCheckStatus, cfg).Status(ctx, req)
}

func (p *Plugin) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	cfg := config.FromTargetConfig(req.TargetConfig)
	if !registry.HasProvisioner(req.ResourceType, resource.OperationList) {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}
	return registry.Get(req.ResourceType, resource.OperationList, cfg).List(ctx, req)
}
