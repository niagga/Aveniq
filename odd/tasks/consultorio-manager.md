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
| T16 | S5 | E1 | Test de fuga cross-tenant en CI y tests de la capa HTTP | worker | pendiente |
| T17 | S3 | E2 | API: el refresh token viaja en cookie `HttpOnly`; login/refresh/logout la setean y la limpian | inline | `6930a07` |
| T18 | S3, S12 | E2 | Frontend: login, guard de sesión y refresco silencioso | inline | `34dbd60` |
| T19 | S5, S11 | E2 | API: tenant desde el token y guard grueso por rol en las rutas de dominio | inline | `aa422af`, `51d4779` |
| T20 | S1, S16 | E2 | Migración `0004`: pacientes con datos fiscales, unicidad de documento por tenant, RLS forzado | inline | `2cf9dd7` |
| T21 | S1, S11 | E2 | Dominio `patient` + repositorio sqlc + tests de aislamiento | — | — |
| T22 | S1, S11 | E2 | HTTP de pacientes (alta, listado con búsqueda, detalle, edición, baja lógica) | — | — |
| T23 | S1, S12 | E2 | Frontend: pacientes (listado, alta, detalle, datos fiscales) | — | — |
| T24 | S1, S16 | E2 | Migración `0005`: ficha clínica 1:1 con el paciente + evoluciones, RLS forzado | — | — |
| T25 | S1, S11 | E2 | Dominio `clinical` + repositorio sqlc + tests de aislamiento | — | — |
| T26 | S1 | E2 | HTTP de historia clínica (ficha y evoluciones; autoría desde el token, nunca del cuerpo) | — | — |
| T27 | S1, S12 | E2 | Frontend: ficha clínica y evoluciones | — | — |
| T28 | S15, S5 | E2 | Migración `0006`: turnos, series recurrentes, zona horaria del tenant, exclusión de solapamiento | — | — |
| T29 | S15, S11 | E2 | Dominio `schedule` + repositorio sqlc (solapamiento, expansión de series, estados) | — | — |
| T30 | S15 | E2 | HTTP de agenda (turnos por rango, alta, cambio de estado, series) | — | — |
| T31 | S15, S12 | E2 | Frontend: agenda semanal por profesional | — | — |
| T32 | S4, S11 | E2 | Verificación end-to-end de E2 y regeneración del cliente OpenAPI | — | — |

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
