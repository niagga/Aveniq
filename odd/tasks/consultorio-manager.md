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

**S14 — Historia clínica del MVP: ficha estructurada más evoluciones por encuentro**
> "Ficha estructurada + evoluciones"

**S15 — Agenda del MVP: turnos con estado más series recurrentes**
> "Turnos + series recurrentes"

**S16 — El paciente se registra con datos fiscales desde el inicio**
> "+ datos fiscales"

**S17 — E2 no incluye la gestión de usuarios del sistema; users CRUD e invitaciones van en E3**
> "No incluir users CRUD"

**S18 — Un professional ve solo sus propios pacientes y turnos (E3)**
> Decisión en AskQuestion: "Solo sus pacientes y turnos (Recomendado)"

**S19 — Papel `assistant` (recepción): agenda y cadastro, sin historia clínica (E3)**
> Decisión en AskQuestion: "Sí, agenda y cadastro sin historia clínica (Recomendado)"

**S20 — Invitación por enlace generado por el owner, sin email automático (E3)**
> Decisión en AskQuestion: "Link generado por el owner y copiado en la pantalla, sin email (Recomendado)"

**S21 — La ficha de identificación es compartida; la historia clínica solo con autorización (E3)**
> "La ficha de información de paciente (nombre, ID, DNI, CUIT) es compartida, la HC solo es compartida si existe una autorización"

**S22 — El papel `accountant` ve solo facturación (E3)**
> "Solo facturación"

**S23 — Autorización de HC: la concede el profesional tratante, por defecto la ve el tratante, revocable sin plazo (E3)**
> Decisiones en AskQuestion: "Lo concede el profesional tratante, para otros profesionales del consultorio"; "Sí, el tratante siempre ve la HC de sus pacientes (Recomendado)"; "Revocable a cualquier momento, sin validez fija (Recomendado)"

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
| T14 | S3 | E1 | Auth: argon2id, login/refresh/logout con rotación y detección de reuso, middleware | inline | `6a3f9eb`, `1905de1`, `3b09615`, `5f50e49`, `98f2dd3`, `011d1ca` |
| T15 | S3, S11 | E1 | Matriz de permisos por dominio, invitaciones y alta de tenant en trial | worker | pendiente |
| T16 | S5 | E1 | Test de fuga cross-tenant en CI y tests de la capa HTTP | inline | `c1f0f89` |
| T17 | S3 | E2 | API: el refresh token viaja en cookie `HttpOnly`; login/refresh/logout la setean y la limpian | inline | `6930a07` |
| T18 | S3, S12 | E2 | Frontend: login, guard de sesión y refresco silencioso | inline | `34dbd60` |
| T19 | S5, S11 | E2 | API: tenant desde el token y guard grueso por rol en las rutas de dominio | inline | `aa422af`, `51d4779` |
| T20 | S1, S16 | E2 | Migración `0004`: pacientes con datos fiscales, unicidad de documento por tenant, RLS forzado | inline | `2cf9dd7` |
| T20b | S16, S5 | E2 | Migración `0005`: países, catálogo de documentos y datos fiscales por país, con seed AR/ES/PT/BR | inline | `b1daf13` |
| T21 | S1, S11 | E2 | Dominio `patient` + repositorio sqlc + tests de aislamiento | inline | `50cb193`, `db2f998`, `b36a756` |
| T22 | S1, S11 | E2 | HTTP de pacientes (alta, listado con búsqueda, detalle, edición, baja lógica) | worker + inline | `5182547`, `5d467fb` |
| T23 | S1, S12 | E2 | Frontend: pacientes (listado, alta, detalle, datos fiscales) | inline | `13c7d72` |
| T24 | S1, S16 | E2 | Migración `0007`: ficha clínica 1:1 con el paciente + evoluciones, RLS forzado | inline | `4d54a24` |
| T25 | S1, S11 | E2 | Dominio `clinical` + repositorio sqlc + tests de aislamiento | inline | `bc4163a` |
| T26 | S1 | E2 | HTTP de historia clínica (ficha y evoluciones; autoría desde el token, nunca del cuerpo) | inline | `771c80f`, `fbbc46b` |
| T27 | S1, S12 | E2 | Frontend: ficha clínica y evoluciones | inline | — |
| T28 | S15, S5 | E2 | Migración `0009`: turnos, series recurrentes, zona horaria del tenant, exclusión de solapamiento | inline | — |
| T29 | S15, S11 | E2 | Dominio `schedule` + repositorio sqlc (solapamiento, expansión de series, estados) | inline | — |
| T30 | S15 | E2 | HTTP de agenda (turnos por rango, alta, cambio de estado, series) | inline | — |
| T31 | S15, S12 | E2 | Frontend: agenda semanal por profesional | inline | — |
| T32 | S4, S11 | E2 | Verificación end-to-end de E2 y regeneración del cliente OpenAPI | Playwright local | — |
| T33 | S3, S18, S19, S21, S22, S23 | E3 | Matriz de permisos por dominio (owner, admin, professional, assistant, accountant) que reemplaza el guard grueso de L14 | inline | — |
| T34 | S3, S19 | E3 | Usuarios del consultorio: listado, detalle, directorio de autores, cambio de papel, baja y reactivación con reglas de autorización. Sin alta (ver T35) | inline | sin commit |
| T35 | S3, S20 | E3 | Invitaciones: token de uso único con vencimiento, enlace generado por el owner. La aceptación crea la cuenta con contraseña (el alta) | inline | sin commit |
| T36 | S3, S5 | E3 | Alta de consultorio en trial: signup público con rate limiting; verificación por email pendiente de proveedor | inline | pendiente |
| T37 | S3, S12 | E3 | Frontend: usuarios, invitación, aceptación y signup público | inline | pendiente |
| T38 | S18, S19 | E3 | Gates por permiso en frontend y tests E2E de aislamiento (un professional no ve pacientes ajenos, assistant no ve historia clínica) | inline | pendiente |

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

**L11 — Decisiones de diseño de T14 (auth), antes de escribir código**

La autenticación tiene una diferencia con T12/T13: el aislamiento multi-tenant lo resuelve la base,
pero *quién es el que llama* no lo puede resolver RLS. Estas decisiones se fijan acá porque cada una
cambia el contrato del endpoint y equivocarse implica reescribir.

1. **El login necesita el tenant, no solo el email.** El email es único por tenant, no global: el mismo
   humano puede existir en dos consultorios. Por eso `POST /api/v1/auth/login` recibe
   `{tenant_slug, email, password}`. Resolver el tenant por slug ocurre **fuera** del scope (la tabla
   `tenants` no tiene RLS); a partir de ahí todo corre dentro de `WithTenant`.
2. **El refresh token lleva el tenant en claro.** Para rotar hay que abrir el scope antes de buscar la
   sesión, y RLS no deja buscar sin scope. Formato `v1.<tenant-uuid>.<32 bytes base64url>`, y en
   `sessions.refresh_hash` se guarda el SHA-256 **del token completo**. Cambiar el prefijo cambia el
   hash y no matchea ninguna fila: el tenant no es un permiso, es una pista de búsqueda. El secreto
   sigue siendo el segundo segmento.
3. **Reuso = matar la cadena, no la hoja.** Si se presenta un token cuyo `revoked_at` no es NULL, no se
   responde "vencido": se revoca toda la cadena de sucesores (`replaced_by`) porque el token robado y
   el legítimo compiten, y no se sabe cuál es cuál. Se responde error, no un token nuevo.
4. **T14 autentica; T15 autoriza.** El middleware resuelve identidad (tenant, usuario, rol) y la pone en
   el contexto. La matriz de permisos por dominio es T15: mezclarlas ahora haría que el middleware
   tenga que importar cada dominio futuro.
5. **Access token firmado con HMAC-SHA256, sin dependencias nuevas.** Claims `sub`, `tid`, `role`, `iat`,
   `exp`. Se implementa con `crypto/hmac` y `encoding/base64` del stdlib en vez de sumar una librería
   JWT cuyo `alg` haya que defender: no se acepta `alg` del token, se verifica siempre HS256 propio.
   Vida corta (15 min) porque no hay revocación de access tokens en E1.
6. **Argon2id con parámetros explícitos y verificables.** `m=64 MiB, t=3, p=4`, salt de 16 bytes, key de
   32 bytes, codificación PHC estándar. El verificador parsea los parámetros **del hash almacenado**, no
   de una constante: si mañana se suben los costos, los hashes viejos siguen verificando.
7. **El agujero de la FK cross-tenant se cierra en el adaptador.** `SessionRepository.Create` verifica en
   la misma transacción que el `user_id` es visible en el scope (`SELECT 1 FROM users WHERE id = $1`):
   RLS devuelve cero filas para un usuario de otro tenant y la sesión no se crea. Se resuelve en el
   adaptador y no en el caso de uso para que ningún llamador futuro pueda olvidarlo.
8. **Sin usuarios activos no hay login, y sin revelar cuál falló.** `is_active = false` y contraseña
   incorrecta producen el mismo error y el mismo tiempo de respuesta: se verifica el hash contra un
   dummy cuando el usuario no existe o está inactivo. Un oráculo de "este email existe" en un SaaS de
   salud es un problema de privacidad, no de UX.

Cortes de review previstos, sobre `feat/e1-t14-auth`:

| Unidad | Contenido | Por qué se separa |
| --- | --- | --- |
| 1 | `platform/authn` (argon2id, tokens, verificación) + `SessionRepository` y queries | Primitivas sin HTTP: se pueden romper sin tocar la API |
| 2 | Casos de uso login/refresh/logout + middleware + handlers + wiring | Es donde vive la lógica de rotación y reuso |


