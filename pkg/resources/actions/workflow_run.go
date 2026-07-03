package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

const WorkflowRunResourceType = "GitHub::Actions::WorkflowRun"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationCheckStatus,
		resource.OperationList,
	}
	registry.Register(WorkflowRunResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &WorkflowRun{cfg: cfg}
	})
}

type WorkflowRun struct {
	cfg *config.Config
}

func (w *WorkflowRun) client() (*github.Client, error) {
	token, err := w.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if w.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(w.cfg.BaseURL(), w.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type workflowRunProperties struct {
	Repository   string                 `json:"repository"`
	Workflow     string                 `json:"workflow"`
	Ref          string                 `json:"ref"`
	Inputs       map[string]interface{} `json:"inputs,omitempty"`
	ArtifactName string                 `json:"artifactName,omitempty"`
	// Read-only
	RunID               int64  `json:"runId,omitempty"`
	Status              string `json:"status,omitempty"`
	Conclusion          string `json:"conclusion,omitempty"`
	HtmlUrl             string `json:"htmlUrl,omitempty"`
	ArtifactID          int64  `json:"artifactId,omitempty"`
	ArtifactDownloadUrl string `json:"artifactDownloadUrl,omitempty"`
	ArtifactSizeInBytes int64  `json:"artifactSizeInBytes,omitempty"`
}

// NativeID format: "owner/repo/runId"
func workflowRunNativeID(repoFullName string, runID int64) string {
	return repoFullName + "/" + strconv.FormatInt(runID, 10)
}

func parseWorkflowRunNativeID(nativeID string) (owner, repo string, runID int64, err error) {
	parts := strings.SplitN(nativeID, "/", 3)
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid workflow run ID %q, expected owner/repo/runId", nativeID)
	}
	runID, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid run ID in %q: %w", nativeID, err)
	}
	return parts[0], parts[1], runID, nil
}

func splitRepo(fullName string) (string, string, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository %q, expected owner/repo", fullName)
	}
	return parts[0], parts[1], nil
}

// workflowFileName extracts the filename from a workflow path.
// GitHub API returns ".github/workflows/deploy.yml" but users specify "deploy.yml".
func workflowFileName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// dispatchTokenPrefix marks a RequestID/NativeID that identifies a workflow that
// has been dispatched but whose run has not been discovered yet.
const dispatchTokenPrefix = "dispatch:"

// dispatchToken encodes the context needed to find a dispatched run later.
// Format: "dispatch:<owner>/<repo>:<workflow>:<ref>:<unixNano>".
//
// workflow_dispatch does not return a run ID, so Create/Update return this
// token immediately after dispatching and Status resolves the concrete run.
// This keeps Create/Update from blocking while GitHub registers the run, which
// would otherwise exceed the agent's plugin-operator call timeout.
func dispatchToken(owner, repo, workflow, ref string, dispatchTime time.Time) string {
	return fmt.Sprintf("%s%s/%s:%s:%s:%d", dispatchTokenPrefix, owner, repo, workflow, ref, dispatchTime.UnixNano())
}

// parseDispatchToken decodes a dispatch token. ok is false if s is not one.
func parseDispatchToken(s string) (owner, repo, workflow, ref string, dispatchTime time.Time, ok bool) {
	if !strings.HasPrefix(s, dispatchTokenPrefix) {
		return "", "", "", "", time.Time{}, false
	}
	parts := strings.Split(strings.TrimPrefix(s, dispatchTokenPrefix), ":")
	if len(parts) != 4 {
		return "", "", "", "", time.Time{}, false
	}
	repoParts := strings.SplitN(parts[0], "/", 2)
	if len(repoParts) != 2 {
		return "", "", "", "", time.Time{}, false
	}
	nano, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", "", "", "", time.Time{}, false
	}
	return repoParts[0], repoParts[1], parts[1], parts[2], time.Unix(0, nano), true
}

