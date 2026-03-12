## Chart Overview
Chart name: `alluredeck`, apiVersion: v2, type: application.
Located at `charts/alluredeck/`.

## Chart Structure
```
charts/alluredeck/
  Chart.yaml              # chart metadata (name, version, appVersion)
  values.yaml             # default configuration values
  .helmignore
  templates/
    _helpers.tpl          # shared template helpers
    ingress.yaml          # unified path-based Ingress (API + UI + Swagger)
    networkpolicy.yaml    # optional NetworkPolicy
    extra-resources.yaml  # arbitrary extra K8s resources via extraResources[]
    NOTES.txt
    api/
      deployment.yaml
      service.yaml
      configmap.yaml      # non-sensitive config (mounted as config.yaml)
      secret.yaml         # credentials (or reference existingSecret)
      serviceaccount.yaml
      pvc.yaml            # persistence volumes (projects + database)
    ui/
      deployment.yaml
      service.yaml
      configmap.yaml
      serviceaccount.yaml
  tests/
    test-api-connection.yaml
    test-ui-connection.yaml
```

## values.yaml Sections
- **api**: `image`, `config` (logLevel, storageType, CORS, upload limits, goMemLimit), `s3` (endpoint, bucket, region, credentials), `security` (users, passwords, JWT keys), `persistence` (projects PVC + database PVC), `resources`, `probes`, `kind` (Deployment or StatefulSet)
- **ui**: `image`, `config` (apiUrl, appTitle), `resources`, `probes`
- **ingress**: path-based routing — `api.path: /api`, `ui.path: /`, optional `swagger.path: /swagger`
- **networkPolicy**: optional allow rules for ingress controller pods
- **extraResources**: arbitrary K8s manifests injected alongside the chart

## Helm Conventions
- Path-based Ingress: UI at `/`, API at `/api`, optional Swagger at `/swagger`
- Non-root security contexts: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, drop all capabilities
- PVCs: `api.persistence.projects` (local file storage) + `api.persistence.database` (DB data)
- Use `api.kind: StatefulSet` for stable PVC binding with local storage; `Deployment` for S3 or dev
- Credentials auto-generated (random) if not provided; use `existingSecret` to reference pre-created secrets
- IRSA on EKS: set `api.serviceAccount.annotations` with role ARN; omit static S3 credentials

## Commands
```
make helm-lint        # lint the chart
make helm-template    # render templates (validate rendering)
make helm-package     # package chart into .tgz
make helm-release     # bump chart version (BUMP=patch|minor|major) and commit
```