**L12 — E1/T14 unidad 1: primitivas de auth y sesiones (commiteado)**

Unidad 1 de L11 completa, en tres cortes sobre `feat/e1-t14-auth`:

| Commit | Unidad | Contenido |
| --- | --- | --- |
| `6a3f9eb` | authn: hashing | argon2id PHC, parámetros leídos del hash almacenado |
| `1905de1` | authn: tokens | refresh `v1.<tenant>.<32B>` + SHA-256 del token completo; access HMAC-SHA256 propio |
| `3b09615` | sesiones | `SessionRepository`, queries, guard cross-tenant, rotación y `RevokeChain` |

Decisión de dependencia tomada por el orquestador, no por el worker: `golang.org/x/crypto`
no figuraba en `go.mod` (solo transitivamente en `go.sum`), y argon2id vive ahí. Se agregó
`v0.58.0` — la versión que ya estaba en el árbol — en vez de dejarla subir a `v0.59.0`, para
que el corte no arrastre un bump ajeno a T14.

Evidencia observada:

- RED de las primitivas: `go test ./internal/platform/authn/` → `no non-test Go files`.
- RED del adaptador: **no se observó**. El writer de ODD falló dos veces con
  `Writer repository preparation was unavailable or its session/target changed`, sin encolar
  tarea, así que la unidad se escribió inline y la implementación precedió a los tests. Se
  compensó con prueba de mutación, no con una declaración de confianza (ver abajo).
- `gofmt` limpio en todo lo tocado; `go build ./...` y `go vet ./...` limpios.
- `sqlc generate` idempotente: segunda corrida no agrega diff.
- `go test ./...` verde **sin** DSN (los de base se saltean) y **con** ambos DSN.
- `TestSessionRepository`: 7/7 subtests. `TestPasswordHasher*` y `TestAccessTokens*` verdes.

**Prueba de mutación del guard cross-tenant.** No es ceremonia: es la única evidencia de que
los tests muerden cuando no hubo RED. Se neutralizó el chequeo de visibilidad
(`SessionUserVisible`) y el subtest falló exactamente como debía:
`error = <nil>, want iam: not found: a foreign user must look like a missing one`. Sin el guard,
la FK de `sessions.user_id` acepta un usuario de otro tenant y la sesión se crea. Esto confirma
empíricamente la advertencia que dejó L10: el chequeo de FK **no** pasa por RLS. Restaurado y
reverificado verde.

Hallazgos que la unidad 2 necesita:

- `Rotate` recibe la sesión previa **ya revocada** por el dominio (`Session.ReplaceWith`), e
  inserta el sucesor antes de escribir `replaced_by` porque esa columna es FK.
- Una actualización de cero filas se reporta como `iam.ErrInvalidTransition`, no `ErrNotFound`:
  "gasto ya usado, posible replay" tiene que ser distinguible de "token que nunca existió".
- `ByRefreshHash` devuelve filas revocadas a propósito. Filtrarlas convertiría "este token ya se
  usó" en "este token no existe" y apagaría la detección de reuso.
- `gofmt -l` reporta `Backend/internal/interfaces/http/tenant_handlers.go`, preexistente a esta
  tarea y no tocado acá.

**L13 — E1/T14 completo: casos de uso, HTTP y un bug que solo aparecía de punta a punta**

T14 cerrado en seis commits sobre `feat/e1-t14-auth`. La unidad 2 se partió en dos por
tamaño de review: casos de uso con sus dobles (`5f50e49`) y capa HTTP + wiring (`011d1ca`).

| Commit | Contenido |
| --- | --- |
| `5f50e49` | `application/auth`: login, refresh, logout + 18 subtests con dobles en memoria |
| `98f2dd3` | corrección del bug de rollback en la revocación por reuso |
| `011d1ca` | middleware de identidad, handlers huma, esquema bearer en OpenAPI, `AUTH_ACCESS_SECRET` |

**El bug que motivó el commit de corrección.** La primera versión revocaba la cadena de
sucesores **dentro** del `WithTenant` que después devolvía `ErrRefreshReused`. Como
`WithTenant` hace rollback cuando la unidad de trabajo devuelve error, la revocación se
deshacía a sí misma: el token robado se rechazaba, el sucesor legítimo seguía vivo y
seguía rotando. El bug es invisible en los tests de casos de uso porque el doble de
`TenantScope` no tiene rollback que deshacer; apareció recién al ejercitar la API real.
Diseño corregido: la negativa se decide en la transacción de lectura, que no escribe; la
revocación corre después, en su propia transacción que commitea. El test de reuso ahora
fija la forma (dos aperturas de scope) en vez de confiar en el efecto.

Regla general que dejó: **en este proyecto, "escribir y después devolver error" dentro de
`WithTenant` no funciona.** Cualquier efecto que deba sobrevivir a un fallo va en su propia
transacción.

**Evidencia end-to-end** (API en `127.0.0.1:8081` sobre la base local, tenant y usuario
reales creados por SQL con un hash argon2id minteado con los parámetros de producción):

| Paso | Respuesta |
| --- | --- |
| `POST /auth/login` credenciales válidas | 200, `expiresIn=900`, `tokenType=Bearer`, `role=owner` |
| `GET /auth/me` con el token | 200, id/tenantId/role del usuario |
| `GET /auth/me` sin token / token basura | 401 / 401 |
| login con password incorrecta | 401 |
| login con email inexistente | 401, **mismo cuerpo** que la password incorrecta |
| login con tenant inexistente | 401 |
| `POST /auth/refresh` | 200, refresh nuevo (rotación real) |
| reuso del token gastado | 401 "already used; the session chain was revoked" |
| refresh con el sucesor después del reuso | **401** (antes del fix: 200) |
| `POST /auth/logout` | 204 |
| logout repetido / con basura | 204 / 204 |
| refresh después de logout | 401 |

**Prueba de mutación de la paridad de tiempo.** Se quitó la verificación contra el hash
dummy en la rama de email inexistente y el subtest falló con
`verified hashes = [], want exactly the dummy hash`. Restaurado y verde.

Decisiones materializadas en la capa HTTP:

- Solo `/auth/me` exige identidad. Gatear `/tenants` habría inventado una política que es
  de T15, y habría roto el cliente actual.
- El refresh token viaja en el cuerpo, no en cookie `HttpOnly`. Es una decisión de
  transporte que el frontend todavía no tomó: si el cliente es un navegador, debería ser
  cookie. Queda anotado, no resuelto.
- `AUTH_ACCESS_SECRET` tiene default de desarrollo para no romper la promesa de "clon
  nuevo corre sin `.env`", y `validate()` rechaza ese default fuera de desarrollo. Un
  secreto de firma commiteado dejaría a cualquiera mintear un token para cualquier tenant.
- `me` no consulta la base: la identidad viaja firmada en el token, que es el punto entero
  de un access token.

Pendiente de E1: T15 matriz de permisos por dominio, invitaciones y alta de tenant en trial;
T16 tests de la capa HTTP y de fuga cross-tenant en CI.

Notas de higiene: `gofmt -l` reporta `tenant_handlers.go` como preexistente; reformatearlo
quedó fuera de este corte a propósito. El writer de ODD no estuvo disponible en toda la
jornada (dos intentos, `repository preparation unavailable`, sin tarea encolada), así que
T14 se escribió inline; por eso la columna Ruta dice `inline` y no `worker`.

**L14 — Desglose de E2: pacientes, historia clínica y agenda**

Decisiones del usuario, verbatim: "Ficha estructurada + evoluciones", "Turnos + series recurrentes",
"+ datos fiscales", "No incluir users CRUD". Es la rama cara de las dos que importaban: ficha **y**
evoluciones, y series recurrentes. E2 pasa a ser la etapa más grande hasta ahora (T17–T32).

Reglas de diseño fijadas acá, porque cada una tiene una forma silenciosa de fallar:

1. **Toda tabla nueva nace con RLS forzado.** `tenant_id`, `ENABLE` + `FORCE ROW LEVEL SECURITY` y
   política contra `NULLIF(current_setting('app.tenant_id', true), '')::uuid`, igual que `0002`.
   Los `GRANT` **no** deberían hacer falta porque `0003` dejó `ALTER DEFAULT PRIVILEGES`, pero eso se
   verifica en T20 en vez de confiarse: si el default privilege no alcanza, el síntoma es `42501` en
   producción y un test de aislamiento que igual pasa.
2. **El tenant sale del token, nunca del cuerpo ni del host.** Un `tenant_id` en el body es un
   agujero: firmado no es lo mismo que autorizado. El host sigue sirviendo para la marca, no para el
   scope.
3. **Guard grueso hasta E3.** Las rutas de dominio exigen un rol con acceso (owner, admin,
   professional). Es deliberadamente tosco y lo reemplaza la matriz de E3. Riesgo residual aceptado y
   explícito: un `assistant` queda afuera aunque en la práctica necesite ver la agenda. Prefiero negar
   de más a filtrar de menos.
4. **La autoría de una evolución sale del token.** Si el `author_user_id` viniera del cuerpo, cualquiera
   podría firmar una nota clínica ajena. Esto no es un detalle de validación: es la diferencia entre un
   registro y un relato.
5. **Lectura de historia clínica con auditoría.** T26 incluye registrar quién leyó qué HC. Es la deuda
   que L10 dejó abierta: `audit_log` con `tenant_id NULL` no se puede escribir con el rol de aplicación
   (`42501`), así que T26 empieza resolviendo esa ruta de escritura. Sin eso, en E2 se leen datos
   clínicos sin rastro, y no hay matriz de permisos (E3) que compense.
