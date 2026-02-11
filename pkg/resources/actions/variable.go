package actions

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

const VariableResourceType = "GitHub::Actions::Variable"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(VariableResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &Variable{cfg: cfg}
	})
}

type Variable struct {
	cfg *config.Config
}

func (v *Variable) client() (*github.Client, error) {
	token, err := v.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if v.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(v.cfg.BaseURL(), v.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type variableProperties struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Name  string `json:"name"`
	Value string `json:"value"`
	// Read-only
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func variableCompositeID(owner, repo, name string) string {
	return owner + "/" + repo + "/" + name
}

func parseVariableID(id string) (owner, repo, name string, err error) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid variable ID %q, expected owner/repo/name", id)
	}
	return parts[0], parts[1], parts[2], nil
}

func (v *Variable) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := v.client()
	if err != nil {
		return nil, err
	}

	var props variableProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	// GitHub normalizes variable names to uppercase.
	props.Name = strings.ToUpper(props.Name)

	_, err = gh.Actions.CreateRepoVariable(ctx, props.Owner, props.Repo, &github.ActionsVariable{
		Name:  props.Name,
		Value: props.Value,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create variable: %w", err)
	}

	b, _ := json.Marshal(props)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           variableCompositeID(props.Owner, props.Repo, props.Name),
			ResourceProperties: b,
		},
	}, nil
}

func (v *Variable) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := v.client()
	if err != nil {
		return nil, err
	}

	owner, repo, name, err := parseVariableID(req.NativeID)
	if err != nil {
		return nil, err
	}

	variable, resp, err := gh.Actions.GetRepoVariable(ctx, owner, repo, name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read variable: %w", err)
	}

	result := variableProperties{
		Owner:     owner,
		Repo:      repo,
		Name:      variable.Name,
		Value:     variable.Value,
		CreatedAt: variable.CreatedAt.String(),
		UpdatedAt: variable.UpdatedAt.String(),
	}
	b, _ := json.Marshal(result)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(b),
	}, nil
}

func (v *Variable) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := v.client()
	if err != nil {
		return nil, err
	}

	var props variableProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	owner, repo, name, err := parseVariableID(req.NativeID)
	if err != nil {
		return nil, err
	}

	_, err = gh.Actions.UpdateRepoVariable(ctx, owner, repo, &github.ActionsVariable{
		Name:  name,
		Value: props.Value,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update variable: %w", err)
	}

	result := variableProperties{
		Owner: owner,
		Repo:  repo,
		Name:  name,
		Value: props.Value,
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

func (v *Variable) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := v.client()
	if err != nil {
		return nil, err
	}

	owner, repo, name, err := parseVariableID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Actions.DeleteRepoVariable(ctx, owner, repo, name)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete variable: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (v *Variable) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (v *Variable) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := v.client()
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
		vars, _, err := gh.Actions.ListRepoVariables(ctx, org, repo.GetName(), &github.ListOptions{PerPage: 100})
		if err != nil {
			continue
		}
		for _, v := range vars.Variables {
			ids = append(ids, variableCompositeID(org, repo.GetName(), v.Name))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
