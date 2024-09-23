#!/bin/bash

# Function to bump version
bump_version() {
    local version=$1
    IFS='.' read -ra parts <<< "$version"
    local major=${parts[0]}
    local minor=${parts[1]}
    local patch=${parts[2]}

    # Bump patch version
    patch=$((patch + 1))

    echo "$major.$minor.$patch"
}

# Check for dry run flag
DRY_RUN=false
if [[ "$1" == "--dry-run" ]]; then
    DRY_RUN=true
    echo "Performing a dry run. No changes will be made."
fi

# Function to execute or simulate command based on dry run flag
execute_or_simulate() {
    if $DRY_RUN; then
        echo "Would execute: $*"
    else
        "$@"
    fi
}

# Get the latest tag
latest_tag=$(git describe --tags --abbrev=0 2>/dev/null)

if [ -z "$latest_tag" ]; then
    echo "No existing tags found. Starting with v0.0.1"
    new_tag="v0.0.1"
else
    # Remove 'v' prefix if it exists
    version=${latest_tag#v}

    # Bump the version
    new_version=$(bump_version "$version")
    new_tag="v$new_version"
fi

echo "Latest tag: $latest_tag"
echo "New tag: $new_tag"

# Update version in version.go
if $DRY_RUN; then
    echo "Would update version.go with: var Version = \"$new_tag\""
else
    # Cross-platform sed command
    sed -i.bak "s/var Version = \".*\"/var Version = \"$new_tag\"/" version.go && rm version.go.bak
fi

# Commit the change to version.go
execute_or_simulate git add version.go
execute_or_simulate git commit -m "Bump version to $new_tag"

# Create and push the new tag
execute_or_simulate git tag "$new_tag"
execute_or_simulate git push origin main "$new_tag"

if $DRY_RUN; then
    echo "Dry run complete. In a real run, version in version.go would be updated,"
    echo "changes would be committed, and new tag $new_tag would be created and pushed to GitHub."
else
    echo "Version in version.go updated, changes committed, and new tag $new_tag has been created and pushed to GitHub."
fi
