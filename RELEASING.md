# Releasing

## Application Release (automatic)

Push to `main` with a [Conventional Commit](https://www.conventionalcommits.org/) message:

```bash
git commit -m "feat: add new feature"
git push origin main
```

This triggers:
1. **semantic-release** determines the next version from commit messages
2. Docker images (`alluredeck-api`, `alluredeck-ui`) built and pushed to GHCR
3. Helm chart version + appVersion synced to the new version and released to GitHub Pages

## Helm Chart Release (manual)

For chart-only changes (templates, values, defaults):

```bash
# Edit chart files
vim charts/alluredeck/...

# Bump chart version and commit
make helm-release              # patch: 0.1.0 → 0.1.1
make helm-release BUMP=minor   # minor: 0.1.0 → 0.2.0
make helm-release BUMP=major   # major: 0.1.0 → 1.0.0

git push origin main
```

The `release-chart.yml` workflow detects the change and publishes the new chart version. `appVersion` stays at the last application release.

## Installing the Chart

```bash
helm repo add alluredeck https://mkutlak.github.io/alluredeck
helm repo update
helm install my-alluredeck alluredeck/alluredeck
```

## Versioning

- **Application version**: Determined by semantic-release from commit history
- **Chart version**: Equals app version on app releases; independent on chart-only releases
- **Docker image tags**: `X.Y.Z`, `X.Y`, `X`, `latest`, `sha-<short>`

## Prerequisites

- GitHub Pages enabled on `gh-pages` branch (Settings → Pages)
- `GITHUB_TOKEN` has `contents: write` and `pages: write` permissions
