# Aveniq

Multi-tenant SaaS for psychology practices: patients with clinical history, agenda and
invoicing. Backend in Go under Clean Architecture, frontend in Next.js.

The product surface is in Spanish because its users are Argentine practices. Code,
comments, commit messages and this documentation are in English.

---

## Architecture in one page

```
Browser
  │  one origin, one cookie jar
  ▼
Caddy / Next.js dev proxy        /
  ├──────────────▶ web  (Next.js 16, App Router, RSC)
  │                    │  server-side calls only
  └──────────────▶ api  (Go, chi + huma)
                       │
                       ├─▶ PostgreSQL 17   (source of truth, RLS at E1)
                       └─▶ Redis           (asynq queue)
                              │
                              ▼
                           worker (same Go binary family)
```

### The two rules that keep this maintainable

1. **Next.js never touches PostgreSQL.** Every read and write goes through the Go API.
   Break this once and you lose centralised authorisation, auditing and row-level
   security at the same time.
2. **The OpenAPI document is the contract.** Go generates it (`huma`) and the TypeScript
   client is generated from it (`openapi-typescript`). No hand-written API types: drift
   becomes a build failure instead of a production bug.

### Layering (Clean Architecture)

Dependencies point inward. `domain` knows nothing; `infrastructure` and `interfaces`
know everything about `domain`.

```
Backend/
├── cmd/api, cmd/worker        composition roots — the only place that wires
├── internal/domain/           entities, invariants, domain errors (stdlib only)
├── internal/application/
│   ├── port/                  interfaces the use cases need (defined by the consumer)
│   └── tenant/                use cases: orchestration, no SQL, no HTTP
├── internal/infrastructure/   adapters: postgres (sqlc), redis
├── internal/interfaces/http/  delivery: huma handlers, middleware, error mapping
└── internal/platform/         config, logging, clock, id generation
```

`Frontend/` follows the same idea: `app/` only routes, `features/` own behaviour,
`entities/` own domain types, `shared/` holds cross-cutting pieces.

---

## Prerequisites

| Tool | Version | Why |
| --- | --- | --- |
| Go | 1.27+ | backend |
| Node.js | 22+ | frontend |
| pnpm | 12+ | frontend package manager |
| Docker + WSL2 | optional | canonical stack (`infra/docker-compose.yml`) |

The current machine has **no Docker and no WSL**, so the supported path is the hermetic
local stack: portable PostgreSQL and Redis inside `.local/`, no administrator rights, no
system-wide installs.

---

## Quickstart (no Docker)

```powershell
# 1. one-time: toolchain, portable database and queue
powershell -ExecutionPolicy Bypass -File scripts/bootstrap.ps1

# 2. database
cd Backend
goose -dir migrations postgres "postgres://aveniq:aveniq_dev@127.0.0.1:5433/aveniq?sslmode=disable" up

# 3. API (terminal 1)
go run ./cmd/api

# 4. web (terminal 2)
cd ../Frontend
pnpm dev
```

Then open <http://localhost:3000>. The API document is at
<http://127.0.0.1:8080/docs> and the raw spec at `/openapi.json`.

### Local service ports

| Service | Address |
| --- | --- |
| PostgreSQL | `127.0.0.1:5433` (non-default on purpose) |
| Redis | `127.0.0.1:6380` |
| API | `127.0.0.1:8080` |
| Web | `localhost:3000` |

`scripts/local-status.ps1` and `scripts/local-down.ps1` manage the stack.

---

## Configuration

`Backend/internal/platform/config/config.go` carries development defaults for every
variable, so a fresh clone runs with no `.env` file. Override through the environment:

| Variable | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `production` enables JSON logs and requires TLS on the DSN |
| `HTTP_ADDR` | `127.0.0.1:8080` | |
| `DATABASE_URL` | `postgres://aveniq:aveniq_dev@127.0.0.1:5433/aveniq?sslmode=disable` | |
| `REDIS_ADDR` | `127.0.0.1:6380` | |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `BASE_DOMAIN` | `localhost` | suffix used to resolve a tenant from the request host |
| `API_ORIGIN` (frontend) | `http://127.0.0.1:8080` | server-side only; never reaches the browser |

There is no committed `.env` file on purpose: the defaults are the local stack, and
secrets belong in the environment of whoever runs the process.

---

## Commands

### Backend

```bash
go test ./...                                                  # unit tests
go vet ./...                                                   # static checks
sqlc generate                                                  # regenerate typed queries
goose -dir migrations postgres "$DATABASE_URL" up              # migrate
goose -dir migrations postgres "$DATABASE_URL" create NAME sql  # new migration
air                                                            # live reload
```

### Frontend

```bash
pnpm dev           # development server
pnpm build         # production build
pnpm typecheck     # tsc --noEmit
pnpm api:types     # regenerate src/shared/api/schema.d.ts from the running API
```

`pnpm api:types` needs the API running. The generated file is committed so CI can
typecheck and build without booting a database.

---

## Troubleshooting

**Postgres says "ready to accept connections" but every client gets "server closed the
connection unexpectedly".**

The user profile path contains a non-ASCII character (`C:\Users\Gastón`). PostgreSQL forks
one backend per connection, and a backend launched with `TEMP`/`TMP` pointing inside such
a path dies with `STATUS_DLL_INIT_FAILED` (`0xC0000142`) while the postmaster itself is
healthy. `scripts/local-up.ps1` points `TEMP`, `TMP` and `PGTMPDIR` at `.local/tmp`
(ASCII) before starting the server. Do not remove that block.

**`pg_ctl start` hangs when its output is piped.**

The postgres child inherits the pipe and keeps it open. The script uses `Start-Process`
with file redirection; keep it that way.

**The frontend renders "La API no responde".**
The Go process is not running, or it is on a port other than 8080. Check
<http://127.0.0.1:8080/readyz>.

**Port already in use.**
`scripts/local-status.ps1` reports which process holds 5433 and 6380.

---

## Docker path

For machines with Docker Desktop:

```bash
docker compose -f infra/docker-compose.yml up --build
```

Same ports as the hermetic stack, so `DATABASE_URL` and `REDIS_ADDR` do not change.
Caddy then front-doors both services on `:8443`.

---

## Roadmap

`odd/tasks/consultorio-manager.md` holds the numbered specs and the task board. The
short version:

- **E0 (done)** foundations: repo, Clean Architecture skeleton, tenant vertical slice,
  local stack, CI.
- **E1** tenancy, IAM, permissions matrix and row-level security.
- **E2–E5** patients, clinical history, agenda, internal invoicing.
- **E6–E7** SaaS minimum (plans, limits, tenant export, audited support access) and MVP
  close.
- **v1** ARCA electronic invoicing, compliance, reporting, observability.
