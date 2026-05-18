#!/usr/bin/env bash
# Bump the Helm chart version in charts/alluredeck/Chart.yaml.
# Usage: helm-release.sh [patch|minor|major]   (default: patch)
set -euo pipefail

BUMP="${1:-patch}"
CHART_FILE="charts/alluredeck/Chart.yaml"

CURRENT=$(yq '.version' "$CHART_FILE")
IFS='.' read -r MAJOR MINOR PATCH <<<"$CURRENT"

case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
  *) echo "Invalid bump '$BUMP'. Use patch, minor, or major." >&2; exit 1 ;;
esac

NEW="$MAJOR.$MINOR.$PATCH"
yq -i ".version = \"$NEW\"" "$CHART_FILE"
echo "Chart version: $CURRENT -> $NEW"
echo "Run: git add $CHART_FILE && git commit -m \"chore(helm): bump chart version to $NEW\""