6. **Series materializadas, no calculadas al leer.** Al crear o editar una serie se emiten los turnos
   concretos del rango; cada turno se edita y cancela por separado. Regla dura: **una edición de la
   serie no toca un turno ya atendido**. Sin esa regla, mover el horario semanal reescribe sesiones que
   ya ocurrieron.
7. **Zona horaria por tenant.** `tenants.timezone` (default `America/Argentina/Buenos_Aires`),
   instantes guardados en UTC, expansión de series en la zona del tenant. Sin esto, "todos los martes
   18:00" se corre una hora dos veces por año.
8. **Solapamiento en la base.** `EXCLUDE USING gist (professional_user_id WITH =, tstzrange(starts_at,
   ends_at) WITH &&) WHERE (status <> 'cancelled')`, que exige `btree_gist`. Si el entorno de
   despliegue no permite extensiones, el fallback es un chequeo dentro de la misma transacción; el
   default es la constraint, porque una validación en Go pierde contra dos requests concurrentes.
9. **Refresh en cookie `HttpOnly`** (T17). El cliente es un navegador, así que el token deja de viajar
   en el cuerpo: `SameSite=Lax`, `HttpOnly`, `Secure` fuera de desarrollo. Sin token CSRF mientras el
   frontend siga siendo mismo-origen; se revisa el día que haya un origen distinto.

Orden: primero sesión en el navegador (T17, T18), después el guard (T19), y recién ahí los dominios.
Sin T19 no hay nada que proteger; sin T17/T18 no hay forma de ejercitar T19 desde el producto.
Cada dominio entra en tres cortes (migración → dominio y repositorio → HTTP) más su corte de frontend,
para que cada uno quede cerca de las 400 líneas de review.

Fuera de E2, a propósito: facturación (S2), notificaciones al paciente, adjuntos, y users CRUD con
invitaciones (S17). T15 sigue pendiente y se hace en E3, con dominios reales sobre los que razonar; T16
se cierra junto con T19, porque los tests de la capa HTTP cubren justamente el guard nuevo.

Riesgo de la etapa, para que quede dicho: T17–T32 son ~16 cortes y del orden del doble de código que
E1. Y hasta E3, la única barrera entre un usuario autenticado de un consultorio y la historia clínica de
sus pacientes es el guard grueso del punto 3.

**L15 — E2/T17: el refresh grant pasa a cookie `HttpOnly` (commiteado)**

Cambio de contrato, no cosmético: `POST /auth/login` y `POST /auth/refresh` ya no devuelven
`refreshToken` en el cuerpo; setean `aveniq_refresh` con `HttpOnly`, `SameSite=Lax`,
`Path=/api/v1/auth` y `Secure` fuera de desarrollo. `POST /auth/logout` la borra con
`Max-Age=0`. El cuerpo sigue aceptando el token como fallback para clientes que no son
navegador (tests, CLI), y la cookie gana cuando llegan las dos.

¿Por qué importa: el refresh es la mitad de vida larga del par (30 días). En el cuerpo,
todo cliente estaba obligado a guardarlo donde JavaScript lo lee, y un XSS pasaba a ser
toma de cuenta completa. El access token sigue en el cuerpo: es corto (15 min) y el cliente
lo necesita en memoria para el header `Authorization`.

Evidencia end-to-end (API en `8081`, tenant y usuario reales):

| Paso | Resultado |
| --- | --- |
| login | 200; `Set-Cookie: aveniq_refresh=v1.…` con `HttpOnly`, `SameSite=Lax`, `Max-Age=2592000` |
| cuerpo del login | 0 ocurrencias de `refreshToken`; campos: `accessToken`, `tokenType`, `expiresIn`, `refreshExpiresAt`, `user` |
| refresh con **solo** la cookie, sin body | 200 y cookie rotada |
| reuso de la cookie anterior | 401 "already used; the session chain was revoked" |
| la cookie rotada después del reuso | 401 (cadena muerta) |
| logout | 204 con `aveniq_refresh=; Max-Age=0; …` |
| refresh tras logout | 401 |
| `openapi.json` | declara el parámetro cookie |

Un detalle que costó un ciclo y queda fijado por test: `http.Cookie` serializa
`MaxAge: -1` como `Max-Age=0`. Mi primer test esperaba `Max-Age=-1` y falló; el test estaba
mal, no el código. La RFC 6265 define `Max-Age=0` como "expirar ya", que es exactamente la
instrucción de borrado. Es la clase de aserción que conviene tener escrita: si alguien
"arregla" el `-1` del código para que coincida con el número del wire, rompe el borrado.

Deuda declarada, no olvidada: no hay token CSRF porque el frontend es mismo-origen y
`SameSite=Lax` ya bloquea POST cross-site. El día que haya un origen distinto (o un
subdominio sirviendo la app) hay que sumarlo. Y en producción conviene evaluar el prefijo
`__Host-`, que exige `Secure` y `Path=/`, incompatible con el path acotado que usamos hoy.

Pendiente de E2: T18 login en el frontend, T19 guard de tenant y rol, y los tres dominios.

**L16 — E2/T18: sesión en el navegador (commiteado)**

Decisión del usuario, entre tres opciones: **memoria + coordinación entre pestañas**. El access
token vive solo en un store en memoria; nunca en `localStorage` ni `sessionStorage`.

El detalle que obligó a elegir con cuidado, y que conviene no olvidar: **cada refresh gasta el
token presentado, y el servidor responde a un token gastado revocando la cadena entera**. Con dos
pestañas abiertas, cada una con su propio ciclo de refresco, la segunda presenta un token que la
primera ya gastó, y las dos quedan deslogueadas. Eso no es un bug del servidor: es la detección de
reuso de T14 haciendo su trabajo. El cliente es el que no puede provocarla.

Cómo se resolvió:

- `navigator.locks` (Web Locks) serializa el refresco: una sola pestaña lo ejecuta.
- `BroadcastChannel` publica el token resultante; las demás lo adoptan en vez de rotar por su
  cuenta.
- Al montar, cualquier pestaña sin token pide uno bajo el lock, y dentro del lock vuelve a
  verificar si ya hay sesión: si otra pestaña refrescó mientras esperaba, no gasta otra rotación.
- Un temporizador refresca a los ~14 minutos, no al recibir un 401: un request que falla por token
  vencido ya es un request perdido.

Evidencia observada (Next en `3000` con `API_ORIGIN` a la API local):

| Verificación | Resultado |
| --- | --- |
| `pnpm typecheck` | limpio |
| `pnpm build` | limpio; rutas `/` (dinámica) y `/login` (estática) |
| `GET /login` | 200 con los tres campos (`tenantSlug`, `email`, `password`) y el botón |
| `GET /` | 200; el gate renderiza "Verificando sesión…" |
| `POST /api/v1/auth/login` vía el proxy de Next | 200 y `Set-Cookie` llega al navegador |
| `POST /api/v1/auth/refresh` con **solo** la cookie, vía el proxy | 200 |

Ese último par importa: confirma que el proxy mismo-origen de `next.config.ts` no rompe ni el
`Set-Cookie` ni el path acotado de la cookie.

Límite honesto de esta verificación: **la coordinación entre pestañas no se ejercitó en tiempo de
ejecución**. No hay automatización de navegador en el proyecto, así que Web Locks y
`BroadcastChannel` están verificados por tipos y revisión, no por una prueba. Un smoke test con
Playwright —dos contextos, refresco simultáneo, sesión viva— corresponde a T32.

Riesgo declarado que T19 debe cerrar: el gate es de UX, no de seguridad. `page.tsx` sigue siendo un
Server Component que consulta la API antes de que el gate corra en el browser, así que el payload
renderizado contiene los consultorios aunque el visitante no tenga sesión. Hoy la API no exige token
para `/tenants`; en cuanto T19 lo exija, el gate pasa a acompañar a una barrera real en vez de
sustituirla.

`Frontend/AGENTS.md` entra al repo porque lo escribe `next dev` en cada arranque y el propio archivo
pide commitearlo en vez de pelear con el árbol sucio.

**L17 — E2/T19: el tenant sale del token y las rutas de dominio exigen identidad (commiteado)**

Hasta este corte, la sesión del navegador era una idea de UX: `/tenants` respondía a cualquiera.
Ahora no.

- `GET /api/v1/tenants` y `GET /api/v1/tenants/{slug}` exigen bearer token y resuelven la práctica
  desde el claim `tid` **firmado**, no desde la URL ni del cuerpo. `app.tenant_id` deja de ser
  elegible por el llamador.
- `GET /tenants` cambia de semántica: devuelve **la práctica de la sesión**, una sola. La vista
  plataforma-wide es de operador y no tiene ruta todavía; mantener la lista completa habría dejado
  un endpoint que enumera consultorios detrás de un guard.
- Un slug ajeno responde **404**, no 403. Confirmar que otra práctica existe ya es información.
- Guard de rol grueso: owner, admin, professional. `accountant` recibe 403.
- `POST /tenants` queda **público a propósito**: es la única puerta por la que una práctica puede
  existir. E3 lo convierte en signup verificado con rate limiting. Está escrito como decisión, no
  como olvido.

