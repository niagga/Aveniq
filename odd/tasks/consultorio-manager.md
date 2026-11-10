# Consultorio Manager — Aveniq

SaaS multi-tenant para consultorios de psicología: pacientes con historia clínica, agenda y
facturación. Backend Go bajo Clean Architecture, Frontend Next.js. Documento vivo: se actualiza
en cada transición de tarea.

## Specs

**S1 — Gestión de clientes: CRUD + historia clínica**
> "Gestion de clientes: CRUD + Historia Clínica"

**S2 — Gestión de contabilidad: CRUD facturación + histórico + integración de envío**
> "Gestión de contabilidad: CRUD Facturación + Historico de facturación + Integración con plataforma de envio de facturación"

**S3 — Gestión de usuarios del sistema: CRUD users + tipos de usuario**
> "Gestión de usuarios del sistema: CRUD Users + Tipo de users"

**S4 — Objetivo MVP primero, versión de producción después**
> "quiero que me des un plan completo de tareas y etapas para crear, poner un objetivo de un MVP primero y luego una versión para ir a producción"

**S5 — Multi-tenant completo, en modo SaaS**
> "Multi-tenant completo"
> "SaaS: otros consultorios contratan el servicio"

**S6 — Agenda básica incluida en el MVP**
> "Sí, agenda básica en el MVP"

**S7 — Facturación electrónica decidida más adelante; el MVP solo registra comprobantes internos**
> "Decidimos más adelante"

**S8 — Stack acordado**
> "TypeScript + Next.js + Go + PostgresSQL + Redis + Docker + Docker Compose"

**S9 — Clean Architecture**
> "Quiero que sumes a la configuración que quiero usar CleanArchitecture"

**S10 — Estructura física con Backend y Frontend como carpetas separadas**
> "que fisicamente quiero una estructura de carpetas con Backend y Frontend y dentro de ellos el codigo (Go y Next.js)"

**S11 — SOLID, patrones de diseño, clean code y buenas prácticas**
> "Quiero usar principios SOLID siempre que se pueda, usar patrones de programación, clean code y buenas practicas"

**S12 — pnpm como manejador de paquetes de Next.js**
> "Mi manejador de package de Next.js es pnpm"

**S13 — Todo lo necesario instalado para correr en local**
> "quiero que instales todo lo que sea necesario para la aplicación correr en local"

## Tasks

| ID | Spec | Etapa | Descripción | Ruta | Commit |
| --- | --- | --- | --- | --- | --- |
| T1 | S10, S12 | E0 | Repo propio `Aveniq` y estructura Backend/Frontend/infra/scripts | inline | — |
| T2 | S4, S9, S11 | E0 | Documento de feature con specs y tareas | inline | — |
| T3 | S13 | E0 | Toolchain Go local: sqlc, goose, air | inline | — |
| T4 | S13 | E0 | Postgres y Redis portables en `.local/` con scripts de ciclo de vida | inline | — |
| T5 | S9, S10, S11 | E0 | Esqueleto Clean Architecture con `cmd/api` y `cmd/worker` | inline | — |
| T6 | S13 | E0 | Migraciones goose, configuración sqlc y migración inicial `tenants` | inline | — |
| T7 | S9, S11, S5 | E0 | Vertical slice `tenants`: dominio → caso de uso → adaptador → handler + tests | inline | — |
| T8 | S10, S12 | E0 | Next.js con pnpm, Tailwind y estructura por features | inline | — |
| T9 | S11 | E0 | Cliente OpenAPI generado y página que consume la API Go | inline | — |
| T10 | S8 | E0 | `docker-compose` + Caddy para el camino con Docker | inline | — |
| T11 | S13, S11 | E0 | CI, scripts de operación, README/RUNBOOK y verificación end-to-end | inline | — |

## Log

**L1 — Pedido original (verbatim)**
> Vamos a comenzar un nuevo proyecto. Quiero que piense y me ayudes a pensarlo de la mejor manera.
> Quiero hacer un gestor de tareas de un consultorio de un Psicologo.
> Estas tareas pueden ser:
> - Gestion de clientes: CRUD + Historia Clínica
> - Gestión de contabilidad: CRUD Facturación + Historico de facturación + Integración con plataforma de envio de facturación
> - Gestión de usuarios del sistema: CRUD Users + Tipo de users
> Quiero que me sugieras el mejor stack para una aplicación de este tipo.
> Adicionalmente quiero que me des un plan completo de tareas y etapas para crear, poner un objetivo de un MVP primero y luego una versión para ir a producción

**L2 — Decisiones de arquitectura (respuestas del usuario)**
- "Multi-tenant completo"
- "Sí, agenda básica en el MVP"
- "Decidimos más adelante" (facturación electrónica)
- "TypeScript + Next.js + Go + PostgresSQL + Redis + Docker + Docker Compose"
- "SaaS: otros consultorios contratan el servicio"

**L3 — Pedido de estructura y entorno (verbatim)**
> Quiero que sumes a la configuración que quiero usar CleanArchitecture, que fisicamente quiero una
> estructura de carpetas con Backend y Frontend y dentro de ellos el codigo (Go y Next.js). Quiero usar
> principios SOLID siempre que se pueda, usar patrones de programación, clean code y buenas practicas.
> Mi manejador de package de Next.js es pnpm.
> Adicionalmente, quiero que instales todo lo que sea necesario para la aplicación correr en local.

**L4 — Hallazgo de entorno**
Docker y WSL no están instalados y no pueden instalarse sin elevación (UAC) y probable reinicio.
Decisión: entorno local **hermético sin admin** en `.local/` (Postgres y Redis portables, con scripts),
manteniendo `infra/docker-compose.yml` como camino canónico para cuando Docker esté disponible.

**L5 — E0 completado y verificado**

Evidencia observada, no declarada:

- `go test ./...` verde en `internal/domain/tenant` y `internal/application/tenant` (RED observado primero:
  `no non-test Go files`, luego GREEN).
- `go vet ./...` sin hallazgos; `go build ./...` limpio.
- `sqlc generate` produce `sqlcgen/` desde las migraciones goose; CI falla si queda desactualizado.
- Migración `0001_create_tenants.sql` aplicada con goose. Constraints verificadas en el catálogo de
  Postgres: `tenants_slug_unique`, `tenants_slug_canonical`, `tenants_slug_length`,
  `tenants_name_not_blank`, `tenants_status_known`.
- API en `127.0.0.1:8080`: `/healthz` 200, `/readyz` 200 (postgres + redis),
  `POST /api/v1/tenants` 201, mismo slug 409, slug inválido 422, `GET /api/v1/tenants` 200.
- OpenAPI servido en `/openapi.json` con paths completos (`/api/v1/tenants`) y schemas
  `TenantBody`, `ListTenantsOutputBody`, `ErrorModel`.
- Frontend: `pnpm typecheck` y `pnpm build` limpios. `http://localhost:3000` renderiza el consultorio
  creado por la API y el proxy mismo-origen `/api/v1/tenants` responde 200.

Hallazgo de entorno que costó tiempo y queda documentado en `README.md`:
Postgres arrancaba y decía "ready to accept connections", pero cada backend moría con
`STATUS_DLL_INIT_FAILED (0xC0000142)` porque `TEMP` apuntaba a `C:\\Users\\Gastón` (ruta no-ASCII).
Los scripts fijan `TEMP`/`TMP`/`PGTMPDIR` a `.local/tmp`.

Riesgo de revisión declarado: el commit inicial tiene ~5.5k líneas porque no existe historial previo
contra el cual trocear. Los cortes por unidad de review empiezan en E1.
