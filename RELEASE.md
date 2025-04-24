# Release Process

The release process for Hermit is currently managed internally by Block engineers. 
The following documents the steps taken to create a new release.

## Creating a New Release

1. Navigate to the [Create New Release](https://github.com/cashapp/hermit/releases/new) page on GitHub
2. Create a new tag that reflects the semantic changes made in this release
   - Follow semantic versioning principles with a 'v' prefix (e.g., `v0.44.7`)
   - Version numbers follow the format `vMAJOR.MINOR.PATCH`
   - Leave the Target branch set to `master`
3. Click "Generate release notes" to automatically compile changes from the commit history
   - This will create a changelog based on merged pull requests and commits
4. Review the generated release notes and make any necessary adjustments if required
5. Click "Publish Release" to create the release
   - This will trigger the release workflow
   - The workflow will handle building and publishing the release artifacts
6. Check that the [release workflow](https://github.com/cashapp/hermit/actions/workflows/release.yml) is successful

## Rolling Back a Release

1. Revert the problematic change and merge the change to master
2. Follow the Creating a New Release process