Consecuencia arquitectónica que hay que registrar, porque invierte lo decidido en E0: **el panel
pasa a ser Client Component**. El access token vive en memoria del navegador, así que el servidor no
tiene credencial para pedir datos en nombre del usuario; un Server Component ahí recibe 401. Lo que
sigue igual es la frontera que importaba: `API_ORIGIN` es server-only y todo pasa por el proxy
mismo-origen. Y el formulario de alta de consultorio sale del panel: una práctica se crea desde
afuera, en el onboarding, no desde adentro de una sesión. Vuelve en E3 como pantalla pública.

El cambio de `port.TenantRepository` (se agregó `ByID`) rompió los dos dobles de test que lo
implementan, que es exactamente lo que se espera de un puerto: los ajustes aparecieron en el build,
no en producción.

Evidencia end-to-end (API real, tenant y usuarios reales: owner y accountant):

| Caso | Resultado |
| --- | --- |
| `GET /tenants` sin token | 401 |
| `GET /tenants/{slug}` sin token | 401 |
| `GET /tenants` con owner | 200, **1** ítem, `slug=e2e-t14` |
| `GET /tenants/e2e-t14` con owner | 200 |
| `GET /tenants/consultorio-sur` con owner | 404 (existe, pero no es mía) |
| `GET /tenants` con accountant | 403 |
| `GET /auth/me` con accountant | 200 (identidad, sin guard de rol: correcto) |
| `POST /tenants` sin token | 201 (onboarding público) |

Suite Go completa (con y sin DSN), `gofmt` limpio, `sqlc generate` idempotente, `pnpm typecheck` y
`pnpm build` limpios; el build ahora marca `/` como estática, porque ya no consulta la API en el
servidor.

Lo que este corte **no** trae: los tests de la capa HTTP completa (T16). Se agregaron dos unitarios
reales —`bearerToken` con 9 casos y la ausencia de identidad sin middleware— pero ejercitar el router
entero con dobles de puertos sigue siendo T16, y ahora tiene un motivo más claro: el guard nuevo es
justo lo que hay que fijar con tests. Riesgo aceptado y escrito desde L14: un `assistant` queda
afuera del dominio aunque en la práctica pueda necesitar ver la agenda.

**L18 — E2/T20: esquema de pacientes (commiteado)**

Migración `0004` (commiteada en `2cf9dd7`), aplicada a la base local (`goose` → versión 4) y
verificada con un test que corre **como el rol de aplicación**, no como el dueño.

Decisiones de esquema que quedan fijadas:

- **Documento opcional como par, no como dos campos.** Tipo sin número (o al revés) es una fila a
  medio llenar que no se puede cruzar con nada: `patients_document_complete` lo prohíbe.
- **Único por tenant, compartible entre tenants.** Índice único parcial sobre
  `(tenant_id, document_type, document_number)`. La misma persona puede ser paciente de dos
  consultorios; el mismo documento, una sola vez en cada uno.
- **`full_name` es lo único obligatorio.** Exigir documento empujaría a registrar a alguien en las
  notas administrativas, donde no se valida nada.
- **Baja lógica, nunca `DELETE`.** `is_active` existe porque `0005` va a colgar notas clínicas de
  esta fila: un borrado o se lleva la historia por cascada o falla en la FK. El índice de listado es
  parcial (`WHERE is_active`).
- **Condición fiscal como conjunto cerrado** de los cuatro valores que AFIP acepta, y **CUIT con
  forma de 11 dígitos**: un typo tiene que fallar en el formulario y no en la plataforma de
  facturación.
- **`administrative_notes` no es clínica.** Está dicho en el comentario de la columna, porque es
  exactamente donde la información clínica se filtra cuando no hay otro lugar.

**Lo que L14 mandó verificar en vez de confiar, verificado.** La pregunta era si los
`ALTER DEFAULT PRIVILEGES` de `0003` alcanzan a una tabla creada por una migración posterior.
Respuesta: **sí**. La migración `0004` tiene **cero** sentencias `GRANT`, y el rol `aveniq_app`
inserta, lee y filtra sin que nadie le otorgue nada.

Evidencia (`TestPatientsSchemaPrivileges`, 5 subtests, todos verdes):

| Caso | Resultado |
| --- | --- |
| `aveniq_app` inserta en `patients` sin GRANT explícito | OK |
| tenant B cuenta pacientes de A | 0 |
| tenant A se cuenta a sí mismo | > 0 |
| sin `app.tenant_id` | 0 filas (cierra por defecto) |
| insert con `tenant_id` ajeno | `SQLSTATE 42501` |
| mismo documento dos veces en un tenant | `SQLSTATE 23505` |
| mismo documento en otro tenant | permitido |

`go test ./...` verde con y sin DSN; `gofmt` limpio; `sqlc generate` idempotente y agrega el modelo
`Patient` (+ `CreatePatient`/queries vendrán en T21).

Nota de higiene: el test de esquema reusa `newScratchDatabase`, `migrate` y `seedTenants` de la
suite existente y agrega un helper local `withTenantSetting`, porque `set_config` transaccional es
la única forma que RLS lee y no estaba expuesto como utilidad de test.

**L19 — E2/T20b: el país es dato, no una rama (commiteado)**

Corrección de una suposición mía, hecha por el usuario: **0004 asumía Argentina**. Estaba horneado en
tres lugares —`CHECK` de `document_type` con DNI/LC/LE/CUIT, `patients_cuit_shape` de once dígitos, y
`fiscal_condition` con la lista de AFIP— y ninguno sobrevive a España, Portugal o Brasil.

Se arregló **antes** de escribir el dominio (T21), que es el único momento en que sale barato: la tabla
estaba vacía. Después de T21 habría sido una migración de datos de identidad.

Decisiones del usuario: países iniciales AR, ES, PT, BR; **el país se elige por consultorio y por
paciente**, con herencia del consultorio cuando el paciente no lo define.

### El modelo

`countries`, `document_kinds`, `tax_id_kinds` y `fiscal_conditions`. Cada uno lleva lo que el dominio
necesita para validar sin conocer el país: `pattern` (forma canónica), `normalizer` (`digits`,
`alnum_upper`, `as_is`) y `checksum` (nombre de la estrategia en Go: `ar_cuit`, `es_dni`, `es_nie`,
`pt_nif`, `br_cpf`, `br_cnpj`, `pt_cc`). Agregar un país es insertar filas; agregar una estrategia de
verificación es una entrada en un mapa, no un `switch` por capa.

`tax_id_kinds` es una tabla **separada** de `document_kinds` a propósito: con una sola tabla y un flag
`is_tax`, ninguna FK podría exigir "este código tiene que ser fiscal", y ese chequeo terminaría en un
trigger.

Una limitación queda escrita en la migración para que nadie la reintente: **un `CHECK` no puede leer
otra tabla**, así que el `pattern` por país lo aplica el dominio. La base se queda con lo que sí puede
sostener declarativamente: el tipo existe *y pertenece al país de la fila*.

### Cambios en `patients`

- `country` (FK a `countries`), que el adaptador escribe siempre resuelto — el paciente hereda el del
  consultorio cuando no lo define, así ninguna fila lleva documento sin país contra el cual validarlo.
- `document_normalized` y el índice único reconstruido sobre `(tenant_id, document_type,
  document_normalized)`: `30.111.222` y `30111222` son la misma persona.
- `cuit` → `tax_id` + `tax_id_kind`, con FK a `tax_id_kinds`.
- `fiscal_condition` pasa a FK a `fiscal_conditions` por país: `monotributo` ya no es válido en Portugal.
- Los catálogos quedan de **solo lectura** para `aveniq_app` (`REVOKE INSERT, UPDATE, DELETE`): agregar
  un país es una migración, no un request.

### Un bug que encontró mi propio test

La primera versión de `patients_document_complete` exigía que el país estuviera **ausente** cuando no
hay documento. El test de condición fiscal lo rompió: un paciente con condición fiscal y sin documento
es el caso normal de cualquier consultorio que empieza. Corregido: el país se exige cuando hay algo
que validar, no cuando hay documento.

Evidencia (`TestPatientsCountryModel`, 6 subtests, todos verdes):

| Caso | Resultado |
| --- | --- |
| país por defecto del tenant | `AR` |
| NIE español con número alfanumérico | aceptado |
| NIE bajo `AR` | `SQLSTATE 23503` |
| documento sin país | `SQLSTATE 23514` |
| `monotributo` bajo `AR` / bajo `PT` | aceptado / `23503` |
| mismo documento normalizado dos veces en un tenant | `23505` |
| `aveniq_app` lee catálogos | OK |
| `aveniq_app` inserta un país | `SQLSTATE 42501` |

Y el test de T20 tuvo que actualizarse: sus inserts de documento ahora necesitan país y forma
normalizada. Es la consecuencia esperada de un modelo más estricto, no una regresión.

`goose up` + `goose down` + `goose up` verificado sobre la base local; suite completa con y sin DSN en
verde; `sqlc generate` idempotente (agrega `Country`, `DocumentKind`, `TaxIDKind`, `FiscalCondition`).

**Renumeración:** la historia clínica pasa a ser la migración `0006` (T24) y la agenda `0007` (T28).

**L20 — E2/T21: dominio y persistencia de pacientes (commiteado)**

Tres cortes en `feat/e2-t21-patient-domain`: dominio (`50cb193`), corrección de precisión de
timestamps (`db2f998`) y persistencia (`b36a756`).

### El dominio no sabe en qué país está

Una regla de país llega **como dato** (`DocumentKind`: `pattern`, `normalizer`, `checksum`) y el
agregado la aplica. Agregar un país es una fila de catálogo más una entrada en
`map[string]func(string) error`; no hay un `if country == ...` en ninguna capa.

