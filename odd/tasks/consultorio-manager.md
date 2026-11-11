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
| T12 | S5, S3 | E1 | Esquema IAM + RLS forzado + catálogo de roles y permisos + test de aislamiento en base | worker | pendiente |
| T13 | S3, S9 | E1 | Dominio IAM y repositorio sqlc | worker | `13bdc2d`, `970221d` |
| T14 | S3 | E1 | Auth: argon2id, login/refresh/logout con rotación y detección de reuso, middleware | worker | pendiente |
| T15 | S3, S11 | E1 | Matriz de permisos por dominio, invitaciones y alta de tenant en trial | worker | pendiente |
| T16 | S5 | E1 | Test de fuga cross-tenant en CI y tests de la capa HTTP | worker | pendiente |

E0 (T1–T11) publicado en el commit `06f745a`.

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

**L6 — Decisiones de diseño de E1**

El aislamiento multi-tenant se diseña en la base, no solo en el código. Cuatro decisiones que no son
negociables, porque cada una tiene una forma silenciosa de fallar:

1. **`FORCE ROW LEVEL SECURITY`.** En PostgreSQL el dueño de una tabla *ignora* sus políticas RLS. Como
   el rol de la aplicación es el dueño de las tablas en el entorno local, sin `FORCE` las políticas no se
   aplican y todo parece funcionar. Es el error más caro de esta etapa.
2. **Cerrar por defecto.** `current_setting('app.tenant_id', true)` devuelve NULL cuando no se seteó. La
   política debe usar `NULLIF(..., '')::uuid` y comparar contra `tenant_id`; una sesión sin contexto no ve
   **ninguna** fila. Nunca al revés.
3. **`WITH CHECK`, no solo `USING`.** `USING` filtra lecturas; sin `WITH CHECK` un bug podría *escribir*
   una fila en el tenant equivocado. Las dos cláusulas van juntas.
4. **El contexto se setea por transacción** con `SET LOCAL app.tenant_id`, nunca por conexión
   (`SET`), porque las conexiones vienen del pool y se reusan entre requests de tenants distintos.

Además: `roles` y `permissions` son catálogos **globales** (código fijo), no por tenant. Roles
personalizados por tenant quedan fuera de E1. El rol `admin` **no** recibe permisos `clinical:*`: el
administrador del sistema no lee historias clínicas. Ese es el punto entero de la matriz.

**L7 — E1/T12: IAM, roles y RLS (completado)**

**Hallazgo crítico.** Al verificar las políticas con datos reales, RLS no hacía nada: cada tenant veía
las filas del otro, y una sesión sin contexto también veía todo. Causa: el rol `aveniq` con el que se
inicializó el clúster local tiene `rolsuper = true` y `rolbypassrls = true`, y PostgreSQL deja que
ambos atributos ignoren RLS por completo. `FORCE ROW LEVEL SECURITY` **no** alcanza a un superusuario.
La migración 0002 era, en la práctica, decorativa: el código parecía correcto y el aislamiento no
existía. Si esto se descubre en producción, se descubre con datos clínicos de dos consultorios
mezclados.

**Corrección.** Migración 0003 crea `aveniq_app` (`NOSUPERUSER`, `NOBYPASSRLS`, dueño de nada), le
otorga privilegios sobre todas las tablas y `ALTER DEFAULT PRIVILEGES` para las futuras. La API y el
worker conectan con ese rol; las migraciones siguen con el dueño. Los tres entornos lo crean:
`scripts/local-up.ps1`, `infra/docker/init/01-app-role.sql` y el workflow de CI.

Evidencia observada, antes y después:

| Escenario | Con el rol dueño | Con `aveniq_app` |
| --- | --- | --- |
| `app.tenant_id` = A | 2 filas | 1 fila |
| `app.tenant_id` = B | 2 filas | 1 fila |
| Sin contexto | 2 filas | 0 filas |
| Contexto vacío | 2 filas | 0 filas |
| INSERT en tenant ajeno | aceptado | `SQLSTATE 42501` |

- `TestIsolation` pasa con 6 subtests y deja cero bases huérfanas. Sin `TEST_DATABASE_URL` y
  `TEST_APP_DATABASE_URL` se saltea, así que `go test ./...` sigue verde sin base de datos.
- `go vet` limpio, `sqlc generate` sin cambios, `go build` limpio.
- API corriendo con el rol de aplicación: `/readyz` 200, `GET /api/v1/tenants` 200,
  `POST /api/v1/tenants` 201.