// findDispatchedRun looks for a workflow run triggered at or after dispatchTime.
// It makes a single API call (no polling) and returns nil if none is found yet.
func findDispatchedRun(ctx context.Context, gh *github.Client, owner, repo, workflow, ref string, dispatchTime time.Time) (*github.WorkflowRun, error) {
	runs, _, err := gh.Actions.ListWorkflowRunsByFileName(ctx, owner, repo, workflow, &github.ListWorkflowRunsOptions{
		Branch:      ref,
		Event:       "workflow_dispatch",
		ListOptions: github.ListOptions{PerPage: 5},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}
	for _, run := range runs.WorkflowRuns {
		if run.GetCreatedAt().After(dispatchTime.Add(-5 * time.Second)) {
			return run, nil
		}
	}
	return nil, nil
}

func (w *WorkflowRun) populateArtifact(ctx context.Context, gh *github.Client, owner, repo string, props *workflowRunProperties) {
	artifacts, _, err := gh.Actions.ListWorkflowRunArtifacts(ctx, owner, repo, props.RunID, &github.ListOptions{PerPage: 100})
	if err != nil || artifacts.GetTotalCount() == 0 {
		return
	}

	var selected *github.Artifact
	if props.ArtifactName != "" {
		for _, a := range artifacts.Artifacts {
			if a.GetName() == props.ArtifactName {
				selected = a
				break
			}
		}
	}
	if selected == nil && len(artifacts.Artifacts) > 0 {
		selected = artifacts.Artifacts[0]
	}

	if selected != nil {
		props.ArtifactID = selected.GetID()
		props.ArtifactDownloadUrl = selected.GetArchiveDownloadURL()
		props.ArtifactSizeInBytes = int64(selected.GetSizeInBytes())
	}
}

func (w *WorkflowRun) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := w.client()
	if err != nil {
		return nil, err
	}

	var props workflowRunProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	owner, repo, err := splitRepo(props.Repository)
	if err != nil {
		return nil, err
	}

	// Check for existing successful run with same workflow+ref (idempotency).
	existingRuns, _, listErr := gh.Actions.ListWorkflowRunsByFileName(ctx, owner, repo, props.Workflow, &github.ListWorkflowRunsOptions{
		Branch:      props.Ref,
		Event:       "workflow_dispatch",
		Status:      "success",
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if listErr == nil && existingRuns.GetTotalCount() > 0 {
		run := existingRuns.WorkflowRuns[0]
		props.RunID = run.GetID()
		props.Status = run.GetStatus()
		props.Conclusion = run.GetConclusion()
		props.HtmlUrl = run.GetHTMLURL()
		w.populateArtifact(ctx, gh, owner, repo, &props)
		b, _ := json.Marshal(props)

		return &resource.CreateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCreate,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           workflowRunNativeID(props.Repository, run.GetID()),
				ResourceProperties: b,
			},
		}, nil
	}

	// Dispatch the workflow and return immediately. Status() discovers the
	// resulting run and polls it to completion, so Create never blocks waiting
	// for GitHub to register the run.
	dispatchTime := time.Now()
	event := github.CreateWorkflowDispatchEventRequest{
		Ref:    props.Ref,
		Inputs: props.Inputs,
	}
	_, err = gh.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, props.Workflow, event)
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch workflow: %w", err)
	}

	token := dispatchToken(owner, repo, props.Workflow, props.Ref, dispatchTime)
	b, _ := json.Marshal(props)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusInProgress,
			RequestID:          token,
			NativeID:           token,
			ResourceProperties: b,
		},
	}, nil
}

func (w *WorkflowRun) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := w.client()
	if err != nil {
		return nil, err
	}

	owner, repo, runID, err := parseWorkflowRunNativeID(req.NativeID)
	if err != nil {
		return nil, err
	}

	run, resp, err := gh.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read workflow run: %w", err)
	}

	result := workflowRunProperties{
		Repository: owner + "/" + repo,
		Workflow:   workflowFileName(run.GetPath()),
		Ref:        run.GetHeadBranch(),
		RunID:      run.GetID(),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
		HtmlUrl:    run.GetHTMLURL(),
	}
	w.populateArtifact(ctx, gh, owner, repo, &result)
	b, _ := json.Marshal(result)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(b),
	}, nil
}

func (w *WorkflowRun) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := w.client()
	if err != nil {
		return nil, err
	}

	var desired workflowRunProperties
	if err := json.Unmarshal(req.DesiredProperties, &desired); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	var prior workflowRunProperties
	if req.PriorProperties != nil {
		_ = json.Unmarshal(req.PriorProperties, &prior)
	}

	// If ref changed, dispatch a new run.
	if desired.Ref != prior.Ref {
		owner, repo, err := splitRepo(desired.Repository)
		if err != nil {
			return nil, err
		}

		// Dispatch and return immediately; Status() resolves the new run.
		dispatchTime := time.Now()
		event := github.CreateWorkflowDispatchEventRequest{
			Ref:    desired.Ref,
			Inputs: desired.Inputs,
		}
		_, err = gh.Actions.CreateWorkflowDispatchEventByFileName(ctx, owner, repo, desired.Workflow, event)
		if err != nil {
			return nil, fmt.Errorf("failed to dispatch workflow: %w", err)
		}

		token := dispatchToken(owner, repo, desired.Workflow, desired.Ref, dispatchTime)
		b, _ := json.Marshal(desired)

		return &resource.UpdateResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationUpdate,
				OperationStatus:    resource.OperationStatusInProgress,
				RequestID:          token,
				NativeID:           token,
				ResourceProperties: b,
			},
		}, nil
	}

	// No change needed.
	b, _ := json.Marshal(prior)
	return &resource.UpdateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationUpdate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           req.NativeID,
			ResourceProperties: b,
		},
	}, nil
}

