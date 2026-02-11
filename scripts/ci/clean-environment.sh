#!/bin/bash
# Clean Environment Hook for GitHub Plugin Conformance Tests
# ===========================================================
# Called before AND after conformance tests to clean up test resources.
#
# Required environment variables:
#   GITHUB_TOKEN       - GitHub PAT with repo and actions permissions
#   GITHUB_TEST_OWNER  - Repository owner (org or user)
#   GITHUB_TEST_REPO   - Repository name
#
# This script also ensures the test repository has the required workflow
# file for WorkflowRun conformance tests.

set -euo pipefail

OWNER="${GITHUB_TEST_OWNER:?GITHUB_TEST_OWNER must be set}"
REPO="${GITHUB_TEST_REPO:?GITHUB_TEST_REPO must be set}"
VAR_PREFIX="${TEST_PREFIX:-FORMAE_TEST_}"
WORKFLOW_FILE=".github/workflows/formae-test.yml"

echo "clean-environment.sh: Cleaning GitHub test resources"
echo "  Owner: ${OWNER}"
echo "  Repo: ${REPO}"
echo "  Variable prefix: ${VAR_PREFIX}"
echo ""

# ---------------------------------------------------------------------------
# 1. Ensure test repository exists
# ---------------------------------------------------------------------------
if ! gh repo view "${OWNER}/${REPO}" --json name >/dev/null 2>&1; then
    echo "Creating test repository ${OWNER}/${REPO}..."
    gh repo create "${OWNER}/${REPO}" --public --description "Formae GitHub plugin conformance test repo" --add-readme
    sleep 2
fi

# ---------------------------------------------------------------------------
# 2. Ensure workflow file exists for WorkflowRun tests
# ---------------------------------------------------------------------------
echo "Checking for workflow file..."
if ! gh api "repos/${OWNER}/${REPO}/contents/${WORKFLOW_FILE}" >/dev/null 2>&1; then
    echo "  Creating ${WORKFLOW_FILE}..."
    WORKFLOW_CONTENT=$(cat <<'YAML'
name: Formae Plugin Test
on:
  workflow_dispatch: {}
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Hello from formae conformance test"
YAML
)
    # Base64 encode and push via Contents API
    ENCODED=$(echo "${WORKFLOW_CONTENT}" | base64 -w 0 2>/dev/null || echo "${WORKFLOW_CONTENT}" | base64)
    gh api -X PUT "repos/${OWNER}/${REPO}/contents/${WORKFLOW_FILE}" \
        -f message="Add formae conformance test workflow" \
        -f content="${ENCODED}" >/dev/null
    echo "  Workflow file created"
    sleep 2
else
    echo "  Workflow file already exists"
fi

# ---------------------------------------------------------------------------
# 3. Clean up test variables
# ---------------------------------------------------------------------------
echo ""
echo "Cleaning Actions variables with prefix '${VAR_PREFIX}'..."
VARS=$(gh api "repos/${OWNER}/${REPO}/actions/variables" --jq ".variables[]? | select(.name | startswith(\"${VAR_PREFIX}\")) | .name" 2>/dev/null || true)
if [[ -n "${VARS}" ]]; then
    echo "${VARS}" | while read -r name; do
        echo "  Deleting variable: ${name}"
        gh api -X DELETE "repos/${OWNER}/${REPO}/actions/variables/${name}" 2>/dev/null || true
    done
else
    echo "  No test variables found"
fi

# ---------------------------------------------------------------------------
# 4. Clean up test secrets
# ---------------------------------------------------------------------------
echo ""
echo "Cleaning Actions secrets with prefix '${VAR_PREFIX}'..."
SECRETS=$(gh api "repos/${OWNER}/${REPO}/actions/secrets" --jq ".secrets[]? | select(.name | startswith(\"${VAR_PREFIX}\")) | .name" 2>/dev/null || true)
if [[ -n "${SECRETS}" ]]; then
    echo "${SECRETS}" | while read -r name; do
        echo "  Deleting secret: ${name}"
        gh api -X DELETE "repos/${OWNER}/${REPO}/actions/secrets/${name}" 2>/dev/null || true
    done
else
    echo "  No test secrets found"
fi

# ---------------------------------------------------------------------------
# 5. Cancel in-progress workflow runs and delete test runs
# ---------------------------------------------------------------------------
echo ""
echo "Cleaning workflow runs..."
# Cancel any in-progress runs for the test workflow
IN_PROGRESS=$(gh api "repos/${OWNER}/${REPO}/actions/workflows/formae-test.yml/runs?status=in_progress" --jq '.workflow_runs[]?.id' 2>/dev/null || true)
if [[ -n "${IN_PROGRESS}" ]]; then
    echo "${IN_PROGRESS}" | while read -r id; do
        echo "  Cancelling run: ${id}"
        gh api -X POST "repos/${OWNER}/${REPO}/actions/runs/${id}/cancel" 2>/dev/null || true
    done
fi

# Delete completed test workflow runs (keep last 5 to avoid rate limiting on large cleanup)
COMPLETED=$(gh api "repos/${OWNER}/${REPO}/actions/workflows/formae-test.yml/runs?event=workflow_dispatch&per_page=100" --jq '.workflow_runs[]?.id' 2>/dev/null || true)
if [[ -n "${COMPLETED}" ]]; then
    COUNT=0
    echo "${COMPLETED}" | while read -r id; do
        COUNT=$((COUNT + 1))
        if [[ ${COUNT} -gt 5 ]]; then
            break
        fi
        echo "  Deleting run: ${id}"
        gh api -X DELETE "repos/${OWNER}/${REPO}/actions/runs/${id}" 2>/dev/null || true
    done
else
    echo "  No test workflow runs found"
fi

# ---------------------------------------------------------------------------
# 6. Clean up test repositories (from Repository conformance tests)
# ---------------------------------------------------------------------------
echo ""
echo "Cleaning test repositories with prefix 'formae-test-'..."
TEST_REPOS=$(gh repo list "${OWNER}" --json name --jq '.[].name | select(startswith("formae-test-"))' 2>/dev/null || true)
if [[ -n "${TEST_REPOS}" ]]; then
    echo "${TEST_REPOS}" | while read -r repo_name; do
        echo "  Deleting repository: ${OWNER}/${repo_name}"
        gh repo delete "${OWNER}/${repo_name}" --yes 2>/dev/null || true
    done
else
    echo "  No test repositories found"
fi

# ---------------------------------------------------------------------------
# 7. Remove branch protection from test repo (from BranchProtection tests)
# ---------------------------------------------------------------------------
echo ""
echo "Removing branch protection from ${OWNER}/${REPO}/main..."
gh api -X DELETE "repos/${OWNER}/${REPO}/branches/main/protection" 2>/dev/null || echo "  No branch protection to remove"

echo ""
echo "clean-environment.sh: Cleanup complete"
