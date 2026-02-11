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

const MembershipResourceType = "GitHub::Teams::Membership"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(MembershipResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &Membership{cfg: cfg}
	})
}

type Membership struct {
	cfg *config.Config
}

func (m *Membership) client() (*github.Client, error) {
	token, err := m.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if m.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(m.cfg.BaseURL(), m.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type membershipProperties struct {
	Organization string `json:"organization"`
	TeamSlug     string `json:"teamSlug"`
	Username     string `json:"username"`
	Role         string `json:"role,omitempty"`
	// Read-only
	State string `json:"state,omitempty"`
}

// compositeID returns "org/team-slug/username".
func membershipCompositeID(org, slug, username string) string {
	return org + "/" + slug + "/" + username
}

func parseMembershipID(id string) (org, slug, username string, err error) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid membership ID %q, expected org/team-slug/username", id)
	}
	return parts[0], parts[1], parts[2], nil
}

func (m *Membership) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := m.client()
	if err != nil {
		return nil, err
	}

	var props membershipProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	role := props.Role
	if role == "" {
		role = "member"
	}

	membership, _, err := gh.Teams.AddTeamMembershipBySlug(ctx, props.Organization, props.TeamSlug, props.Username, &github.TeamAddTeamMembershipOptions{
		Role: role,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add team membership: %w", err)
	}

	result := membershipProperties{
		Organization: props.Organization,
		TeamSlug:     props.TeamSlug,
		Username:     props.Username,
		Role:         membership.GetRole(),
		State:        membership.GetState(),
	}
	b, _ := json.Marshal(result)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           membershipCompositeID(props.Organization, props.TeamSlug, props.Username),
			ResourceProperties: b,
		},
	}, nil
}

func (m *Membership) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := m.client()
	if err != nil {
		return nil, err
	}

	org, slug, username, err := parseMembershipID(req.NativeID)
	if err != nil {
		return nil, err
	}

	membership, resp, err := gh.Teams.GetTeamMembershipBySlug(ctx, org, slug, username)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read team membership: %w", err)
	}

	result := membershipProperties{
		Organization: org,
		TeamSlug:     slug,
		Username:     username,
		Role:         membership.GetRole(),
		State:        membership.GetState(),
	}
	b, _ := json.Marshal(result)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(b),
	}, nil
}

func (m *Membership) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := m.client()
	if err != nil {
		return nil, err
	}

	var props membershipProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	org, slug, username, err := parseMembershipID(req.NativeID)
	if err != nil {
		return nil, err
	}

	membership, _, err := gh.Teams.AddTeamMembershipBySlug(ctx, org, slug, username, &github.TeamAddTeamMembershipOptions{
		Role: props.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update team membership: %w", err)
	}

	result := membershipProperties{
		Organization: org,
		TeamSlug:     slug,
		Username:     username,
		Role:         membership.GetRole(),
		State:        membership.GetState(),
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

func (m *Membership) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := m.client()
	if err != nil {
		return nil, err
	}

	org, slug, username, err := parseMembershipID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Teams.RemoveTeamMembershipBySlug(ctx, org, slug, username)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to remove team membership: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (m *Membership) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (m *Membership) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := m.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization
	if org == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	// List all teams, then all members per team.
	teams, _, err := gh.Teams.ListTeams(ctx, org, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	var ids []string
	for _, team := range teams {
		members, _, err := gh.Teams.ListTeamMembersBySlug(ctx, org, team.GetSlug(), &github.TeamListTeamMembersOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		})
		if err != nil {
			continue
		}
		for _, member := range members {
			ids = append(ids, membershipCompositeID(org, team.GetSlug(), member.GetLogin()))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