func (w *WorkflowRun) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := w.client()
	if err != nil {
		return nil, err
	}

	owner, repo, runID, err := parseWorkflowRunNativeID(req.NativeID)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: &resource.ProgressResult{
				Operation:       resource.OperationDelete,
				OperationStatus: resource.OperationStatusSuccess,
			},
		}, nil
	}

	resp, deleteErr := gh.Actions.DeleteWorkflowRun(ctx, owner, repo, runID)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete workflow run: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (w *WorkflowRun) Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	gh, err := w.client()
	if err != nil {
		return nil, err
	}

	// A dispatch token means the run has not been discovered yet. Find it with
	// a single API call; until it appears, stay InProgress with the same token.
	if owner, repo, workflow, ref, dispatchTime, ok := parseDispatchToken(req.RequestID); ok {
		run, err := findDispatchedRun(ctx, gh, owner, repo, workflow, ref, dispatchTime)
		if err != nil {
			return nil, err
		}
		if run == nil {
			props := workflowRunProperties{Repository: owner + "/" + repo, Workflow: workflow, Ref: ref}
			b, _ := json.Marshal(props)
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:          resource.OperationCheckStatus,
					OperationStatus:    resource.OperationStatusInProgress,
					RequestID:          req.RequestID,
					NativeID:           req.RequestID,
					ResourceProperties: b,
					StatusMessage:      "waiting for workflow run to appear",
				},
			}, nil
		}
		return w.statusForRun(ctx, gh, owner, repo, run.GetID())
	}

	// RequestID is the resolved NativeID: "owner/repo/runId".
	owner, repo, runID, err := parseWorkflowRunNativeID(req.RequestID)
	if err != nil {
		return nil, fmt.Errorf("invalid request ID: %w", err)
	}
	return w.statusForRun(ctx, gh, owner, repo, runID)
}

// statusForRun fetches a concrete run and maps its state to a progress result.
func (w *WorkflowRun) statusForRun(ctx context.Context, gh *github.Client, owner, repo string, runID int64) (*resource.StatusResult, error) {
	run, _, err := gh.Actions.GetWorkflowRunByID(ctx, owner, repo, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to check workflow run status: %w", err)
	}

	result := workflowRunProperties{
		Repository: owner + "/" + repo,
		Workflow:   workflowFileName(run.GetPath()),
		Ref:        run.GetHeadBranch(),
		RunID:      run.GetID(),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
		HtmlUrl:    run.GetHTMLURL(),
	}

	nativeID := workflowRunNativeID(owner+"/"+repo, runID)

	if run.GetStatus() == "completed" {
		w.populateArtifact(ctx, gh, owner, repo, &result)
		b, _ := json.Marshal(result)

		if run.GetConclusion() != "success" {
			return &resource.StatusResult{
				ProgressResult: &resource.ProgressResult{
					Operation:          resource.OperationCheckStatus,
					OperationStatus:    resource.OperationStatusFailure,
					NativeID:           nativeID,
					ResourceProperties: b,
					StatusMessage:      fmt.Sprintf("workflow run concluded with: %s", run.GetConclusion()),
				},
			}, nil
		}

		return &resource.StatusResult{
			ProgressResult: &resource.ProgressResult{
				Operation:          resource.OperationCheckStatus,
				OperationStatus:    resource.OperationStatusSuccess,
				NativeID:           nativeID,
				ResourceProperties: b,
			},
		}, nil
	}

	b, _ := json.Marshal(result)
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCheckStatus,
			OperationStatus:    resource.OperationStatusInProgress,
			RequestID:          nativeID,
			NativeID:           nativeID,
			ResourceProperties: b,
			StatusMessage:      fmt.Sprintf("workflow run status: %s", run.GetStatus()),
		},
	}, nil
}

func (w *WorkflowRun) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := w.client()
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
		runs, _, err := gh.Actions.ListRepositoryWorkflowRuns(ctx, org, repo.GetName(), &github.ListWorkflowRunsOptions{
			Event:       "workflow_dispatch",
			ListOptions: github.ListOptions{PerPage: 10},
		})
		if err != nil {
			continue
		}
		for _, run := range runs.WorkflowRuns {
			ids = append(ids, workflowRunNativeID(repo.GetFullName(), run.GetID()))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
