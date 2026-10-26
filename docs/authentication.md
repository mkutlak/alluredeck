# Authentication

AllureDeck supports two authentication methods:

1. **Local authentication** — static admin/viewer credentials via environment variables
2. **OIDC SSO** — OpenID Connect with any compliant identity provider (Azure AD, Keycloak, Okta, Google Workspace, etc.)

Both methods can operate simultaneously. Local auth serves as a break-glass fallback when SSO is enabled.

## Roles

AllureDeck uses 3-level role-based access control:

| Role | Permissions |
|------|-------------|
| **admin** | Full access: create/delete projects, manage reports, manage users, system settings |
| **editor** | Upload results, generate reports, manage known issues, set default branches |
| **viewer** | Read-only access to all reports, dashboards, and analytics |

## Local Authentication

Configured via environment variables. Off by default: `SECURITY_ENABLED` is `false` in the API. In that mode requests are not authenticated and carry no role, but role checks still run, so every role-checked endpoint returns `403 Access denied: missing claims` until you enable it. The full-stack Docker Compose files and the Helm chart set it to `true`.

```bash
SECURITY_ENABLED=true
ADMIN_USER=admin
ADMIN_PASS=<strong-password>
VIEWER_USER=viewer
VIEWER_PASS=<strong-password>
JWT_SECRET_KEY=<64-char-random-string>
```

Passwords are bcrypt-hashed on startup. The `JWT_SECRET_KEY` is used to sign access and refresh tokens.

---

## OIDC SSO

### How It Works

AllureDeck uses the **Authorization Code + PKCE** flow. The entire OIDC exchange happens server-side — the frontend only provides an SSO button that redirects to the backend.

```
Browser                    AllureDeck API              Identity Provider
  │                              │                              │
  ├── click "Sign in with SSO" ──►                              │
  │                              ├── redirect with PKCE ────────►
  │                              │                              │
  │                              │◄──── authorization code ─────┤
  │                              │                              │
  │                              ├── exchange code + verifier ──►
  │                              │◄──── ID token ───────────────┤
  │                              │                              │
  │                              │  validate token, extract claims,
  │                              │  resolve role from groups,
  │                              │  JIT-provision user record,
  │                              │  issue AllureDeck JWT cookies
  │                              │                              │
  │◄── redirect with cookies ───┤                              │
```

Key security properties:
- **PKCE (S256)** on every authorization request
- **State parameter** encrypted with AES-GCM in an httpOnly cookie
- **Nonce** validated in the ID token to prevent replay attacks
- **Group claims** read from the signed ID token only (not the userinfo endpoint)
- **JIT provisioning** — user records are created on first login, updated on subsequent logins

### Configuration Reference