Pendiente de E1: T13 dominio IAM y repositorio sqlc; T14 auth con argon2id, rotación de refresh y
detección de reuso; T15 matriz de permisos e invitaciones; T16 tests de la capa HTTP.

**L8 — Corte de jornada (T12 cerrado, T13 sin iniciar)**

Estado al detener:

- Rama `feat/e1-iam-rls`, commit `a65c581`, árbol limpio. **Sin pushear.**
- Revisión nativa: el usuario desactivó el switch para este clon (`clone_local: off`). La lineage
  `review-709b7cd4cda86349` quedó en `reviewing` porque el reviewer `nan/deepseek-v4-flash` agotó su
  ventana de salida (`stopReason: length`, 184k caracteres de razonamiento, 0 de texto) y no se recolectó
  ningún artefacto. Queda resumible vía `STATUS` o abandonable con decisión explícita. **Esta candidata no
  fue revisada por la vía nativa**: la verificación existente es la del writer.

Punto de reanudación de T13, para no reabrir la discusión:

El adaptador de Postgres necesita exponer el contexto de tenant **por transacción** (algo como
`WithTenant(ctx, tenantID, func(tx) error)`), porque RLS lee `app.tenant_id`. Setearlo por conexión con
`SET` filtaría tenants entre requests: el pool reutiliza conexiones entre consultorios distintos. Ese
diseño se define antes de escribir el dominio.

**L9 — E1/T13: dominio IAM y repositorio de usuarios (completado)**

Cortes de review, en dos unidades sobre `feat/e1-t13-iam-domain`:

| Commit | Unidad | Líneas |
| --- | --- | --- |
| `13bdc2d` | Dominio IAM puro (user, role, session, errors) + tests | ~960 |
| `970221d` | Transactor, repositorio, queries, sqlcgen y tests de base | ~1040 |

El corte no es cosmético: el segundo slice es el que toca aislamiento multi-tenant, así que la review
nativa lo puede tomar solo, sin el dominio de por medio.

Decisiones materializadas:

- `NormalizeEmail` replica `users_email_canonical` (`lower(btrim(...))`) y `users_email_shape` exactamente,
  incluida la rareza de que `btrim` recorta espacios pero no tabs. El caso `"\ta@b"` queda fijado por test.
- `PasswordHash` es opaco y solo exige no-vacío. Hashear es T14: un dominio que sabe hashear pierde la
  frontera que lo hace testeable sin stack de auth.
- `RoleCode` es un conjunto cerrado de los 5 roles sembrados, y `TestRoleCatalogDrift` compara contra la
  tabla `roles` en las dos direcciones (ningún constante huérfano, ningún rol sembrado ausente).
- `Transactor.WithTenant` usa `set_config('app.tenant_id', $1, true)`, la forma parametrizada de
  `SET LOCAL` — que no acepta bind parameters y obligaría a interpolar el identificador en el SQL. La
  transacción viaja en el contexto bajo una clave no exportada, así que solo el paquete de postgres
  puede ponerla o sacarla.
- Las queries **no** llevan filtro por tenant a propósito: RLS lo deriva de `app.tenant_id`, y un
  `WHERE tenant_id = $n` sugeriría que el filtro es lo que protege el dato. Fuera de una transacción de
  tenant las políticas cierran por defecto (lecturas 0 filas, escrituras `SQLSTATE 42501`).

Evidencia observada:

- RED del dominio: `go test ./internal/domain/iam/` → `no non-test Go files` (build failed).
- RED de infraestructura: `go vet` → `undefined: postgres.Transactor`, `undefined: postgres.NewUserRepository`.
- `go build ./...` y `go vet ./...` limpios.
- `sqlc generate` no modifica ningún archivo existente; solo agrega `sqlcgen/iam.sql.go` desde las queries nuevas.
- `go test ./...` con ambos DSN: `TestUserRepository` 7/7 subtests, `TestRoleCatalogDrift`,
  `TestIsolation` 6/6 (intacto), 10 subtests de validación de usuario, 8 de `NormalizeEmail`,
  8 de transiciones, `TestNewSession` y `TestSessionRevocation`.
- `go test ./...` sin DSN sigue verde: los tests de base se saltean limpiamente.

