---
description: Create a new release by tagging and pushing to trigger the release workflow
allowed-tools:
  - Bash
  - Read
  - Glob
  - AskUserQuestion
---

Create a new release for coragent by tagging and pushing to trigger the GitHub Actions release workflow.

## Steps

### 1. Check current state

First, check the current git status and existing tags:

```bash
git status
git tag --list 'v*' --sort=-v:refname | head -10
git log --oneline -5
```

### 2. Determine the next version

Based on the latest tag and changes, suggest an appropriate next version following semantic versioning (x.y.z):
- **Major (x)**: Breaking changes
- **Minor (y)**: New features, backwards compatible
- **Patch (z)**: Bug fixes, backwards compatible

If no tags exist yet, suggest starting with `v0.1.0`.

### 3. Ask user for version confirmation

Use AskUserQuestion to confirm the version with the user. Present options based on the current version:
- If current is v1.2.3, suggest: v1.2.4 (patch), v1.3.0 (minor), v2.0.0 (major)
- Allow user to specify a custom version via "Other"

### 4. Verify the release

Before tagging, ensure:
- Working directory is clean (no uncommitted changes)
- All tests pass: `go test ./...`
- Build succeeds: `go build ./...`

If there are uncommitted changes, ask the user if they want to commit them first.

### 5. Create and push the tag

Once confirmed, create an annotated tag and push it:

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

### 6. Confirm success

After pushing the tag:
1. Show the GitHub Actions URL where the release workflow will run
2. Provide the link to view releases: `https://github.com/<owner>/<repo>/releases`

## Version Format

- Tags must be in format `vX.Y.Z` (e.g., `v1.0.0`, `v0.2.1`)
- The `v` prefix is required for the release workflow to trigger
