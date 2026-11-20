# Shell de interfaz — rail de iconos + panel contextual (dirección C)

Feature de UI del frontend Next.js. Implementa el artboard `design/shell-comparacion/03-c-rail-dos-niveles.html` como shell único de las pantallas privadas, con modo oscuro y diseño responsive. El sistema de tokens vive en `design/system.md`; el artefacto visual aprobado es el canvas `design/shell-comparacion/`.

No toca Backend ni contratos de API. El estado de sesión, permisos y trial siguen siendo autoridad del API; el shell sólo refleja lo que ya se lee hoy.

## Specs

- **S1** — Dirección aprobada: `Vamos avanzar con C`. El shell de las pantallas privadas es un rail de iconos de 64 px + un panel contextual de 300 px propiedad de la pantalla + el área de contenido. Reemplaza los botones repetidos `← Volver al consultorio` por una navegación única en el layout.
- **S2** — `Opción de Dark-Theme`. El producto ofrece tema oscuro conmutable. Los tokens oscuros son una propuesta derivada del set claro y viven en un solo lugar; el tema claro no cambia de apariencia.
- **S3** — `Diseño responsive`. El shell reflowa por ancho: ≥ 1080 px rail + panel + contenido; 900–1080 px el panel contextual se oculta y sus vistas vuelven a chips; < 736 px el rail pasa a barra inferior, las columnas secundarias de tabla se ocultan y la acción primaria se mantiene.
- **S4** — `La agenda mono-profesional es por falta de UI`. `/agenda` debe ofrecer selector de profesional para que dueño, administración y recepción no vean una semana vacía, y debe mostrar un estado de carga real en lugar de afirmar `Sin turnos` mientras carga.
- **S5** — Requisito heredado del diagnóstico aprobado: ninguna acción destructiva queda sin confirmación explícita, y todo control tiene foco visible por teclado. Hoy hay 6 acciones destructivas sin confirmar y cero `:focus-visible` en `Frontend/src`.

## Tasks

| ID | Spec | Tarea | Route |
| --- | --- | --- | --- |
| T1 | S2, S5 | Tokens de sistema: `--radius-control`, `--color-border-strong`, bloque oscuro y regla base de foco visible | inline |
| T2 | S1 | Shell: componente `AppShell` con rail, layout privado en route group y migración de la home | worker |
| T3 | S1, S5 | Primitivas compartidas: `Button`, `Field` único, `ConfirmDialog`, `EmptyState`, `LoadingState`, `Badge` | worker |
| T4 | S1, S3 | Migrar `/facturacion` al shell con panel contextual de vistas | worker |
| T5 | S1, S3 | Migrar `/patients`, `/tarifario`, `/mutuales`, `/usuarios` al shell | worker |
| T6 | S4 | Selector de profesional y estado de carga real en `/agenda` | worker |
| T7 | S2 | Control de tema en la UI, persistencia y respeto de `prefers-color-scheme` | worker |
| T8 | S1, S2, S3, S5 | Barrido responsive y de accesibilidad, y actualización de los e2e afectados | worker |
| T9 | S1 | Mover el aviso de trial al pie del rail: `PracticeProvider` lo inyecta global hoy y el shell necesita leer el consultorio | worker |
| T10 | — | **Requiere autorización aparte.** `GET /billing/unbilled` en el Backend: sesiones atendidas sin comprobante, con total y rango. Sin ese endpoint el panel de Facturación no puede mostrar contadores reales y el artboard sólo se cumple en estructura | Backend + Frontend |

### Evidencia de commits

| Task | Commit |
| --- | --- |
| T1 | `040b9e0` — tokens de control, valores oscuros y foco visible |
| T2 | `4018b41` — shell con rail, layout privado y fin de los wrappers duplicados |
| T3 | `6af924e` — primitivas `Button`, `ConfirmDialog`, `EmptyState`, `LoadingState`, `Badge` y error inline en `Field` |
| T7 | `b6aa2a1` — interruptor de tema con script sin flash y verificación de capas sobre el CSS compilado |
| T4 | `e72dc15` — filtros en la URL, panel contextual de vistas y primitivas en uso |
| T6 | `d078096` — agenda de todo el consultorio, selector de profesional y estado de carga real |

## Log

