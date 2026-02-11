package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v69/github"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const BranchProtectionResourceType = "GitHub::Repos::BranchProtection"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(BranchProtectionResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &BranchProtection{cfg: cfg}
	})
}

type BranchProtection struct {
	cfg *config.Config
}

func (bp *BranchProtection) client() (*github.Client, error) {
	token, err := bp.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if bp.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(bp.cfg.BaseURL(), bp.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type statusChecksProps struct {
	Strict   bool     `json:"strict"`
	Contexts []string `json:"contexts,omitempty"`
}

type pullRequestReviewProps struct {
	RequiredApprovingReviewCount int  `json:"requiredApprovingReviewCount,omitempty"`
	DismissStaleReviews          bool `json:"dismissStaleReviews,omitempty"`
	RequireCodeOwnerReviews      bool `json:"requireCodeOwnerReviews,omitempty"`
	RequireLastPushApproval      bool `json:"requireLastPushApproval,omitempty"`
}

type branchProtectionProperties struct {
	Owner                      string                  `json:"owner"`
	Repo                       string                  `json:"repo"`
	Branch                     string                  `json:"branch"`
	EnforceAdmins              bool                    `json:"enforceAdmins"`
	RequiredLinearHistory      bool                    `json:"requiredLinearHistory"`
	AllowForcePushes           bool                    `json:"allowForcePushes"`
	AllowDeletions             bool                    `json:"allowDeletions"`
	RequiredStatusChecks       *statusChecksProps      `json:"requiredStatusChecks,omitempty"`
	RequiredPullRequestReviews *pullRequestReviewProps `json:"requiredPullRequestReviews,omitempty"`
}

func bpCompositeID(owner, repo, branch string) string {
	return owner + "/" + repo + "/" + branch
}

func parseBPID(id string) (owner, repo, branch string, err error) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid branch protection ID %q, expected owner/repo/branch", id)
	}
	return parts[0], parts[1], parts[2], nil
}

func buildProtectionRequest(props *branchProtectionProperties) *github.ProtectionRequest {
	req := &github.ProtectionRequest{
		EnforceAdmins:        props.EnforceAdmins,
		RequireLinearHistory: github.Ptr(props.RequiredLinearHistory),
		AllowForcePushes:     github.Ptr(props.AllowForcePushes),
		AllowDeletions:       github.Ptr(props.AllowDeletions),
	}

	if props.RequiredStatusChecks != nil {
		checks := make([]*github.RequiredStatusCheck, 0, len(props.RequiredStatusChecks.Contexts))
		for _, c := range props.RequiredStatusChecks.Contexts {
			checks = append(checks, &github.RequiredStatusCheck{Context: c})
		}
		req.RequiredStatusChecks = &github.RequiredStatusChecks{
			Strict: props.RequiredStatusChecks.Strict,
			Checks: &checks,
		}
	}

	if props.RequiredPullRequestReviews != nil {
		req.RequiredPullRequestReviews = &github.PullRequestReviewsEnforcementRequest{
			RequiredApprovingReviewCount: props.RequiredPullRequestReviews.RequiredApprovingReviewCount,
			DismissStaleReviews:         props.RequiredPullRequestReviews.DismissStaleReviews,
			RequireCodeOwnerReviews:     props.RequiredPullRequestReviews.RequireCodeOwnerReviews,
			RequireLastPushApproval:     github.Ptr(props.RequiredPullRequestReviews.RequireLastPushApproval),
		}
	}

	return req
}

func protectionToJSON(p *github.Protection, owner, repo, branch string) json.RawMessage {
	props := branchProtectionProperties{
		Owner:  owner,
		Repo:   repo,
		Branch: branch,
	}

	if p.EnforceAdmins != nil {
		props.EnforceAdmins = p.EnforceAdmins.Enabled
	}
	if p.RequireLinearHistory != nil {
		props.RequiredLinearHistory = p.RequireLinearHistory.Enabled
	}
	if p.AllowForcePushes != nil {
		props.AllowForcePushes = p.AllowForcePushes.Enabled
	}
	if p.AllowDeletions != nil {
		props.AllowDeletions = p.AllowDeletions.Enabled
	}
	if p.RequiredStatusChecks != nil {
		sc := &statusChecksProps{
			Strict: p.RequiredStatusChecks.Strict,
		}
		if p.RequiredStatusChecks.Checks != nil {
			for _, check := range *p.RequiredStatusChecks.Checks {
				sc.Contexts = append(sc.Contexts, check.Context)
			}
		}
		props.RequiredStatusChecks = sc
	}
	if p.RequiredPullRequestReviews != nil {
		props.RequiredPullRequestReviews = &pullRequestReviewProps{
			RequiredApprovingReviewCount: p.RequiredPullRequestReviews.RequiredApprovingReviewCount,
			DismissStaleReviews:         p.RequiredPullRequestReviews.DismissStaleReviews,
			RequireCodeOwnerReviews:     p.RequiredPullRequestReviews.RequireCodeOwnerReviews,
			RequireLastPushApproval:     p.RequiredPullRequestReviews.RequireLastPushApproval,
		}
	}

	b, _ := json.Marshal(props)
	return b
}

func (bp *BranchProtection) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := bp.client()
	if err != nil {
		return nil, err
	}

	var props branchProtectionProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	protection, _, err := gh.Repositories.UpdateBranchProtection(ctx, props.Owner, props.Repo, props.Branch, buildProtectionRequest(&props))
	if err != nil {
		return nil, fmt.Errorf("failed to create branch protection: %w", err)
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           bpCompositeID(props.Owner, props.Repo, props.Branch),
			ResourceProperties: protectionToJSON(protection, props.Owner, props.Repo, props.Branch),
		},
	}, nil
}

func (bp *BranchProtection) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := bp.client()
	if err != nil {
		return nil, err
	}

	owner, repo, branch, err := parseBPID(req.NativeID)
	if err != nil {
		return nil, err
	}

	protection, resp, err := gh.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read branch protection: %w", err)
	}

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(protectionToJSON(protection, owner, repo, branch)),
	}, nil
}

func (bp *BranchProtection) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := bp.client()
	if err != nil {
		return nil, err
	}

	var props branchProtectionProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	owner, repo, branch, err := parseBPID(req.NativeID)
	if err != nil {
		return nil, err
	}

	protection, _, err := gh.Repositories.UpdateBranchProtection(ctx, owner, repo, branch, buildProtectionRequest(&props))
	if err != nil {
		return nil, fmt.Errorf("failed to update branch protection: %w", err)
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           req.NativeID,
			ResourceProperties: protectionToJSON(protection, owner, repo, branch),
		},
	}, nil
}

func (bp *BranchProtection) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := bp.client()
	if err != nil {
		return nil, err
	}

	owner, repo, branch, err := parseBPID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Repositories.RemoveBranchProtection(ctx, owner, repo, branch)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to remove branch protection: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (bp *BranchProtection) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (bp *BranchProtection) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := bp.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization
	if org == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	repos, _, err := gh.Repositories.ListByOrg(ctx, org, &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	var ids []string
	for _, repo := range repos {
		branches, _, err := gh.Repositories.ListBranches(ctx, org, repo.GetName(), &github.BranchListOptions{
			Protected:   github.Ptr(true),
			ListOptions: github.ListOptions{PerPage: 100},
		})
		if err != nil {
			continue
		}
		for _, branch := range branches {
			ids = append(ids, bpCompositeID(org, repo.GetName(), branch.GetName()))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
