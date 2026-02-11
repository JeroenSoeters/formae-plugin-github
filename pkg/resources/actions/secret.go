package actions

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v69/github"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/prov"
	"github.com/platform-engineering-labs/formae-plugin-github/pkg/registry"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"golang.org/x/crypto/nacl/box"
)

const SecretResourceType = "GitHub::Actions::Secret"

func init() {
	ops := []resource.Operation{
		resource.OperationCreate,
		resource.OperationRead,
		resource.OperationUpdate,
		resource.OperationDelete,
		resource.OperationList,
	}
	registry.Register(SecretResourceType, ops, func(cfg *config.Config) prov.Provisioner {
		return &Secret{cfg: cfg}
	})
}

type Secret struct {
	cfg *config.Config
}

func (s *Secret) client() (*github.Client, error) {
	token, err := s.cfg.Token()
	if err != nil {
		return nil, err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	if s.cfg.BaseURL() != "https://api.github.com" {
		var parseErr error
		client, parseErr = client.WithEnterpriseURLs(s.cfg.BaseURL(), s.cfg.BaseURL())
		if parseErr != nil {
			return nil, fmt.Errorf("invalid API URL: %w", parseErr)
		}
	}
	return client, nil
}

type secretProperties struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	// Read-only
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func secretCompositeID(owner, repo, name string) string {
	return owner + "/" + repo + "/" + name
}

func parseSecretID(id string) (owner, repo, name string, err error) {
	parts := strings.SplitN(id, "/", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid secret ID %q, expected owner/repo/name", id)
	}
	return parts[0], parts[1], parts[2], nil
}

// encryptSecret encrypts a secret value using the repository's public key.
func encryptSecret(gh *github.Client, ctx context.Context, owner, repo, value string) (*github.EncryptedSecret, error) {
	pubKey, _, err := gh.Actions.GetRepoPublicKey(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo public key: %w", err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(pubKey.GetKey())
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	var recipientKey [32]byte
	copy(recipientKey[:], keyBytes)

	encrypted, err := box.SealAnonymous(nil, []byte(value), &recipientKey, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	return &github.EncryptedSecret{
		Name:           "",
		KeyID:          pubKey.GetKeyID(),
		EncryptedValue: base64.StdEncoding.EncodeToString(encrypted),
	}, nil
}

func (s *Secret) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	gh, err := s.client()
	if err != nil {
		return nil, err
	}

	var props secretProperties
	if err := json.Unmarshal(req.Properties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	encrypted, err := encryptSecret(gh, ctx, props.Owner, props.Repo, props.Value)
	if err != nil {
		return nil, err
	}
	encrypted.Name = props.Name

	_, err = gh.Actions.CreateOrUpdateRepoSecret(ctx, props.Owner, props.Repo, encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	result := secretProperties{
		Owner: props.Owner,
		Repo:  props.Repo,
		Name:  props.Name,
	}
	b, _ := json.Marshal(result)

	return &resource.CreateResult{
		ProgressResult: &resource.ProgressResult{
			Operation:          resource.OperationCreate,
			OperationStatus:    resource.OperationStatusSuccess,
			NativeID:           secretCompositeID(props.Owner, props.Repo, props.Name),
			ResourceProperties: b,
		},
	}, nil
}

func (s *Secret) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	gh, err := s.client()
	if err != nil {
		return nil, err
	}

	owner, repo, name, err := parseSecretID(req.NativeID)
	if err != nil {
		return nil, err
	}

	secret, resp, err := gh.Actions.GetRepoSecret(ctx, owner, repo, name)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.ReadResult{
				ResourceType: req.ResourceType,
				ErrorCode:    resource.OperationErrorCodeNotFound,
			}, nil
		}
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	result := secretProperties{
		Owner:     owner,
		Repo:      repo,
		Name:      secret.Name,
		CreatedAt: secret.CreatedAt.String(),
		UpdatedAt: secret.UpdatedAt.String(),
	}
	b, _ := json.Marshal(result)

	return &resource.ReadResult{
		ResourceType: req.ResourceType,
		Properties:   string(b),
	}, nil
}

func (s *Secret) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	gh, err := s.client()
	if err != nil {
		return nil, err
	}

	var props secretProperties
	if err := json.Unmarshal(req.DesiredProperties, &props); err != nil {
		return nil, fmt.Errorf("failed to parse properties: %w", err)
	}

	owner, repo, name, err := parseSecretID(req.NativeID)
	if err != nil {
		return nil, err
	}

	encrypted, err := encryptSecret(gh, ctx, owner, repo, props.Value)
	if err != nil {
		return nil, err
	}
	encrypted.Name = name

	_, err = gh.Actions.CreateOrUpdateRepoSecret(ctx, owner, repo, encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	result := secretProperties{
		Owner: owner,
		Repo:  repo,
		Name:  name,
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

func (s *Secret) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	gh, err := s.client()
	if err != nil {
		return nil, err
	}

	owner, repo, name, err := parseSecretID(req.NativeID)
	if err != nil {
		return nil, err
	}

	resp, deleteErr := gh.Actions.DeleteRepoSecret(ctx, owner, repo, name)
	if deleteErr != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return &resource.DeleteResult{
				ProgressResult: &resource.ProgressResult{
					Operation:       resource.OperationDelete,
					OperationStatus: resource.OperationStatusSuccess,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete secret: %w", deleteErr)
	}

	return &resource.DeleteResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationDelete,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (s *Secret) Status(_ context.Context, _ *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
		},
	}, nil
}

func (s *Secret) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	gh, err := s.client()
	if err != nil {
		return nil, err
	}

	cfg := config.FromTargetConfig(req.TargetConfig)
	org := cfg.Organization
	if org == "" {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	// List secrets for all repos in the org.
	repoOpts := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	repos, _, err := gh.Repositories.ListByOrg(ctx, org, repoOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	var ids []string
	for _, repo := range repos {
		secrets, _, err := gh.Actions.ListRepoSecrets(ctx, org, repo.GetName(), &github.ListOptions{PerPage: 100})
		if err != nil {
			continue
		}
		for _, secret := range secrets.Secrets {
			ids = append(ids, secretCompositeID(org, repo.GetName(), secret.Name))
		}
	}

	return &resource.ListResult{
		NativeIDs:     ids,
		NextPageToken: nil,
	}, nil
}
