default:
    @just --list

repo := "cashapp/hermit"

# Create a new patch release (e.g. v0.49.1 -> v0.49.2)
release: (_release "patch")

# Create a new minor release (e.g. v0.49.1 -> v0.50.0)
release-minor: (_release "minor")

_release bump:
    #!/usr/bin/env bash
    set -euo pipefail
    git fetch --tags --quiet
    latest=$(git tag --sort=-v:refname | head -1)
    major=$(echo "$latest" | sed 's/^v//' | cut -d. -f1)
    minor=$(echo "$latest" | sed 's/^v//' | cut -d. -f2)
    patch=$(echo "$latest" | sed 's/^v//' | cut -d. -f3)
    if [ "{{bump}}" = "minor" ]; then
        next="v${major}.$((minor + 1)).0"
    else
        next="v${major}.${minor}.$((patch + 1))"
    fi
    echo "Latest release: $latest"
    echo "{{bump}} release:  $next"
    if [ "{{bump}}" = "patch" ]; then
        echo "(Run 'just release-minor' to bump the minor version instead)"
    fi
    read -p "Proceed? [y/N] " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "Aborted."
        exit 1
    fi
    gh release create "$next" \
        --repo "{{repo}}" \
        --target master \
        --generate-notes

# Check the status of the most recent release workflow run
release-status:
    gh run list \
        --repo "{{repo}}" \
        --workflow release.yml \
        --limit 5

# Watch the latest release workflow run
release-watch:
    gh run watch \
        --repo "{{repo}}" \
        $(gh run list --repo "{{repo}}" --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')

# List recent releases
release-list:
    gh release list --repo "{{repo}}" --limit 10
