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

const ResourceType = "GitHub::Repos::Repository"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(ResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &Repository{cfg: cfg}
	})
}

// Repository implements the provisioner for GitHub repositories.
type Repository struct {
	cfg *config.Config
}

func (r *Repository) client() (*github.Client, error) {
	token, err := r.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if r.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(r.cfg.BaseURL(), r.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

// repoProperties is the JSON structure for repository properties.
type repoProperties struct {
	Owner                string   `json:"owner"`
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Visibility           string   `json:"visibility,omitempty"`
	Homepage             string   `json:"homepage,omitempty"`
	HasIssues            *bool    `json:"hasIssues,omitempty"`
	HasWiki              *bool    `json:"hasWiki,omitempty"`
	HasProjects          *bool    `json:"hasProjects,omitempty"`
	IsTemplate           *bool    `json:"isTemplate,omitempty"`
	DefaultBranch        string   `json:"defaultBranch,omitempty"`
	AllowSquashMerge     *bool    `json:"allowSquashMerge,omitempty"`
	AllowMergeCommit     *bool    `json:"allowMergeCommit,omitempty"`
	AllowRebaseMerge     *bool    `json:"allowRebaseMerge,omitempty"`
	AllowAutoMerge       *bool    `json:"allowAutoMerge,omitempty"`
	DeleteBranchOnMerge  *bool    `json:"deleteBranchOnMerge,omitempty"`
	Archived             *bool    `json:"archived,omitempty"`
	Topics               []string `json:"topics,omitempty"`
	AutoInit             *bool    `json:"autoInit,omitempty"`
	GitignoreTemplate    string   `json:"gitignoreTemplate,omitempty"`
	LicenseTemplate      string   `json:"licenseTemplate,omitempty"`
	// Read-only fields populated by Read
	FullName string `json:"fullName,omitempty"`
	HtmlUrl  string `json:"htmlUrl,omitempty"`
	CloneUrl string `json:"cloneUrl,omitempty"`
	SshUrl   string `json:"sshUrl,omitempty"`
}

func parseProps(raw json.RawMessage) (*repoProperties, error) {
	var p repoProperties
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}
	return &p, nil
}

func repoToJSON(repo *github.Repository) json.RawMessage {
	p := repoProperties{
		Owner:       repo.GetOwner().GetLogin(),
		Name:        repo.GetName(),
		Description: repo.GetDescription(),
		Visibility:  repo.GetVisibility(),
		Homepage:    repo.GetHomepage(),
		FullName:    repo.GetFullName(),
		HtmlUrl:     repo.GetHTMLURL(),
		CloneUrl:    repo.GetCloneURL(),
		SshUrl:      repo.GetSSHURL(),
	}
	p.HasIssues = repo.HasIssues
	p.HasWiki = repo.HasWiki
	p.HasProjects = repo.HasProjects
	p.IsTemplate = repo.IsTemplate
	p.AllowSquashMerge = repo.AllowSquashMerge
	p.AllowMergeCommit = repo.AllowMergeCommit
	p.AllowRebaseMerge = repo.AllowRebaseMerge
	p.AllowAutoMerge = repo.AllowAutoMerge
	p.DeleteBranchOnMerge = repo.DeleteBranchOnMerge
	p.Archived = repo.Archived
	if repo.GetDefaultBranch() != "" {
		p.DefaultBranch = repo.GetDefaultBranch()
	}
	p.Topics = repo.Topics
	b, _ := json.Marshal(p)
	return b
}

// splitOwnerRepo splits "owner/repo" into its parts.
func splitOwnerRepo(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository identifier %q, expected owner/repo", fullName)
	}
	return parts[0], parts[1], nil
}

func (r *Repository) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := r.client()
	if err != nil {
		return nil, err
	}
	props, err := parseProps(req.Properties)
	if err != nil {
		return nil, err
	}

	createRepo := &github.Repository{
		Name:                github.Ptr(props.Name),
		Description:         github.Ptr(props.Description),
		Homepage:            github.Ptr(props.Homepage),
		Visibility:          github.Ptr(props.Visibility),
		HasIssues:           props.HasIssues,
		HasWiki:             props.HasWiki,
		HasProjects:         props.HasProjects,
		IsTemplate:          props.IsTemplate,
		AllowSquashMerge:    props.AllowSquashMerge,
		AllowMergeCommit:    props.AllowMergeCommit,
		AllowRebaseMerge:    props.AllowRebaseMerge,
		AllowAutoMerge:      props.AllowAutoMerge,
		DeleteBranchOnMerge: props.DeleteBranchOnMerge,
		AutoInit:            props.AutoInit,
		GitignoreTemplate:   github.Ptr(props.GitignoreTemplate),
		LicenseTemplate:     github.Ptr(props.LicenseTemplate),
	}

	// go-github requires empty string for user-owned repos, org name for org repos.
	// Check if the authenticated user matches the owner to determine which path.
	org := props.Owner
	authUser, _, userErr := gh.Users.Get(ctx, "")
	if userErr == nil && strings.EqualFold(authUser.GetLogin(), props.Owner) {
		org = ""
	}

	var repo *github.Repository
	repo, _, err = gh.Repositories.Create(ctx, org, createRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Set topics if specified (separate API call).
	if len(props.Topics) > 0 {
		_, _, err = gh.Repositories.ReplaceAllTopics(ctx, props.Owner, props.Name, props.Topics)
		if err != nil {
			return nil, fmt.Errorf("failed to set topics: %w", err)
		}
		repo.Topics = props.Topics
	}

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCreate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:           repo.GetFullName(),
			ResourceProperties: repoToJSON(repo),
		},
	}, nil
}