Orden del pipeline, y el orden es la mitad del diseño: **normalizar → patrón → dígito verificador**.
Normalizar primero es lo que hace que `30.111.222` y `30111222` sean la misma persona y que un NIE
en minúsculas no burle el índice único.

Estrategias implementadas y verificadas con vectores conocidos: `ar_cuit` (mod 11 con pesos),
`br_cpf`, `br_cnpj`, `es_dni` (tabla de 23 letras), `es_nie` (X/Y/Z como 0/1/2), `pt_nif`.
**`es_nif` y `pt_cc` quedan como `shapeOnly`, escrito por nombre**: el patrón sigue rechazando
basura y el hueco se encuentra buscándolo, en vez de descubrirse cuando un dígito inválido pasa.

**Una estrategia desconocida falla cerrado.** Si el catálogo nombra un `checksum` que nadie
implementó, el documento se rechaza. Saltearlo en silencio haría que todos los documentos de ese
tipo parezcan validados.

### Dos cosas que el código me obligó a cambiar

1. **`Reconstitute` con 17 parámetros posicionales.** Dos `*string` intercambiados compilan, corren,
   y escriben el contacto de emergencia en la dirección fiscal. Pasó a un struct `Stored`.
2. **Precisión de timestamps.** Postgres guarda microsegundos y `time.Now()` trae nanosegundos, así
   que "CreatedAt no cambió" dependía de dónde corriera la comparación. Lo encontró el test de
   edición; se corrigió en el agregado (`stored()`), que es el lugar más barato para que dominio y
   almacenamiento coincidan.

### Persistencia

Mismo molde que el repositorio de usuarios de T13, que ya había sido verificado adversarialmente:
`scoped()` que **se niega antes de tocar la base** en vez de caer al pool, **cero `WHERE tenant_id`**
en las queries (lo pone RLS), y los códigos del driver traducidos a centinelas del dominio:

| SQLSTATE | Se traduce a | Por qué |
| --- | --- | --- |
| `23505` en el índice del documento | `ErrDocumentTaken` | es un conflicto de negocio, no un error de driver |
| `23503` en una referencia al catálogo | `ErrDocumentInvalid` | "este país no tiene ese tipo" es lo que el llamador hizo mal |
| `42501` | `ErrInsufficientPrivilege` | un grant faltante no es un problema de scope |

El adaptador del catálogo corre **sin** scope de tenant a propósito: los catálogos son globales y
un caso de uso necesita normalizar el documento **antes** de abrir la transacción donde va a escribir.

**Migración `0006`**: `patients.country` pasa a `NOT NULL`. 0005 la dejó nullable porque un paciente
puede existir sin documento, pero el agregado siempre resuelve un país (elegido o heredado del
consultorio), así que un NULL solo podía significar una escritura que salteó el dominio. Moverlo al
esquema es lo que permite que el adaptador deje de tratar la columna como opcional.

Búsqueda: fragmento del nombre **o documento normalizado exacto**. Una coincidencia parcial sobre un
identificador es cómo la ficha de un paciente termina mostrándose por otro.

### Evidencia

- Dominio: 29 tests/subtests verdes, incluidos los seis algoritmos de dígito verificador con vectores
  reales (`20-12345678-6`, `529.982.247-25`, `11.222.333/0001-81`, `12345678Z`, `X1234567L`,
  `123456789`) y sus pares inválidos.
- Persistencia: 7 subtests de repositorio (alta y lectura con datos de país, documento duplicado,
  tipo inexistente para el país rechazado por la FK, scope ausente, invisibilidad cross-tenant,
  edición, filtros de listado) y 4 de catálogo.
- Suite completa con y sin DSN; `gofmt` limpio; `sqlc generate` idempotente.

### Efectos en el plan

`0006` corre la numeración otra vez: historia clínica pasa a `0007` (T24) y agenda a `0008` (T28).
Los tests de esquema de T20 y T20b se actualizaron porque `country` ahora es obligatorio — es la
consecuencia esperada de un invariante más fuerte, no una regresión.

Pendiente: T22 (HTTP de pacientes), T23 (frontend), T16 (tests de la capa HTTP) y T15 (matriz).

**L21 — E2/T22: casos de uso y HTTP de pacientes (commiteado)**

Dos cortes: capa de aplicación (`5182547`, delegada al writer) y HTTP con wiring (`5d467fb`, inline).
Fue el primer writer que aceptó la tarea desde T14.

### Lo que el worker entregó, verificado

Diez archivos en `internal/application/patient/`: seis casos de uso, `ProfileInput`/`PatientView` y un
`normalizeProfile` compartido. Verifiqué sin confiar en el reporte: `gofmt` limpio, `build`/`vet`
limpios, 18 tests verdes, y la propiedad que más importaba —**la normalización de catálogo corre antes
de `WithTenant`**— confirmada leyendo `create.go`: el único `WithTenant` envuelve el alta, y las
llamadas al catálogo viven en `normalizeProfile`, fuera.

`List` valida paginación en el caso de uso (rechaza `limit` fuera de 1..100 y `offset` negativo) en vez
de dejar que el driver devuelva `2201X`, que es lo que L10 había registrado.

### Lo que agregué en el corte de HTTP

Seis rutas, todas detrás del guard de rol de T19 y con el tenant saliendo del token. El mapeo de
errores es la parte que hace usable el formulario:

| Error | Respuesta |
| --- | --- |
| `*patient.ValidationError` | 422 con el campo |
| `patient.ErrDocumentInvalid` (tipo desconocido, dígito verificador) | 422 con el motivo |
| `patient.ErrDocumentTaken` | 409 |
| `patient.ErrNotFound` | 404 |
| `patient.ErrInvalidTransition` | 409 |

**No hay `DELETE`.** La baja es `POST /patients/{id}/deactivate`: la fila sobrevive porque la historia
clínica va a colgar de ella, y un `DELETE` que responde 204 estaría mintiendo sobre lo que pasó.

### Evidencia end-to-end (API real, base local, dos usuarios reales)

| Caso | Resultado |
| --- | --- |
| alta con `30.111.222` | 201, `number` tal cual se tipeó, `country=AR`, `isActive=true` |
| el mismo documento como `30111222` | 409 |
| NIE bajo AR (el catálogo no lo tiene) | 422 |
| NIE español con dígito verificador inválido | 422 |
| NIE español válido (`x-1234567-l`) | 201 |
| nombre en blanco | 422 |
| sin token / con `accountant` | 401 / 403 |
| detalle, PATCH y baja lógica | 200, 200 (`isActive=false`), 200 |
| listado con `activeOnly=true` tras la baja | 0 filas |
| reactivación | 200 (`isActive=true`) |
| id inexistente | 404 |
| `limit=500` | 422 |

### Un falso positivo que vale registrar

La primera pasada mostró que buscar `Lucía` devolvía 0 filas, y parecía un bug de búsqueda con acentos.
No lo era: los bytes guardados eran `efbfbd` (U+FFFD) porque **mi terminal mandó Latin-1, no UTF-8**.
Repetido con el cuerpo en UTF-8 crudo (`printf` con escapes), la fila se guardó como `c3b1` (ñ) y la
búsqueda acentuada devolvió su fila. Limpié las dos filas corruptas que había dejado en la base de
desarrollo.

Hallazgo real que queda anotado, de menor prioridad: **`encoding/json` reemplaza UTF-8 inválido por
U+FFFD en vez de rechazar el request**, así que un cliente que mande bytes mal formados guarda un
nombre corrupto en lugar de recibir un 400. Atajarlo exige validar el cuerpo crudo antes de decodificar;
queda para cuando haya más superficie de entrada, no para el MVP.

Pendiente: T23 (frontend de pacientes), T24 (historia clínica), T28 (agenda), T16 y T15.

**L22 — T16: HTTP layer tests (committed)**

Note on language: entries up to L21 were recorded in Spanish because the conversation was in
Spanish. Everything committed to the repository is now written in English (L19 of the Engram record
`aveniq/language-policy`), so this entry and the ones that follow are in English. Previous entries
stay as they were: they are history, not drafts.

### What was added

One file, `Backend/internal/interfaces/http/http_layer_test.go`, package `http_test`. It drives the
real router through `httptest` — real huma registration, real `requireRoles`, real `problem()` — with
in-memory doubles for `PatientRepository`, `DocumentCatalog`, `TenantScope`, `TenantRepository`,
`Clock` and `IDGenerator`.

Access tokens are minted directly with `authn.NewAccessTokens(...).Issue(...)` instead of going
through the login use case: the signed token is the contract the guard reads, and a test that had to
authenticate first would fail for two reasons at once.

Covered:

| Area | Cases |
| --- | --- |
| Authentication | no header on patients, create, tenants and me → 401; a token signed with a foreign key → 401 |
| Role guard | `accountant` → 403 on both domain routes, with no body leaked; `owner` → 200 |
| Cross-tenant isolation | a patient created with practice A's token is 404 by id and absent from the list under practice B's token, and readable by A |
| Error mapping | unknown document kind for the country → 422 naming the field; duplicate normalized document → 409; unknown id → 404; second deactivation → 409; editing a deactivated patient → 409; `limit=500` → 422 |
| Identity | `/auth/me` answers the tenant and role that came from the token |

### On evidence

**No RED was observed.** The tests were written against working code, so they are regression tests by
construction and passing proves nothing on its own. Verified by mutation instead: adding
`iam.RoleAccountant` to the allowed roles in `patient_handlers.go` makes
`TestDomainRoutesAreGuardedByRole` fail with `/api/v1/patients with accountant = 200, want 403`.
Reverted and green again.

