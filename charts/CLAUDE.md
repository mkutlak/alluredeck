## Helm Conventions
- Path-based Ingress: API at `/api`, trace viewer at `/trace`, UI at `/`, optional Swagger at `/swagger`; `extraPaths` for custom rules
- Non-root security contexts: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, drop all capabilities
- PVCs: `api.persistence.projects` (local file storage) + `api.persistence.database` (DB data)
- Use `api.kind: StatefulSet` for stable PVC binding with local storage; `Deployment` for S3 or dev
- Credentials auto-generated (random) if not provided; use `existingSecret` to reference pre-created secrets
- IRSA on EKS: set `api.serviceAccount.annotations` with role ARN; omit static S3 credentials

## Release
`mise run helm:release` (patch|minor|major) bumps the chart version **and commits**.
