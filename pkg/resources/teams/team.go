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

const TeamResourceType = "GitHub::Teams::Team"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(TeamResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &Team{cfg: cfg}
	})
}

type Team struct {
	cfg *config.Config
}

func (t *Team) client() (*github.Client, error) {
	token, err := t.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if t.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(t.cfg.BaseURL(), t.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type teamProperties struct {
	Organization        string `json:"organization"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	Privacy             string `json:"privacy,omitempty"`
	NotificationSetting string `json:"notificationSetting,omitempty"`
	Permission          string `json:"permission,omitempty"`
	ParentTeamID        *int64 `json:"parentTeamId,omitempty"`
	// Read-only
	ID      int64  `json:"id,omitempty"`
	Slug    string `json:"slug,omitempty"`
	HtmlUrl string `json:"htmlUrl,omitempty"`
}

// teamCompositeID returns "org/team-slug".
func teamCompositeID(org, slug string) string {
	return org + "/" + slug
}

func parseTeamID(id string) (org, slug string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid team ID %q, expected org/team-slug", id)
	}
	return parts[0], parts[1], nil
}

func teamToJSON(team *github.Team, org string) json.RawMessage {
	p := teamProperties{
		Organization:        org,
		Name:                team.GetName(),
		Description:         team.GetDescription(),
		Privacy:             team.GetPrivacy(),
		NotificationSetting: team.GetNotificationSetting(),
		Permission:          team.GetPermission(),
		ID:                  team.GetID(),
		Slug:                team.GetSlug(),
		HtmlUrl:             team.GetHTMLURL(),
	}
	if team.Parent != nil {
		parentID := team.Parent.GetID()
		p.ParentTeamID = &parentID
	}
	b, _ := json.Marshal(p)
	return b
}

func (t *Team) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := t.client()
	if err != nil {
		return nil, err
	}

	var props teamProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	newTeam := github.NewTeam{
		Name:                props.Name,
		Description:         &props.Description,
		Privacy:             &props.Privacy,
		NotificationSetting: &props.NotificationSetting,
	}
	if props.ParentTeamID != nil {
		newTeam.ParentTeamID = props.ParentTeamID
	}

	team, _, err := gh.Teams.CreateTeam(ctx, props.Organization, newTeam)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           teamCompositeID(props.Organization, team.GetSlug()),
			ResourceProperties: teamToJSON(team, props.Organization),
		},
	}, nil
}

func (t *Team) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := t.client()
	if err != nil {
		return nil, err
	}

	org, slug, err := parseTeamID(req.NativeID)
	if err != nil {
		return nil, err
	}

	team, resp, err := gh.Teams.GetTeamBySlug(ctx, org, slug)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read team: %w", err)
	}

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(teamToJSON(team, org)),
	}, nil
}

func (t *Team) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := t.client()
	if err != nil {
		return nil, err
	}

	var props teamProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	editTeam := github.NewTeam{
		Name:                props.Name,
		Description:         &props.Description,
		Privacy:             &props.Privacy,
		NotificationSetting: &props.NotificationSetting,
	}
	if props.ParentTeamID != nil {
		editTeam.ParentTeamID = props.ParentTeamID
	}

	_, slug, err := parseTeamID(req.NativeID)
	if err != nil {
		return nil, err
	}

	team, _, err := gh.Teams.EditTeamBySlug(ctx, props.Organization, slug, editTeam, false)
	if err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           teamCompositeID(props.Organization, team.GetSlug()),
			ResourceProperties: teamToJSON(team, props.Organization),
		},
	}, nil
}

func (t *Team) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := t.client()
	if err != nil {
		return nil, err
	}

	org, slug, err := parseTeamID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Teams.DeleteTeamBySlug(ctx, org, slug)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete team: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (t *Team) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (t *Team) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := t.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization
	if org == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	page := 1
	if req.PageToken != nil {
		fmt.Sscanf(*req.PageToken, "%d", &page)
	}

	teams, resp, err := gh.Teams.ListTeams(ctx, org, &github.ListOptions{
		Page: page, PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	var ids []string
	for _, team := range teams {
		ids = append(ids, teamCompositeID(org, team.GetSlug()))
	}

	var nextToken *string
	if resp.NextPage != 0 {
		tok := fmt.Sprintf("%d", resp.NextPage)
		nextToken = &tok
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nextToken,
	}, nil
}