That is the same gap L21 recorded for the UTF-8 false positive, and the reason the mutation step is
worth its two minutes: a test that has never failed is a hypothesis, not a guarantee.

### What this still does not cover

- `/auth/login`, `/auth/refresh` and `/auth/logout` at the HTTP level. Their use cases are covered by
  unit tests and the cookie helpers by `auth_cookie_test.go`, but the routes themselves are verified
  end-to-end only. Adding them needs doubles for the user and session repositories in this package,
  which is a slice of its own.
- The OpenAPI document is served but not asserted: no test fails if an operation loses its `Security`
  requirement or a schema drifts. Worth a small test when the frontend client is generated again.

### State of E1 and E2

E1 is now complete except T15 (permission matrix, invitations, trial sign-up), which L14 moved to E3.
E2 has the whole patient slice: schema (T20, T20b), domain and persistence (T21), application and HTTP
(T22), and the HTTP layer under test (T16). Remaining in E2: T23 (patient frontend), T24–T27 (clinical
record), T28–T31 (agenda), T32 (end-to-end verification).

**L23 — E2/T23: the patient screen (committed)**

`entities/patient` (types and labels), `features/patients` (table, form, detail) and
`app/patients` (the page), plus a link from the dashboard. `schema.d.ts` regenerated: it now carries
the six patient operations, which is what makes the screen type-check against the API instead of
against a hand-written copy of it.

### Product decisions the spec did not settle

- **The document kind select is filtered by the chosen country**, and changing the country drops a
  kind the new one does not have. The API would answer 422 anyway, but an offer that cannot be
  fulfilled is a bug in the offer, not in the server.
- **The document and the tax id are validated as pairs in the form** before the request. The server
  refuses a kind without a number; a receptionist should read that in the form, not as a failed
  request.
- **Deactivation lives in the detail panel**, not as a button per row: it is a deliberate act on a
  person's record, and a per-row button is one somebody clicks while scrolling.
- **Unknown document kinds render as the raw code.** The catalog grows by migration; a screen that
  hides what it does not recognise would show a patient with no document at all.
- The selected patient is re-read from the list after every change, so the detail panel cannot show
  stale data after an edit or a deactivation.

### Evidence and its limits

- `pnpm typecheck` clean after `pnpm build` regenerated the typed routes. Two errors were fixed on
  the way: `Link href="/patients"` needed the route map (same trap as T18), and `Notice` has no
  `neutral` tone.
- `pnpm build` clean; the route table now lists `/`, `/login` and `/patients`, all static.
- `GET /patients` served by `next start` → 200, and the HTML contains the session gate. That is the
  honest limit: without a browser the interactive flow — login, create, edit, deactivate — is not
  exercised. It is verified at the API level from L21 and by types at the client, not end to end
  through the UI.

Two environment notes from this session: the owner's dev server on port 3000 (PID 1272, started the
previous day) accepted the connection but never answered a request, and Next 16 refuses to start a
second dev server in the same directory, so the check ran against `next start` on 3001 instead. Worth
restarting that server before using the screen by hand.

**L24 — E2/T24: the clinical schema, and closing the foreign-key hole for good (committed)**

Migration `0007`. Two tables: `clinical_records` (1:1 with a patient: chief complaint, background,
objectives) and `clinical_notes` (one per encounter, with author and `encountered_at`). The split is
not cosmetic — mixing the stable frame with the running story is what makes a permission matrix
unable to say "the frame is administrative, the notes are clinical".

### The decision worth keeping

**The tenant travels inside the foreign keys.** A foreign key does not consult row-level security, so
`references patients (id)` would let a note hang off another practice's patient if someone knew the
UUID. That is the hole L10 recorded for sessions, and T14 had to close in Go with an explicit
visibility query.

For clinical data the guard belongs in the schema: `(tenant_id, patient_id) -> patients (tenant_id, id)`,
and the same for the author. It needed two unique indexes on the parents (`patients`, `users` on
`(tenant_id, id)`), added here.

This is the difference between a guarantee and a habit: the adapter can forget a visibility check; a
constraint cannot.

### Verified, including the mutation

`TestClinicalSchema`, 7 subtests, all green:

| Case | Result |
| --- | --- |
| application role writes both tables with zero GRANT statements | OK |
| a second record for the same patient | `23505` |
| a note pointing at another practice's patient | `23503`, and nothing written |
| a note signed by a user of another practice | `23503` |
| blank content | `23514` |
| tenant B sees A's clinical rows | 0 |
| no `app.tenant_id` | 0 rows on both tables |

**The mutation matters, and the first attempt was invalid.** Replacing the composite key with a
syntax error made the test fail for the wrong reason — the migration did not apply at all — which
proves nothing. The valid mutation, dropping the tenant from the key, produced exactly the failure the
test exists for:

```
clinical_schema_test.go:87: note pointing at a foreign patient: error = <nil>, want SQLSTATE 23503
--- FAIL: TestClinicalSchema/a_note_cannot_hang_off_another_practice's_patient
```

`error = <nil>` is the vulnerability itself: without the composite key the insert succeeds.

Also recorded in the schema, as comments, for the next reader: notes are never deleted (a correction
is a new note), and `encountered_at` is not constrained to the past because `now()` is not immutable
and a `CHECK` cannot express it — the domain validates that, not the database.

`goose up` / `down` / `up` verified on the local database; full suite with DSNs green; `sqlc generate`
idempotent (adds the two models).

### Next

T25 (clinical domain and repository), T26 (HTTP, which starts by giving `audit_log` a write path),
T27 (frontend), then the schedule in T28–T31 and T32 for the end-to-end pass.

**L25 — E2/T25: clinical domain and persistence (committed)**

`internal/domain/clinical` (record, note, errors), the `ClinicalRepository` port, the queries, the
adapter and its tests. `sqlc generate` adds `clinical.sql.go`.

### Where the invariants live, and why that split

| Invariant | Enforced by | Why not the other side |
| --- | --- | --- |
| One record per patient | the schema (`UNIQUE (tenant_id, patient_id)`) | it is a storage fact, and the adapter uses it as an upsert key |
| The patient and the author are in this tenant | the schema (composite FKs, 0007) | a foreign key cannot consult RLS, so the guard belongs in the key |
| Content not blank | both | the schema refuses it and the domain names the field |
| Author present | the domain | the token decides it; the column is NOT NULL, but "which claim" is not a schema question |
| **The encounter is not in the future** | **only the domain** | `now()` is not immutable, so a `CHECK` cannot express it. The schema comment says so, to stop the next reader from trying |
| Notes are never revised | the domain (no method) and the comment on the table | a delete would cascade clinical history away; an in-place edit erases who changed what |

### Two aggregates, one port

`ClinicalRepository` covers both, because the screen reads them as a unit and the writes share a
tenant transaction. Splitting it would create two seams that always move together — the kind of
abstraction that costs a decision every time it is touched and never pays for it.

### Verified

- Domain: 13 tests/subtests. **Mutation-checked**: removing the future-date check makes
  `TestNewNoteValidation/future_encounter` fail with `error = <nil>, want a ValidationError`. That is
  the one invariant no constraint can back up, so it is the one worth proving a test holds.
- Repository: 7 subtests — upsert creates once and updates after, notes append and list newest first
  with their author, a note cannot hang off another practice's patient (`ErrPatientNotVisible`), the
  rows are invisible across tenants, a missing record is `ErrNotFound`, everything is refused outside
  a tenant transaction, and a malformed id is a `ValidationError` naming the field rather than a
  driver error.
- Schema tests from T24 still green (7 subtests). Full suite with and without DSNs; `gofmt` clean;
  `sqlc` idempotent.

### Next

T26 (HTTP). It starts with the decision recorded in L10 and repeated in L24: `audit_log` cannot be
written by the application role when the row has no tenant (`42501`). Reading clinical data without a
trail is a compliance problem, and two routes are on the table — widen the policy, or a narrow
`SECURITY DEFINER` writer. The choice belongs to the owner, with the code in front of them.

**L26 — Audit log: correcting an overstated blocker, and a seeded development practice**

### The correction

L10, L24 and L25 all implied that `audit_log` "cannot be written by the application role". That is
imprecise, and the imprecision mattered: it made T26 look blocked behind a `SECURITY DEFINER`
function that nothing needs.

What the schema actually says (`0002`):

```sql
CREATE POLICY audit_log_tenant_isolation ON audit_log
    FOR ALL
    USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
```

- An INSERT whose `tenant_id` equals the tenant set for the transaction **passes**.
- An INSERT with `tenant_id IS NULL` **fails** with `42501`, because `NULL = <uuid>` is not true.

A clinical access event always has a tenant: the record belongs to a practice and the reader belongs
to that practice. So it writes today, inside the same `WithTenant` as the read. No policy widening, no
`SECURITY DEFINER`.

The NULL case existed for platform events — the schema even documents it: `actors` "may be a support
operator outside the tenant", and the foreign key is `ON DELETE SET NULL`. E2 has no platform events.

**One real consequence worth recording.** `ON DELETE SET NULL` means that deleting a practice turns
its audit rows into NULL-tenant rows, which then match no policy and become unreadable by anyone. The
trail of a deleted practice is lost. When deletion becomes a feature (E3 or later), it has to become a
soft delete on `tenants` or a dedicated archival path — not a cascade.

What T26 still needs, and it is smaller than advertised:

1. Write the audit row inside the same transaction as the clinical read, so an unaudited read cannot
   commit.
