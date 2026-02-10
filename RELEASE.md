# Release Process

The release process for Hermit is currently managed internally by Block engineers. 
The following documents the steps taken to create a new release.

## Prerequisites

Ensure you have activated the Hermit environment so that `just` and `gh` are available.

## Creating a New Release

1. Run `just release` for a patch release, or `just release-minor` for a minor release
2. Confirm the version when prompted
3. Check that the release workflow is successful with `just release-status` or `just release-watch`

You can view recent releases with `just release-list`.

## Rolling Back a Release

1. Revert the problematic change and merge the change to master
2. Follow the Creating a New Release process
