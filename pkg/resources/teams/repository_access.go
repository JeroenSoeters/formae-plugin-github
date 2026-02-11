package teams

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

const RepositoryAccessResourceType = "GitHub::Teams::RepositoryAccess"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(RepositoryAccessResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &RepositoryAccess{cfg: cfg}
	})
}

type RepositoryAccess struct {
	cfg *config.Config
}

func (ra *RepositoryAccess) client() (*github.Client, error) {
	token, err := ra.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if ra.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(ra.cfg.BaseURL(), ra.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type repoAccessProperties struct {
	Organization string `json:"organization"`
	TeamSlug     string `json:"teamSlug"`
	Repository   string `json:"repository"`
	Permission   string `json:"permission,omitempty"`
}

// compositeID returns "org/team-slug/owner/repo".
func repoAccessCompositeID(org, slug, repo string) string {
	return org + "/" + slug + "/" + repo
}

func parseRepoAccessID(id string) (org, slug, owner, repo string, err error) {
	parts := strings.SplitN(id, "/", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("invalid repo access ID %q, expected org/team-slug/owner/repo", id)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func (ra *RepositoryAccess) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := ra.client()
	if err != nil {
		return nil, err
	}

	var props repoAccessProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	repoParts := strings.SplitN(props.Repository, "/", 2)
	if len(repoParts) != 2 {
		return nil, fmt.Errorf("invalid repository %q, expected owner/repo", props.Repository)
	}
	repoOwner, repoName := repoParts[0], repoParts[1]

	perm := props.Permission
	if perm == "" {
		perm = "pull"
	}

	_, err = gh.Teams.AddTeamRepoBySlug(ctx, props.Organization, props.TeamSlug, repoOwner, repoName, &github.TeamAddTeamRepoOptions{
		Permission: perm,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add team repository access: %w", err)
	}

	result := repoAccessProperties{
		Organization: props.Organization,
		TeamSlug:     props.TeamSlug,
		Repository:   props.Repository,
		Permission:   perm,
	}
	b, _ := json.Marshal(result)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           repoAccessCompositeID(props.Organization, props.TeamSlug, props.Repository),
			ResourceProperties: b,
		},
	}, nil
}

func (ra *RepositoryAccess) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := ra.client()
	if err != nil {
		return nil, err
	}

	org, slug, owner, repo, err := parseRepoAccessID(req.NativeID)
	if err != nil {
		return nil, err
	}

	ghRepo, resp, err := gh.Teams.IsTeamRepoBySlug(ctx, org, slug, owner, repo)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read team repository access: %w", err)
	}

	// Extract the permission from the repo's permissions field.
	perm := "pull"
	if ghRepo.Permissions != nil {
		if ghRepo.Permissions["admin"] {
			perm = "admin"
		} else if ghRepo.Permissions["maintain"] {
			perm = "maintain"
		} else if ghRepo.Permissions["push"] {
			perm = "push"
		} else if ghRepo.Permissions["triage"] {
			perm = "triage"
		}
	}

	result := repoAccessProperties{
		Organization: org,
		TeamSlug:     slug,
		Repository:   owner + "/" + repo,
		Permission:   perm,
	}
	b, _ := json.Marshal(result)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(b),
	}, nil
}

func (ra *RepositoryAccess) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	// Update is the same as Create for team repo access (PUT is idempotent).
	gh, err := ra.client()
	if err != nil {
		return nil, err
	}

	var props repoAccessProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	org, slug, owner, repo, err := parseRepoAccessID(req.NativeID)
	if err != nil {
		return nil, err
	}

	_, err = gh.Teams.AddTeamRepoBySlug(ctx, org, slug, owner, repo, &github.TeamAddTeamRepoOptions{
		Permission: props.Permission,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update team repository access: %w", err)
	}

	result := repoAccessProperties{
		Organization: org,
		TeamSlug:     slug,
		Repository:   owner + "/" + repo,
		Permission:   props.Permission,
	}
	b, _ := json.Marshal(result)

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           req.NativeID,
			ResourceProperties: b,
		},
	}, nil
}

func (ra *RepositoryAccess) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := ra.client()
	if err != nil {
		return nil, err
	}

	org, slug, owner, repo, err := parseRepoAccessID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Teams.RemoveTeamRepoBySlug(ctx, org, slug, owner, repo)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to remove team repository access: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (ra *RepositoryAccess) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (ra *RepositoryAccess) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := ra.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization
	if org == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	teams, _, err := gh.Teams.ListTeams(ctx, org, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	var ids []string
	for _, team := range teams {
		repos, _, err := gh.Teams.ListTeamReposBySlug(ctx, org, team.GetSlug(), &github.ListOptions{PerPage: 100})
		if err != nil {
			continue
		}
		for _, repo := range repos {
			ids = append(ids, repoAccessCompositeID(org, team.GetSlug(), repo.GetFullName()))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