2. Decide failure semantics. Recommendation: fail closed — if the audit insert fails, the read fails.
   It runs on the same engine in the same transaction, so a failure there means the database is
   already broken.
3. Make `audit_log` append-only for the application role: `REVOKE UPDATE, DELETE`. Today the `FOR ALL`
   policy lets an application bug delete the trail it is supposed to leave.

### The seeded practice

`scripts/local-seed.ps1` creates the practice `demo` and an owner `demo@aveniq.local`, idempotently,
with a development password printed on screen. Not a migration: a seeded practice with a known
password must never ship to production, and migrations run wherever the schema runs.

The argon2id hash is embedded because PowerShell cannot compute it; it was minted with the production
parameters, so the seeded login exercises the real verification path. Verified end to end: login 200
with `role: owner`, wrong password 401, and the patients endpoint answers an empty roster for the new
practice.

The script is written as UTF-8 **with BOM**: PowerShell 5.1 reads a BOM-less file as ANSI and mangles
accents, which would have silently corrupted a practice name the day somebody types one with a tilde.

**L27 — E2/T26: clinical HTTP, and the audit trail that makes reading it accountable (committed)**

Two slices: the audit path (`771c80f`) and the clinical endpoints (`fbbc46b`).

### The audit trail, finished

`0008` revokes UPDATE and DELETE on `audit_log` from the application role. SELECT and INSERT stay. The
policy was `FOR ALL`, which meant an application bug could rewrite or erase the trail it is supposed to
leave; privileges are what make "append-only" true rather than intended.

**Mutation-checked**: removing the revoke makes the schema test report
`updating the audit log: error = <nil>, want SQLSTATE 42501` for both UPDATE and DELETE.

The adapter writes through the tenant transaction in the context, never through the pool. If the audit
insert had its own connection, a read could commit while its trail did not — and a read without a trail
is the exact thing the table exists to prevent. Fail closed is nearly free here: same engine, same
transaction, so a failure means the database is already broken.

### The endpoints

Four routes under the patient they belong to, because that is how the data is read and how the E3 matrix
will reason: access is decided per patient, not per table.

| Route | |
| --- | --- |
| `GET /patients/{id}/clinical-record` | the frame, audited as `clinical.read` |
| `PUT /patients/{id}/clinical-record` | upsert of the frame, audited as `clinical.write` |
| `GET /patients/{id}/clinical-notes` | one page, newest encounter first, audited as `clinical.read` |
| `POST /patients/{id}/clinical-notes` | append, audited as `clinical.write` |

**The author is not a body field.** It comes from the verified token; a body field with that name would
let a client sign a clinical note as somebody else. The request type simply has no such field, which is
a stronger statement than validating one.

### Evidence

End to end, against the real API on the seeded `demo` practice:

| Step | Result |
| --- | --- |
| create patient | 201 |
| `PUT` record | 200 |
| `GET` record | 200, `chiefComplaint=Insomnio` |
| `POST` note | 201, `authorId` equals the logged user |
| `GET` notes | 200, 1 item |
| note dated in the future | 422 `clinical: invalid encountered_at: must not be in the future` |
| no token | 401 |
| another practice | 404 |

And the trail, read straight from the table:

```
clinical.write clinical_record por 5ad0f7fc
clinical.read  clinical_record por 5ad0f7fc
clinical.write clinical_note   por 5ad0f7fc
clinical.read  clinical_note   por 5ad0f7fc
```

Every entry attributed to the caller, in the order the operations happened.

Tests: `internal/application/clinical` has four (read is audited once and shares the transaction; a
broken audit fails the read; the note author is the caller; pagination is validated here), plus
`TestAuditLogAppendOnly` with four subtests against the real database. Full suite with DSNs green,
`gofmt` clean, `sqlc` idempotent.

### What this still does not do, deliberately

- **No per-role authorization.** Any owner, admin or professional of a practice can read every patient's
  clinical record of that practice. The permission matrix (T15, E3) is what will say "a professional
  sees their own patients"; until then the guarantee is "within the practice", not "within the
  treatment".
- Reading is audited; *which fields* were shown is not. The event records the entity and its id, not a
  summary of the content. That is enough to answer "who looked", which is what the trail is for.

**L28 — E2/T27: the clinical screen (uncommitted)**

`entities/clinical` (wire types and date helpers), `features/patients/clinical-panel` (ficha +
evoluciones) and a tab on the existing patient detail. `schema.d.ts` regenerated so the screen
type-checks against the four clinical operations, not a hand-written copy.

### Product decisions the spec did not settle

- **Historia clínica is a tab on the patient, not a route.** Access is decided per patient; a
  second page would be a second place to forget the session gate.
- **The clinical panel mounts only when that tab is opened.** Opening the administrative data of a
  patient must not write `clinical.read`. Switching patients resets to Datos so the previous
  record cannot linger.
- **404 on the frame is an empty form**, not an error. The first save creates the row. Any other
  status is a real problem.
- **Notes have no edit and no author field.** A correction is a new note. The encounter date is
  `datetime-local`, converted to RFC 3339; a future date is refused in the form before the 422.
- **Author UUIDs are not shown.** There is no user-name API yet (E3). Displaying the raw id would
  look like identity without being one.

### Evidence

- `pnpm typecheck` clean after `pnpm api:types` against the running API.
- Interactive flow (login, open a patient, save the frame, append a note) is verified in the
  browser against the local stack.

Pendiente de E2: T28–T31 (agenda), T32 (E2E). T15 sigue en E3.

**L29 — E2/T28–T31: agenda (uncommitted)**

The schedule is migration `0009` because `0008` already made `audit_log` append-only. `tenants.timezone`
defaults to `America/Argentina/Buenos_Aires`. Occurrences are rows; overlap is
`EXCLUDE USING gist` on live (non-cancelled) slots of the same professional. Series expansion
runs in the practice timezone so "every Tuesday at 18:00" does not slide on DST.

HTTP: `GET/POST /appointments`, attend/cancel, `POST/PATCH /appointment-series` and cancel remaining
scheduled occurrences. A series edit deletes future *scheduled* rows and rematerializes; attended
slots stay (L14).

Frontend: `/agenda`, week grid for the signed-in professional, one-off booking and weekly series.
`schema.d.ts` regenerated from the new OpenAPI.

### Evidence

- `go test` of domain, application, HTTP layer, and postgres schema tests (with DSNs) green.
- `pnpm typecheck` clean.
- Local `goose up` applied `0009` on the hermetic stack.

Pendiente de E2: T32 (E2E). T15 sigue en E3. The API process on :8080 must be restarted to serve
the new routes.

**L30 — E2/T32: Playwright journeys and OpenAPI client (uncommitted)**

Playwright against the local stack (`pnpm e2e`): login, two tabs calling
`window.__aveniqEnsureSession` in parallel without killing the session, then alta of a
patient, ficha + evolución, and a one-off booking that shows on the week grid.

The access token stays in memory; a cookie-only `GET /auth/me` from Playwright is 401
and is not a session proof. The two-tab proof is both tabs still showing "Cerrar sesión".
The booking must land inside the visible Monday–Sunday window — a +3 day slot from
Friday is next week and never appears. The patient name in the hidden `<option>` is not
a visible locator.

`schema.d.ts` regenerated from `http://127.0.0.1:8080/openapi.json` (`pnpm api:types`).
The hook `__aveniqEnsureSession` is assigned only when `NODE_ENV !== "production"`.

### Evidence

- `pnpm typecheck` clean.
- `pnpm e2e` 3 passed (Chromium): login, two-tab refresh, patient + clinical + agenda.

E2 (T17–T32) is closed on this machine. T15 remains in E3. Not on CI: the journeys need
Postgres, Redis, the API, the seed, and a browser.

**L31 — Decisiones de E3 (antes de codar)**

Tres decisiones tomadas en AskQuestion, registradas como S18–S20:

- Un professional ve **solo sus pacientes y sus turnos** (S18). Esto cambia el alcance de la historia
  clínica: deja de ser "dentro del consultorio" y pasa a ser "dentro de la relación del profesional con
  el paciente". Hay que definir qué pasa con un paciente que atienden dos profesionales.
- Existe el papel `assistant` con agenda y cadastro, **sin historia clínica** (S19).
- Los convites son **por enlace** y no hay email automático (S20). Por eso la verificación de la alta
  en trial (T36) queda abierta: no hay proveedor de correo.

Tareas T33–T38 en la tabla. T33 (matriz) va primero: T34–T38 dependen de los permisos que defina.

**L32 — Respuestas de E3: ficha compartida, HC con autorización, accountant solo facturación**

Dos decisiones más, verbatim en S21 y S22:

- **Ficha de identificación compartida** (nombre, ID, DNI, CUIT) dentro del consultorio. Es lo que
  cualquiera con acceso a pacientes puede ver, incluido el `assistant`.
- **Historia clínica solo con autorización** (S21). Esto desdobla el permiso de paciente: ver que
  existe y cómo se llama no es lo mismo que leer sus evoluciones. Un `assistant` ve la ficha de
  identificación y no la HC, en línea con S19.
- **`accountant` ve solo facturación** (S22). No ve agenda ni HC. Esto se suma a lo que T33 tiene que
  cerrar: la facturación (S2) todavía no existe, así que el papel queda con permisos vacíos hasta que
  exista ese dominio.

La autorización de HC queda definida en S23 (ver L33). Antes de T33 solo falta decidir si el paciente
también tiene que consentir (la respuesta del usuario apunta al profesional tratante como único
concedente, así que no hay paso del paciente en E3).

