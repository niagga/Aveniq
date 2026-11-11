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
| T14 | S3 | E1 | Auth: argon2id, login/refresh/logout con rotación y detección de reuso, middleware | inline | `6a3f9eb`, `1905de1`, `3b09615`, `5f50e49`, `98f2dd3`, `011d1ca` |
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
