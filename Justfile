repo := "cashapp/hermit"
root := justfile_directory()
build_dir := root / "build"
version := `git describe --tags --dirty --always`
goos := `go env GOOS`
goarch := `go env GOARCH`
export HERMIT_EXE := ""

_help:
    @just --list

# Run lint, test, and build
all: lint test build

# Run golangci-lint
lint:
    golangci-lint run

# Run tests
test:
    go test ./...

# Run integration tests
test-integration:
    go test -count=1 -tags integration -v ./integration

# Build binary and gzip it
build channel="canary" os=goos arch=goarch: (build-binary channel os arch)
    gzip -f9 {{ build_dir / ("hermit-" + os + "-" + arch) }}

# Build binary
build-binary channel="canary" os=goos arch=goarch:
    mkdir -p {{ build_dir }}
    CGO_ENABLED=0 GOOS={{ os }} GOARCH={{ arch }} go build -ldflags "-X main.version={{ version }} -X main.channel={{ channel }}" -o {{ build_dir / ("hermit-" + os + "-" + arch) }} {{ root / "cmd/hermit" }}

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
    if [ "{{ bump }}" = "minor" ]; then
        next="v${major}.$((minor + 1)).0"
    else
        next="v${major}.${minor}.$((patch + 1))"
    fi
    echo "Latest release: $latest"
    echo "{{ bump }} release:  $next"
    if [ "{{ bump }}" = "patch" ]; then
        echo "(Run 'just release-minor' to bump the minor version instead)"
    fi
    read -p "Proceed? [y/N] " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "Aborted."
        exit 1
    fi
    gh release create "$next" \
        --repo "{{ repo }}" \
        --target master \
        --generate-notes

# Check the status of the most recent release workflow run
release-status:
    gh run list \
        --repo "{{ repo }}" \
        --workflow release.yml \
        --limit 5

# Watch the latest release workflow run
release-watch:
    gh run watch \
        --repo "{{ repo }}" \
        $(gh run list --repo "{{ repo }}" --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')

# List recent releases
release-list:
    gh release list --repo "{{ repo }}" --limit 10

# Generate docs schema
docs-schema:
    go run ./cmd/gendocs ./docs/docs/packaging/schema/
    go run ./cmd/hermit dump-user-config-schema | sed 's,//,#,g' > docs/docs/usage/user-config-schema.hcl