**L33 — Autorización de HC (S23)**

- La concede el **profesional tratante** a otro profesional del consultorio. El paciente no aparece en
  el flujo de E3.
- El **tratante ve la HC de sus pacientes por defecto**. La autorización es para terceros, no para él.
- Es **revocable sin plazo**. No hay fecha de vencimiento.

Consecuencias para el modelo de datos (T33–T34):

- Una tabla de concesiones `(tenant, paciente, profesional concedido, concedida por, revocada_at)`,
  con RLS forzado como toda tabla nueva (L14, regla 1).
- Ver la HC = el profesional es tratante del paciente **o** tiene una concesión activa para ese paciente.
- `assistant` y `accountant` nunca leen HC, con o sin concesión (S19, S22).
- La revocación no borra nada: se marca `revocada_at`, para mantener el rastro en la auditoría.

Snyk: el usuario indicó que no se usa en este proyecto. No se corre `snyk_code_scan` ni `snyk_auth`
en Aveniq.

**L34 — E3/T33: the permission matrix replaces the coarse role guard (uncommitted)**

`requireRoles(owner, admin, professional)` is gone. Domain routes ask `iam.Allows` /
`iam.AllowsAny` through `requirePermission`. The identity still comes from the token;
the matrix only answers "does this role have this verb".

Migration `0010` adds `schedule:read/write`, `tenant:read` and `clinical:grant`, and
drops `billing:read` from professional and assistant (S19, S22). Admin still has no
`clinical:*`. Owner still holds every catalog row.

| Role | Identification | HC | Agenda | Billing | Practice |
| --- | --- | --- | --- | --- | --- |
| owner | read/write | any + grant | read/write | all | read + settings |
| admin | read/write | — | read/write | read | read + settings |
| professional | read/write | own + grant | read/write | — | read |
| assistant | read/write | — | read/write | — | read |
| accountant | — | — | — | all | — |

Instance-level (which patient is "own", the grant table) is T38. `clinical:grant` is
seeded so that cut does not need another catalog change.

### Evidence

- `go test ./internal/domain/iam/` and `./internal/interfaces/http/` green.
- `TestPermissionMatrixDrift` green against a scratch database with the real migrations.
- HTTP: assistant lists patients (200) and is refused on HC (403); admin is refused on
  HC; accountant is refused on patients, tenants, clinical and appointments.

Pendiente de E3: T35 (invites), T36 (trial signup), T37 (frontend), T38
(instance-level + E2E). The running API process must be restarted to pick up the guard.

**L35 — E3/T34: cuentas del consultorio, directorio de autores y cambio de acceso (sin commit)**

El alta no entra en T34. `users.password_hash` es `NOT NULL` y solo el invitado puede elegir su
contraseña, así que una cuenta nace al aceptar una invitación (T35). Crear un usuario desde el
panel sin ese paso obligaría a inventar una contraseña temporal o a relajar el esquema.

Rutas (`0011` agrega `users:directory`):

| Ruta | Permiso | Quién |
| --- | --- | --- |
| `GET /users` | `users:read` | owner, admin |
| `GET /users/directory` | `users:directory` | owner, admin, professional, assistant |
| `GET /users/{id}` | `users:read` | owner, admin |
| `PUT /users/{id}/role` | `users:manage` | owner, admin (reglas abajo) |
| `POST /users/{id}/deactivate`, `/activate` | `users:manage` | owner, admin (reglas abajo) |

`users:directory` existe para que un profesional vea **quién** escribió una evolución sin ver los
emails de sus colegas. Es nombre, id y papel, nada más.

Reglas de cambio (`iam.AuthorizeChange`), que son la respuesta a "quién puede dejar el consultorio
sin dueño":

1. **Nadie cambia su propio acceso.** El único owner no puede degradarse; por eso el consultorio
   siempre conserva un owner. Un admin tampoco se cambia a sí mismo.
2. **Solo un owner otorga o toca el papel owner.** Un admin gestiona el personal de abajo y no
   reparte ni quita la cima.

Cada cambio corre en una sola transacción de tenant: carga, autoriza, persiste y escribe en la
auditoría (`user.role_changed` con `from`/`to`, `user.deactivated`, `user.activated`). Si la
auditoría falla, el cambio se revierte.

Errores: 403 con mensaje fijo cuando la regla lo impide (no dice qué regla), 409 si la cuenta ya
tiene ese papel o ya está en ese estado, 404 para cuentas de otro consultorio.

Lo que T34 **no** hace y queda explícito: filtrar el listado por papel o por nombre, y regenerar el
cliente OpenAPI (`schema.d.ts`), que pasa a T37 cuando el frontend lo consuma.

### Evidencia

- `go test ./internal/domain/... ./internal/application/... ./internal/interfaces/...` verde.
- `AuthorizeChange`: 7 casos (propio acceso, owner por admin, owner por owner, admin cediendo owner).
- HTTP: assistant 403 en `/users` y 200 en directorio; el directorio no trae email ni estado; accountant
  403 en directorio y detalle; cuenta ajena 404; trail con `from>to` y actor.
- `0011` aplicada en la base local; `TestPermissionMatrixDrift`, `TestRoleCatalogDrift` y
  `TestIsolation` verdes con base real.
- `go build ./...` limpio.

**L36 — E3/T35: invitaciones y aceptación con contraseña (sin commit)**

Una invitación es una oferta pendiente, no una cuenta. Solo al aceptarla nace la fila en `users`,
con la contraseña que elige el invitado (hash argon2id). Sin email: el owner o admin copia el
enlace de la pantalla (S20).

Decisiones:

- **Token** `inv1.<tenant-uuid>.<43 base64url>`, 32 bytes aleatorios. La base guarda el SHA-256 de
  la cadena completa. El segmento de tenant es solo una pista para abrir el scope correcto: el mismo
  secreto bajo otra práctica no coincide con ninguna fila.
- **Vigencia**: 7 días (`iam.InvitationTTL`).
- **Contraseña**: entre 12 y 256 caracteres, sin reglas de composición (`iam.ValidatePassword`).
  Violarla responde 422 y el enlace sigue vivo para reintentar.
- **Quién invita**: `users:manage` (owner, admin). Solo un owner invita como owner o retira una
  invitación de owner (`iam.AuthorizeInvitation`). Mismo criterio que cambiar papeles.
- **Email ya con cuenta en el consultorio**: 409 al **crear**, no al aceptar, para que el owner lo
  sepa mientras puede retirar la invitación.
- **Enlace público** (sin sesión): `GET /invitations/preview?token=` y `POST /invitations/accept`.
  Malformado, desconocido, aceptado, vencido o de otra práctica responden **410** con el mismo
  mensaje. Distinguirlos le diría a un extraño qué enlaces son reales.
- **Uso único**: el guard vive en el `UPDATE` condicional (`accepted_at IS NULL AND expires_at > $now`),
  no en Go. Aceptar crea la cuenta y marca la invitación en la misma transacción de tenant: si la
  marca pierde la carrera, la cuenta se revierte. Test real: 8 aceptaciones simultáneas, exactamente
  una gana.
- **Aceptar no inicia sesión**: responde 201 con `userId`, `email` y `role`, y el invitado entra
  después con la contraseña recién elegida. Así el flujo público no emite tokens ni cookies; si el
  frontend quiere entrar de una vez, se resuelve en T37 con el login existente.
- **Revocar**: borra una invitación pendiente (204). Una aceptada no se revoca (409) y queda como
  historia de quién entró y cuándo.
- **Autor eliminado**: `invited_by` pasa a `NULL` (`ON DELETE SET NULL`) y la invitación sigue
  vigente. Por eso `ReconstituteInvitation` no exige autor.
- **Auditoría** en la misma transacción: `invitation.created` y `invitation.revoked` (con `role`),
  `invitation.accepted` (con `role` y `user_id`, el actor es la cuenta nueva).

Piezas: `0002` ya tenía la tabla `invitations` con RLS forzado, así que **no hay migración nueva**.
Se agregaron `queries/invitations.sql` (sqlc regenerado), `port.InvitationRepository`, el adaptador
`postgres.InvitationRepository`, el paquete `application/invitation`, `invitation_handlers.go`,
`authn.NewInvitationToken`/`ParseInvitationToken`, y en `problem()` los casos `ErrEmailTaken` → 409 e
`ErrInvitationUnusable` → 410.

Lo que T35 **no** hace: rate limit de las rutas públicas (T36, junto con el trial signup que comparte
la exposición), envío por email, ni pantallas (T37).

### Evidencia

- `go vet ./...` y `gofmt -l` limpios; `go test ./... -count=1` verde.
- Dominio (`invitation_test.go`): normalización del email, vigencia exacta, aceptada no reutilizable,
  matriz de `AuthorizeInvitation`, reconstitución sin autor, política de contraseña en los bordes.
- Token (`invitation_token_test.go`): ida y vuelta, unicidad, hash que cubre el tenant, 5 formas
  inválidas.
- HTTP (`invitation_routes_test.go`): ciclo completo, reglas por papel, enlace de otra práctica
  rechazado, revocación, expiración y contraseña corta.
- Postgres real (`invitation_repository_test.go`): aislamiento entre prácticas, escopo obligatorio,
  carrera de aceptaciones, historia inmutable y convite que sobrevive a la baja de su autor.

Pendiente: reiniciar la API (`go run main.go`) para cargar las rutas de invitación. T36 (trial
signup y rate limit de rutas públicas), T37 (pantallas y `schema.d.ts`), T38 (reglas de instancia y E2E).