func (r *Repository) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := r.client()
	if err != nil {
		return nil, err
	}

	owner, name, err := splitOwnerRepo(req.NativeID)
	if err != nil {
		return nil, err
	}

	repo, resp, err := gh.Repositories.Get(ctx, owner, name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read repository: %w", err)
	}

	// Fetch topics separately.
	topics, _, topicsErr := gh.Repositories.ListAllTopics(ctx, owner, name)
	if topicsErr == nil {
		repo.Topics = topics
	}

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(repoToJSON(repo)),
	}, nil
}

func (r *Repository) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := r.client()
	if err != nil {
		return nil, err
	}

	owner, name, err := splitOwnerRepo(req.NativeID)
	if err != nil {
		return nil, err
	}

	props, err := parseProps(req.DesiredProperties)
	if err != nil {
		return nil, err
	}

	updateRepo := &github.Repository{
		Name:                github.Ptr(props.Name),
		Description:         github.Ptr(props.Description),
		Homepage:            github.Ptr(props.Homepage),
		Visibility:          github.Ptr(props.Visibility),
		HasIssues:           props.HasIssues,
		HasWiki:             props.HasWiki,
		HasProjects:         props.HasProjects,
		IsTemplate:          props.IsTemplate,
		AllowSquashMerge:    props.AllowSquashMerge,
		AllowMergeCommit:    props.AllowMergeCommit,
		AllowRebaseMerge:    props.AllowRebaseMerge,
		AllowAutoMerge:      props.AllowAutoMerge,
		DeleteBranchOnMerge: props.DeleteBranchOnMerge,
		Archived:            props.Archived,
	}
	if props.DefaultBranch != "" {
		updateRepo.DefaultBranch = github.Ptr(props.DefaultBranch)
	}

	repo, _, err := gh.Repositories.Edit(ctx, owner, name, updateRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to update repository: %w", err)
	}

	// Update topics if specified.
	if props.Topics != nil {
		_, _, err = gh.Repositories.ReplaceAllTopics(ctx, owner, name, props.Topics)
		if err != nil {
			return nil, fmt.Errorf("failed to update topics: %w", err)
		}
		repo.Topics = props.Topics
	}

	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationUpdate,
			OperationStatus: resource.OperationStatusSuccess,
			NativeID:           repo.GetFullName(),
			ResourceProperties: repoToJSON(repo),
		},
	}, nil
}

func (r *Repository) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := r.client()
	if err != nil {
		return nil, err
	}

	owner, name, err := splitOwnerRepo(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, err := gh.Repositories.Delete(ctx, owner, name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			// Already deleted — treat as success.
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete repository: %w", err)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (r *Repository) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	// Repository operations are synchronous.
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (r *Repository) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := r.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization

	var allRepos []string
	page := 1
	if req.PageToken != nil {
		if _, err := fmt.Sscanf(*req.PageToken, "%d", &page); err != nil {
			page = 1
		}
	}

	opts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: 100},
	}

	if org != "" {
		repos, resp, err := gh.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}
		for _, repo := range repos {
			allRepos = append(allRepos, repo.GetFullName())
		}
		var nextToken *string
		if resp.NextPage != 0 {
			t := fmt.Sprintf("%d", resp.NextPage)
			nextToken = &t
		}
		return &resource.ListResult{
			NativeIDs:     allRepos,
			NextPageToken: nextToken,
		}, nil
	}

	// List for authenticated user.
	userOpts := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{Page: page, PerPage: 100},
	}
	repos, resp, err := gh.Repositories.ListByAuthenticatedUser(ctx, userOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	for _, repo := range repos {
		allRepos = append(allRepos, repo.GetFullName())
	}
	var nextToken *string
	if resp.NextPage != 0 {
		t := fmt.Sprintf("%d", resp.NextPage)
		nextToken = &t
	}
	return &resource.ListResult{
		NativeIDs:     allRepos,
		NextPageToken: nextToken,
	}, nil
}