Corrección sobre el trabajo del worker: `WithTenant` no tenía rollback diferido, así que un `panic`
dentro de la unidad de trabajo dejaba tomada una conexión del pool por el resto de la vida del proceso.
Se agregó `defer tx.Rollback` y se quitaron los dos rollbacks explícitos que quedaban redundantes.

Hallazgo de esquema para T15+, sin acción en T13: `audit_log.tenant_id` es nullable y su política RLS
compara contra esa columna, así que una fila de auditoría sin tenant (operador de soporte, tenant
borrado) no se puede leer nunca a través de RLS, porque NULL no matchea. Parece intencional para
auditoría de solo-escritura, pero quien escriba el visor de auditoría tiene que saberlo.

Pendiente de E1: T14 auth con argon2id, rotación de refresh y detección de reuso; T15 matriz de
permisos e invitaciones; T16 tests de la capa HTTP.

**L10 — T13 bajo verificación adversarial: veredicto y correcciones (commit `5a857c7`)**

Se corrió un verificador independiente con el encargo explícito de romper el aislamiento, no de
confirmarlo. Veredicto: **la garantía se sostiene** — ninguna ruta devuelve datos de otro tenant — pero
el código que la rodea mentía sobre lo que pasaba. Un defecto real y varias advertencias.

Corregido:

1. **`scoped` dejó de caer al pool.** Antes, sin transacción de tenant, la respuesta la daba RLS: cero
   filas, que el llamador veía como `ErrNotFound`. Un bug de scope quedaba indistinguible de un usuario
   borrado. Ahora devuelve `ErrTenantScopeMissing` antes de tocar la base, en los seis métodos.
2. **`42501` ya no se reporta como falta de scope.** El mapeo asumía que solo un `app.tenant_id` sin
   setear produce ese código. Es falso: un `REVOKE INSERT` o un INSERT en tenant ajeno dan lo mismo, así
   que una regresión de permisos mandaba a leer los logs buscando el problema equivocado. Ahora mapea a
   `ErrInsufficientPrivilege`.
3. **Los comentarios decían que UPDATE y DELETE "levantan 42501" fuera de scope. No lo hacen.** `USING`
   filtra la fila, la sentencia afecta cero y sqlc reporta `ErrNoRows`; solo el INSERT es rechazado por
   `WITH CHECK`. La asimetría quedó escrita y fijada por test, porque leer "cero filas" como veredicto de
   permisos es exactamente cómo se malinterpreta una frontera.
4. **`NewPool` se niega a arrancar** si el rol de conexión es superusuario o tiene `BYPASSRLS`. No es
   teórico: el rol original del clúster local tenía ambos atributos y volvía decorativa toda la capa de
   aislamiento mientras los tests seguían verdes. Verificado: con el rol dueño la API aborta con mensaje
   explícito; con `aveniq_app` escucha normal.
5. La aserción de compilación ahora nombra `port.UserRepository` en vez de repetir su conjunto de
   métodos, así un rename en el port es un error de build acá y no una sorpresa en el composition root.

Hallazgo nuevo del propio test: **una sentencia fallida aborta la transacción completa**. Por eso cada
intento cross-tenant del test corre en su propia `WithTenant`, y por eso un caso de uso no puede intentar
recuperarse dentro de la misma unidad de trabajo: si una sentencia falla, la unidad terminó.

Advertencias **no** corregidas, preexistentes en `a65c581` o fuera del alcance de T13, asignadas a T14/T15:

- Una fila de `sessions` en tenant A puede referenciar un `user_id` de tenant B si se conoce el UUID: el
  chequeo de FK no pasa por RLS. **T14 no puede construir sesiones sobre un `user_id` sin validar.**
- `audit_log` con `tenant_id NULL` no se puede leer ni **escribir** con el rol de aplicación (42501). L9
  decía solo "no se puede leer"; el plan de auditoría de solo-escritura necesita una ruta propia.
- `Create` en un tenant inexistente devuelve el `PgError` 23503 crudo, violando el contrato de `port.go`,
  y es un oráculo de existencia de tenants una vez que T15 resuelva tenants desde el request.
- `List(-1, -1)` devuelve `2201X` crudo: la paginación no se valida en el port.

Evidencia: `go build`, `go vet` y `gofmt` limpios; `go test ./...` con ambos DSN verde, con
`TestUserRepositoryRefusesToRunOutsideTenantScope` y `TestUserRepositoryRejectsCrossTenantAccess`
(5 subtests) nuevos, y `TestIsolation` intacto.