| Environment Variable | Required | Default | Description |
|---------------------|----------|---------|-------------|
| `OIDC_ENABLED` | No | `false` | Master toggle for SSO |
| `OIDC_ISSUER_URL` | Yes* | — | IdP discovery URL (must support `/.well-known/openid-configuration`) |
| `OIDC_CLIENT_ID` | Yes* | — | OAuth2 client ID registered with the IdP |
| `OIDC_CLIENT_SECRET` | Yes* | — | OAuth2 client secret (confidential client) |
| `OIDC_REDIRECT_URL` | Yes* | — | Callback URL: `https://<your-domain>/api/v1/auth/oidc/callback` |
| `OIDC_SCOPES` | No | `openid,profile,email` | Comma-separated OIDC scopes |
| `OIDC_GROUPS_CLAIM` | No | `groups` | JWT claim name containing group memberships |
| `OIDC_ADMIN_GROUPS` | No | — | Comma-separated group IDs that map to the `admin` role |
| `OIDC_EDITOR_GROUPS` | No | — | Comma-separated group IDs that map to the `editor` role |
| `OIDC_DEFAULT_ROLE` | No | `viewer` | Role assigned when no group matches |
| `OIDC_STATE_COOKIE_SECRET` | Yes* | — | AES encryption key for state cookies. Must be exactly 16, 24, or 32 bytes (AES-128/192/256); 32 recommended |
| `OIDC_POST_LOGIN_REDIRECT` | No | `/` | Frontend URL to redirect to after successful SSO login |
| `OIDC_END_SESSION_URL` | No | — | RP-initiated logout URL (optional) |
| `OIDC_AUTO_LINK_BY_EMAIL` | No | `false` | How to resolve a sign-in whose email already belongs to another account (see [Email Collisions](#email-collisions)) |

*Required when `OIDC_ENABLED=true`. The server refuses to start if any required field is missing.

### Role Mapping

Roles are resolved from IdP group claims using a highest-priority-wins strategy:

1. If any group in the user's token matches `OIDC_ADMIN_GROUPS` → **admin**
2. Else if any group matches `OIDC_EDITOR_GROUPS` → **editor**
3. Else → `OIDC_DEFAULT_ROLE` (default: **viewer**)

### Email Collisions

Email addresses are unique across providers. When an SSO sign-in presents an email that already belongs to an account created another way (for example a local account), the outcome depends on `OIDC_AUTO_LINK_BY_EMAIL`:

| Setting | Result |
|---------|--------|
| `false` (default) | Sign-in is rejected with `409 Conflict`. An administrator must untangle the duplicate identities |
| `true` | The existing account is rebound to the SSO identity **if** the IdP's ID token has `email_verified=true`, and the sign-in succeeds. An unverified email is still rejected with `409` |

Every rejection records an `auth.login.failure` audit event with `metadata.reason` set to `oidc_email_collision`, `oidc_email_collision_unverified`, `oidc_email_collision_lookup_failed`, or `oidc_email_collision_relink_failed` (the last two are internal failures while auto-linking); a successful link records `previous_provider` in the `auth.login.success` metadata. Enable auto-link only with an IdP that verifies email ownership — otherwise anyone who can register an arbitrary email there can take over the matching account. See [security.md](security.md#email-uniqueness-across-providers).

### User Lifecycle

- **First login:** A user record is created automatically (JIT provisioning) with the resolved role
- **Subsequent logins:** The user's name, email, and role are updated from the latest token claims
- **Deactivation:** An admin deactivates the user from **Administration → Users** (or `DELETE /api/v1/users/{id}`; see [user-management.md](user-management.md)). Access ends on the user's next request, within about 2 seconds: access tokens and API keys are refused and every refresh-token family — including SSO sessions — is revoked. The user receives `403 Forbidden` ("Account is deactivated") on the next SSO login even though IdP authentication succeeds. Reactivating the account lets the user sign in again, but API keys are not restored when deactivation went through `DELETE` (what the Users page uses) — they are deleted, and the user must create new ones

---

## Provider-Specific Examples

### Azure AD (Entra ID)

1. Register an application in Azure Portal → App registrations
2. Set the redirect URI to `https://alluredeck.example.com/api/v1/auth/oidc/callback` (Web platform)
3. Create a client secret under Certificates & secrets
4. Under Token configuration, add an optional `groups` claim (Security groups, type: Group ID)
5. Note your tenant ID

```bash
OIDC_ENABLED=true
OIDC_ISSUER_URL=https://login.microsoftonline.com/<tenant-id>/v2.0
OIDC_CLIENT_ID=<application-client-id>
OIDC_CLIENT_SECRET=<client-secret-value>
OIDC_REDIRECT_URL=https://alluredeck.example.com/api/v1/auth/oidc/callback
OIDC_GROUPS_CLAIM=groups
OIDC_ADMIN_GROUPS=<azure-ad-group-object-id-for-admins>
OIDC_EDITOR_GROUPS=<azure-ad-group-object-id-for-editors>
OIDC_STATE_COOKIE_SECRET=<32-byte-random-string>
```

> **Azure AD group overage:** When a user belongs to 200+ groups, Azure AD returns a `_claim_sources` reference instead of inline groups. AllureDeck detects this and assigns `OIDC_DEFAULT_ROLE`. To avoid this, use Azure AD group filtering or assign users to fewer groups.

### Keycloak

1. Create a client in your realm (Client type: OpenID Connect, Access type: confidential)
2. Set Valid redirect URIs to `https://alluredeck.example.com/api/v1/auth/oidc/callback`
3. Under Client scopes → add a mapper of type "Group Membership" with token claim name `groups`

```bash
OIDC_ENABLED=true
OIDC_ISSUER_URL=https://keycloak.example.com/realms/my-realm
OIDC_CLIENT_ID=alluredeck
OIDC_CLIENT_SECRET=<keycloak-client-secret>
OIDC_REDIRECT_URL=https://alluredeck.example.com/api/v1/auth/oidc/callback
OIDC_GROUPS_CLAIM=groups
OIDC_ADMIN_GROUPS=/alluredeck-admins
OIDC_EDITOR_GROUPS=/alluredeck-editors
OIDC_STATE_COOKIE_SECRET=<32-byte-random-string>
```

### Google Workspace

1. Create OAuth 2.0 credentials in Google Cloud Console → APIs & Services → Credentials
2. Set the authorized redirect URI to `https://alluredeck.example.com/api/v1/auth/oidc/callback`
3. Google does not provide group claims in the ID token by default. All users will receive `OIDC_DEFAULT_ROLE` unless you use Google Workspace directory groups with a custom claim mapper

```bash
OIDC_ENABLED=true
OIDC_ISSUER_URL=https://accounts.google.com
OIDC_CLIENT_ID=<google-client-id>.apps.googleusercontent.com
OIDC_CLIENT_SECRET=<google-client-secret>
OIDC_REDIRECT_URL=https://alluredeck.example.com/api/v1/auth/oidc/callback
OIDC_DEFAULT_ROLE=editor
OIDC_STATE_COOKIE_SECRET=<32-byte-random-string>
```

### Okta

1. Create an OIDC Web Application in Okta Admin Console
2. Set the sign-in redirect URI to `https://alluredeck.example.com/api/v1/auth/oidc/callback`
3. Under the application's Sign On → OpenID Connect ID Token, add a Groups claim filter

```bash
OIDC_ENABLED=true
OIDC_ISSUER_URL=https://<your-okta-domain>/oauth2/default
OIDC_CLIENT_ID=<okta-client-id>
OIDC_CLIENT_SECRET=<okta-client-secret>
OIDC_REDIRECT_URL=https://alluredeck.example.com/api/v1/auth/oidc/callback
OIDC_GROUPS_CLAIM=groups
OIDC_ADMIN_GROUPS=AllureDeck-Admins
OIDC_EDITOR_GROUPS=AllureDeck-Editors
OIDC_STATE_COOKIE_SECRET=<32-byte-random-string>
```

---

## Deployment Examples

### Docker Compose

Uncomment the OIDC variables in `docker/docker-compose.yml` and fill in your IdP details:

```yaml
services:
  allure-api:
    environment:
      # ... existing config ...
      OIDC_ENABLED: "true"
      OIDC_ISSUER_URL: "https://login.microsoftonline.com/<tenant>/v2.0"
      OIDC_CLIENT_ID: "<client-id>"
      OIDC_CLIENT_SECRET: "<client-secret>"
      OIDC_REDIRECT_URL: "http://localhost:5050/api/v1/auth/oidc/callback"
      OIDC_ADMIN_GROUPS: "<admin-group-id>"
      OIDC_EDITOR_GROUPS: "<editor-group-id>"
      OIDC_STATE_COOKIE_SECRET: "<32-byte-random-string>"
```

### Helm Chart

```yaml
# values-oidc.yaml
api:
  oidc:
    enabled: true
    issuerUrl: "https://login.microsoftonline.com/<tenant>/v2.0"
    clientId: "<client-id>"
    clientSecret: "<client-secret>"
    redirectUrl: "https://alluredeck.example.com/api/v1/auth/oidc/callback"
    adminGroups: "<admin-group-id>"
    editorGroups: "<editor-group-id>"
    stateCookieSecret: "<32-byte-random-string>"
```

```bash
helm upgrade --install alluredeck charts/alluredeck -f values-oidc.yaml
```

The chart stores `clientSecret` and `stateCookieSecret` in a Kubernetes Secret (never in the ConfigMap). If either is left empty, a random value is auto-generated on first install and preserved across upgrades.

To use a pre-created Secret instead:

```yaml
api:
  security:
    existingSecret: "my-alluredeck-secret"
```

The Secret must include `oidcClientSecret` and `oidcStateCookieSecret` keys alongside the existing auth keys.

The chart has no value for `OIDC_AUTO_LINK_BY_EMAIL`. Set it with `api.extraEnv`:

```yaml
api:
  extraEnv:
    - name: OIDC_AUTO_LINK_BY_EMAIL
      value: "true"
```

---

## Generating Secrets

Generate a 32-byte random string for `OIDC_STATE_COOKIE_SECRET`:

```bash
# OpenSSL
openssl rand -base64 32 | head -c 32

# Python
python3 -c "import secrets; print(secrets.token_urlsafe(24)[:32])"

# /dev/urandom
head -c 32 /dev/urandom | base64 | head -c 32
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Server refuses to start with `OIDC_ISSUER_URL is required` | OIDC enabled but required fields missing | Set all required env vars |
| Server refuses to start with `OIDC_STATE_COOKIE_SECRET must be exactly 16, 24, or 32 bytes` | Key length is wrong | Generate a key of exactly 32 bytes |
| `OIDC discovery failed` at startup | Cannot reach the issuer URL | Verify `OIDC_ISSUER_URL` is reachable from the API pod and returns `/.well-known/openid-configuration` |
| SSO login redirects but callback returns 400 | State mismatch or expired | Ensure `OIDC_REDIRECT_URL` exactly matches what's registered in the IdP; check clock sync between API server and IdP |
| User gets `viewer` role despite being in an admin group | Group ID mismatch | Compare the group claim value in the ID token against `OIDC_ADMIN_GROUPS`; Azure AD uses GUIDs, Keycloak uses paths like `/group-name` |
| Azure AD user gets `viewer` despite group membership | Group overage (200+ groups) | Reduce group count or use filtered group claims |
| User gets 403 after successful SSO auth | Account deactivated | Reactivate the user on the Users page, or check `is_active` in the `users` table |
| SSO login returns 409 "An account with this email already exists under a different identity" | Email collides with an existing account and `OIDC_AUTO_LINK_BY_EMAIL=false` (or the IdP did not assert `email_verified`) | Enable `OIDC_AUTO_LINK_BY_EMAIL` for a trusted IdP, or have an administrator resolve the duplicate account |
| SSO button not visible on login page | Config endpoint reports OIDC disabled | Verify `OIDC_ENABLED=true` and the API is reachable |
