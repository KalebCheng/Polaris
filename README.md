# Polaris

**English** | [简体中文](README.zh-CN.md)

Polaris is a modular IAM and OIDC identity provider. It uses institutions as
tenant boundaries and manages users, departments, job titles, roles,
permissions, applications, providers, certificates, tokens, sessions, audit
records, and identity connections.

Polaris can act as an OIDC Provider and as an OIDC/SAML-free SP-side connector
for external identity sources. The product scope is intentionally narrower
than Casdoor: commercial billing, Forms, Tickets, SAML/CAS, WebAuthn,
federated credentials, and custom HTML/CSS injection are out of scope.

## Capabilities

| Domain | Capabilities |
| --- | --- |
| Identity directory | Institutions, users, departments, job titles, job levels, external identities |
| Access control | Nested roles, allow/deny permissions, wildcard resources/actions, authorization diagnostics |
| Applications | OAuth2/OIDC clients, sign-in methods, signup policy, session policy, token policy, branding, redirects |
| Providers | OAuth/OIDC, email, SMS, captcha, storage, and MFA providers managed at runtime |
| IdP endpoints | Discovery, JWKS, authorize, token, userinfo, introspection, logout |
| Tokens | Authorization code + PKCE, refresh-token rotation, password, client credentials |
| Security | Password policy/history, Redis sessions, TOTP and SMS MFA, login throttling, CSRF, captcha, IP policy |
| Operations | Audit trail, task center, token cleanup, session revocation, identity sync jobs |

## Stack

- Backend: Go 1.26+, Gin, PostgreSQL 16+, pgx v5, Redis 7+, Zap, OpenTelemetry
- Frontend: React 19, TypeScript strict, Vite 8, Tailwind CSS v4, Radix UI, React Router, i18next, Vitest

## Quick Start

### 1. Start dependencies

Copy `.env.example` to `.env` and set strong, explicit values for
`POLARIS_DB_PASSWORD`, `POLARIS_REDIS_PASSWORD`,
`POLARIS_ADMIN_PASSWORD`, `POLARIS_JWT_SECRET`, and
`POLARIS_CREDENTIAL_ENC_KEY`.

```bash
cp .env.example .env
docker compose up -d
```

For local development, start PostgreSQL and Redis directly:

```bash
docker run -d --name polaris-dev-postgres -p 5432:5432 \
  -e POSTGRES_USER=polaris \
  -e POSTGRES_PASSWORD="$POLARIS_DB_PASSWORD" \
  -e POSTGRES_DB=polaris postgres:16-alpine

docker run -d --name polaris-dev-redis -p 6379:6379 \
  redis:7-alpine --requirepass "$POLARIS_REDIS_PASSWORD"
```

### 2. Start the backend

```bash
export POLARIS_DB_PASSWORD='replace-with-strong-password'
export POLARIS_REDIS_PASSWORD='replace-with-strong-password'
export POLARIS_ADMIN_PASSWORD='replace-with-strong-admin-password'
export POLARIS_JWT_SECRET='replace-with-random-jwt-secret'
make dev
```

The backend listens on `:8100`. On first startup it applies
`migrations/001_init.up.sql` and, when seeding is enabled, creates the
`built-in` institution and `admin` user.

```bash
make test
make migrate-up
./scripts/smoke-test.sh
```

### 3. Start the frontend

```bash
cd frontend/web
pnpm install
pnpm dev
```

The frontend runs at `http://localhost:5173` and proxies `/api`, `/oauth`,
`/.well-known`, and `/static` to the backend.

## Configuration

`configs/app.yaml` contains placeholders only. Real secrets are injected with
`POLARIS_*` environment variables. Important variables include:

| Variable | Purpose |
| --- | --- |
| `POLARIS_DB_*` | PostgreSQL connection; password is required |
| `POLARIS_REDIS_*` | Redis connection; password is required when authentication is enabled |
| `POLARIS_JWT_SECRET` | JWT signing secret; required |
| `POLARIS_CREDENTIAL_ENC_KEY` | Base64-encoded 32-byte secret used by AES-GCM; required in production |
| `POLARIS_ADMIN_PASSWORD` | Required when seeding is enabled |
| `POLARIS_ISSUER` | Public OIDC issuer URL |
| `POLARIS_OAUTH_*` | SSO callback, base URL, and provider secrets |
| `POLARIS_COOKIE_SECURE` | Must be `true` in production |
| `POLARIS_CSRF_ALLOWED_ORIGINS` | Trusted browser origins for cookie-backed writes |

See [deployment documentation](docs/deployment.md) for the complete list.

## Database Baseline

The repository keeps one migration baseline:

- `migrations/001_init.up.sql` creates the current schema.
- `migrations/001_init.down.sql` removes application tables while preserving
  the migrator-owned `schema_migrations` table.

There is no in-place upgrade path for databases created from the removed
historical migration files. Existing deployments must be reset or migrated
with an explicit, reviewed data migration before switching to this baseline.

## API and Integration

- OIDC discovery: `/.well-known/openid-configuration`
- JWKS: `/.well-known/jwks`
- Token endpoint: `/oauth/token`
- User information: `/oauth/userinfo`
- Management API: `/api/v1`

Response shape:

```json
{ "code": "OK", "message": "ok", "data": {} }
```

See [OIDC integration](docs/oidc-integration.md) and the
[API reference](docs/api-reference.md).

## Security Rules

- JWTs contain only a session ID.
- Password hashes, MFA secrets, recovery codes, private keys, client secrets,
  and token plaintext are never serialized to clients.
- Permission, role, job-title, and user-status changes invalidate affected
  sessions.
- Provider and connector credentials are encrypted with AES-256-GCM.
- Code comments are English. Comments explain constraints, intent, or
  non-obvious behavior; redundant comments are removed.

## Repository Layout

```text
cmd/polaris/        application entry point
internal/app/       dependency composition and seed data
internal/auth/      session, JWT, password, TOTP, OAuth, OIDC
internal/handler/   HTTP handlers and response DTOs
internal/repository entity models and PostgreSQL/Redis repositories
internal/server/    routes and middleware
internal/service/   domain services, syncers, provisioners, providers, asyncd
pkg/                database, Redis, and migrator clients
migrations/         current schema baseline
frontend/web/       React administration console
docs/               architecture, API, deployment, and design documents
```

## License

Internal project. No public license is declared.
