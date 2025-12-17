# Release Command

Release a new version of jubako.

## Usage

```
/release <version>
```

Example: `/release 0.3.0`

## Process

Execute the following release process:

1. Update `version.txt` to the specified version (without 'v' prefix)
2. Run `make update-version` to update all submodule go.mod files
3. Stage and commit the version changes with message: `chore(release): bump version to v<version>`
4. Run `make release` to create and push tags
5. Push the commit to origin main

## Prerequisites

- All tests must pass before release
- Working directory must be clean (no uncommitted changes except version files)
- Must be on the main branch

## Arguments

- `$ARGUMENTS`: The version number to release (e.g., "0.3.0")