- **L1** — Pedido original del usuario: `Me gustaria que tu analizes el sistema completo y seas tu quien me de sugerencias de que pantallas modificar y que modificaciones hacer.`
- **L2** — Corrección de método y requisitos nuevos: `No consigo definir, las 3 pantallas tienen diferentes estilos? / No es mejor que me des la misma pantalla con los estilos diferentes para poder definirlos, o las tres pantallas, con cada estilo para ver yo 3 pantallas diferentes. / Adicionalmente, quiero 2 cosas: / - Opción de Dark-Theme / - Diseño responsive`
- **L3** — Respuesta a la única pregunta abierta del diseño: `La agenda mono-profesional es por falta de UI`. Queda como defecto de UI a corregir (S4), no como decisión de producto.
- **L4** — Elección de dirección: `Vamos avanzar con C`. Se registra en el canvas con `gentle_design action "implement"` sobre `design/shell-comparacion/03-c-rail-dos-niveles.html`.
- **L5** — Decisión de implementación: route group `app/(private)/` con layout propio, en lugar de repetir `SessionGate` en cada pantalla. Los tres flujos públicos (`/login`, `/registro`, `/invitacion`) quedan fuera del shell.
- **L6** — Evidencia de entorno: `Frontend/node_modules` estaba incompleto (instalado con pnpm, sin `@playwright/test`), así que `typecheck` y `build` no corrían antes de este trabajo. `Frontend/pnpm-lock.yaml` está trackeado y se reinstala con `pnpm install --frozen-lockfile`, que no puede reescribirlo. Ninguna tarea se marca hecha sin `typecheck` y `build` en verde después de esa reinstalación.
- **L7** — Trabajo sobre rama `feat/shell-interfaz`, creada desde `main`. `pnpm install --frozen-lockfile` instaló lo que faltaba sin tocar `pnpm-lock.yaml` (verificado con `git diff`); `tsc --noEmit` y `pnpm build` quedaron en verde con las 11 rutas intactas antes de commitear T1.
- **L8** — `gentle-ai-worker` no arrancó para T2: `Writer repository preparation was unavailable or its session/target changed`, dos veces, con árbol limpio y `git worktree list` normal. Es una falla del harness, no del repo. T2 se ejecuta inline y las superficies de edición se ampliaron a `Frontend/src/features/*/components/*-screen.tsx`, porque ahí vivían 4 de los 6 `← Volver al consultorio`; sin eso el criterio de aceptación (cero ocurrencias) no se cumple.
- **L9** — Los e2e dependen de los links del dashboard (`e2e/e2.spec.ts:13`, `e2e/e4-billing.spec.ts:173-176`). Por eso la fila de 6 links se **elimina** del dashboard en lugar de convivir con el rail: duplicada, `getByRole("link", { name: "Pacientes" })` resuelve a dos elementos y falla por strict mode. El rail conserva exactamente los mismos nombres accesibles y el mismo gating por rol.
- **L10** — Los e2e siguen **sin ejecutarse**: no hay navegadores de Playwright instalados en esta máquina (`~/AppData/Local/ms-playwright` vacío) aunque el backend conteste en `127.0.0.1:8080`. Ninguna tarea de esta feature se apoya en e2e como prueba de cierre mientras siga así; las verificaciones reales son `pnpm build`, `tsc --noEmit` y los greps de aceptación. Instalar navegadores es una decisión pendiente del usuario, no algo que este trabajo haga por su cuenta.
- **L11** — Alcance deliberado de T3: las primitivas quedan escritas y las dos *definiciones* duplicadas de estilo de input se eliminan, pero el barrido de los ~15 inputs con clases escritas dentro de JSX y del `Field` local de `clinical-panel` se hace con cada pantalla (T4/T5), donde el layout alrededor es revisable. Los conteos al cerrar T3: una sola definición de `inputClass` en el repo, dos `Field` locales pendientes, cero usos de `Button`/`ConfirmDialog`/`Badge` todavía.
- **L12** — T4 camino A (sin Backend). Los filtros pasan a la URL con las claves `estado`/`desde`/`hasta` y el `voucher` se suma en vez de reemplazarlos; `billingHref`/`billingUrl` y `VoucherTable` aceptan filtros para que abrir y cerrar un comprobante no borre la vista. El panel contextual muestra las vistas **sin contadores**, porque el agregado de sesiones sin comprobante no existe en el Backend: es T10 y no se inventa. Facturación pasa a ser dueña de su `ScreenFrame` porque el panel depende de su estado. Bajo `md` quedan chips y un `<details>` con el rango: evita una regresión, no es el barrido responsive de T8. e2e sin ejecutar (sin navegadores); `e4-billing.spec.ts` no toca los filtros, verificado por grep sobre el spec, no por ejecución.
- **L13** — El API local en `127.0.0.1:8080` **es más viejo que el código**: responde 200 en `/patients`, `/tenants` y `/auth/login`, y **404** en `/appointments`, `/vouchers`, `/tariffs`, `/insurers` y `/users/directory`. Consecuencia doble: las pantallas de agenda, facturación, tarifario, mutuales y usuarios fallan en runtime aunque el login funcione, y la premisa de T6 no se puede verificar ejecutando. Se verificó en las reglas de dominio (`internal/domain/care/rules.go:87,115`) y queda pendiente el chequeo real después de reiniciar el backend (`cd Backend; go run ./cmd/api`, README:95).
- **L14** — Decisión del usuario para S4: dueño, administración y recepción abren **todos los profesionales por defecto**, con el nombre en cada turno y un selector para acotar. Un profesional sigue abriendo su propia semana y no ve selector. El alta de turno sigue necesitando un profesional concreto, así que las formas reservan para el filtrado o, en «Todos», para el primero de la lista.
