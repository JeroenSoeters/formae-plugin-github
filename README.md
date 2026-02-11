# GitHub Plugin for Formae

Manage GitHub repositories, teams, branch protection, and CI/CD configuration as infrastructure-as-code.

## Installation

```bash
make install
```

## Credentials

Set the `GITHUB_TOKEN` environment variable with a Personal Access Token.

**Fine-grained PAT** (recommended) with these repository permissions:
- **Administration**: Read and write (repositories)
- **Actions**: Read and write (workflow runs, dispatches)
- **Variables**: Read and write
- **Secrets**: Read and write
- **Contents**: Read
- **Members**: Read and write (teams, memberships)

**Classic PAT** with `repo`, `admin:org` scopes also works.

For GitHub Enterprise, set `apiUrl` in the target config:

```pkl
config = new github.Config {
  apiUrl = "https://github.example.com/api/v3"
}
```

## Supported Resources

| Resource Type | Description |
|---|---|
| `GitHub::Repos::Repository` | Repositories with settings, topics, merge strategies |
| `GitHub::Repos::BranchProtection` | Branch protection rules with status checks and review requirements |
| `GitHub::Teams::Team` | Organization teams |
| `GitHub::Teams::Membership` | Team memberships (add users to teams with roles) |
| `GitHub::Teams::RepositoryAccess` | Team repository permissions (pull, push, admin, etc.) |
| `GitHub::Actions::Variable` | Repository Actions variables |
| `GitHub::Actions::Secret` | Repository Actions secrets (write-only, NaCl encrypted) |
| `GitHub::Actions::WorkflowRun` | Dispatch and track workflow runs (async) |

## Configuration

```pkl
import "@github/github.pkl"

new formae.Target {
  label = "github"
  config = new github.Config {}
}
```

The `Config` class accepts:
| Field | Default | Description |
|---|---|---|
| `apiUrl` | `https://api.github.com` | GitHub API base URL |
| `organization` | (none) | Default org for discovery/listing |

## Usage

```pkl
import "@github/repos/repository.pkl"
import "@github/actions/variable.pkl"

new repository.Repository {
  label = "my-service"
  owner = "my-org"
  name = "my-service"
  description = "My microservice"
  visibility = "private"
  deleteBranchOnMerge = true
}

new variable.Variable {
  label = "deploy-env"
  owner = "my-org"
  repo = "my-service"
  name = "DEPLOY_ENVIRONMENT"
  value = "staging"
}
```

See [examples/basic/main.pkl](examples/basic/main.pkl) for a complete example managing a repository with branch protection, teams, and CI/CD configuration.

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)
- GitHub PAT for testing

### Building

```bash
make build      # Build plugin binary
make test       # Run all tests
make lint       # Run linter
make install    # Build + install locally
```

### Testing

```bash
# Unit/integration tests (mock HTTP server, no GitHub access needed)
make test-integration

# Conformance tests (requires GitHub PAT + test repo)
export GITHUB_TOKEN=ghp_...
export GITHUB_TEST_OWNER=my-org
export GITHUB_TEST_REPO=my-test-repo
make conformance-test
```

The conformance tests validate the full CRUD lifecycle through the formae agent for Variable and WorkflowRun resources. The `scripts/ci/clean-environment.sh` script creates the test repo (if needed) and cleans up test resources.

## Licensing

Apache-2.0
