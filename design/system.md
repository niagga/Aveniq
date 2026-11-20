# Aveniq — sistema de diseño

Sincronizado desde el código: `Frontend/src/app/globals.css` es el único archivo que declara tokens. No hay librería de componentes (las dependencias son `next`, `react`, `react-dom`, `openapi-fetch`) y sólo existen dos primitivas compartidas: `Frontend/src/shared/ui/notice.tsx` y `Frontend/src/shared/ui/field.tsx`.

Regla vigente del proyecto, citada en el propio CSS: los componentes referencian estos nombres, nunca hex crudos.

## Colores

| Token | Valor | Uso observado | Fuente |
| --- | --- | --- | --- |
| `--color-surface` | `#ffffff` | Fondo de paneles, tablas e inputs | `Frontend/src/app/globals.css:8` |
| `--color-surface-muted` | `#f6f7f9` | Fondo de página (`body`) y de cabeceras de tabla | `Frontend/src/app/globals.css:9` |
| `--color-border` | `#e3e6ea` | Bordes de panel, separadores de fila, bordes de control | `Frontend/src/app/globals.css:10` |
| `--color-ink` | `#16181d` | Texto principal | `Frontend/src/app/globals.css:11` |
| `--color-ink-muted` | `#5c6472` | Texto secundario, labels, metadatos, cabeceras de tabla | `Frontend/src/app/globals.css:12` |
| `--color-brand` | `#2f5d8a` | Acción primaria, foco de input, fila seleccionada | `Frontend/src/app/globals.css:13` |
| `--color-brand-ink` | `#ffffff` | Texto sobre `--color-brand` | `Frontend/src/app/globals.css:14` |
| `--color-success` | `#1f7a4d` | Estado activo, confirmaciones | `Frontend/src/app/globals.css:15` |
| `--color-warning` | `#8a5a00` | Avisos y trial por vencer | `Frontend/src/app/globals.css:16` |
| `--color-danger` | `#a4232b` | Errores y acciones destructivas | `Frontend/src/app/globals.css:17` |

## Tipografía

| Declaración | Valor | Uso observado | Fuente |
| --- | --- | --- | --- |
| `font-family` (body) | `ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` | Toda la aplicación; no hay fuente de marca | `Frontend/src/app/globals.css:28-34` |

No existen tokens `--font-*` ni escala tipográfica declarada. Los roles salen de utilidades repetidas en los componentes:

| Rol | Utilidad observada | Ejemplo |
| --- | --- | --- |
| Título de página | `text-2xl font-semibold tracking-tight` | `Frontend/src/app/page.tsx:68` |
| Título de sección | `text-sm font-semibold` | `Frontend/src/features/billing/components/billing-screen.tsx:195` |
| Cuerpo y celdas | `text-sm` | `Frontend/src/features/patients/components/patient-table.tsx:33` |
| Label de control | `text-xs font-medium text-ink-muted` | `Frontend/src/shared/ui/field.tsx:23` |
| Metadatos y cabecera de tabla | `text-xs uppercase tracking-wide text-ink-muted` | `Frontend/src/features/patients/components/patient-table.tsx:37` |
| Datos fiscales y numéricos | `font-mono text-xs` | `Frontend/src/features/patients/components/patient-table.tsx:66` |

## Espaciado

No hay tokens de espaciado declarados. La escala activa es la default de Tailwind v4 (base 4 px). Ritmo observado en el código:

| Patrón | Valor efectivo | Uso observado | Fuente |
| --- | --- | --- | --- |
| `px-4 py-3` | 16 px / 12 px | Celda de tabla y cabecera | `Frontend/src/features/patients/components/patient-table.tsx:40` |
| `px-3 py-2` | 12 px / 8 px | Input de formulario | `Frontend/src/shared/ui/field.tsx:4` |
| `px-4 py-3` | 16 px / 12 px | Bloque de aviso | `Frontend/src/shared/ui/notice.tsx:21` |
| `px-3 py-1.5` | 12 px / 6 px | Botón secundario | `Frontend/src/app/page.tsx:77` |
| `px-6 py-12` | 24 px / 48 px | Frame de página | `Frontend/src/app/page.tsx:36` |
| `gap-6` | 24 px | Separación entre bloques de página | `Frontend/src/app/page.tsx:36` |
| `gap-3` | 12 px | Separación entre secciones internas | `Frontend/src/features/users/components/users-screen.tsx:110` |

Anchos de contenedor en uso — inconsistentes: `max-w-md` (login, registro, invitación), `max-w-5xl` (home), `max-w-6xl` (pacientes, usuarios, tarifario, mutuales, facturación), `max-w-7xl` (agenda).

## Radio

| Token | Valor | Uso observado | Fuente |
| --- | --- | --- | --- |
| `--radius-panel` | `0.75rem` | Paneles, tablas, avisos | `Frontend/src/app/globals.css:19` |
| (sin token) `rounded-md` | `0.375rem` | Botones e inputs — crudo, en ~60 sitios | `Frontend/src/shared/ui/field.tsx:4` |
| (sin token) `rounded-full` | `calc(infinity * 1px)` | Badges de estado | `Frontend/src/features/patients/components/patient-table.tsx:74` |

Deuda conocida: el radio está partido entre el token y el valor crudo. Un solo token para controles lo unifica.

## Sombras

No se declara ninguna sombra en el sistema. La elevación se expresa hoy sólo con `border` + diferencia entre `--color-surface` y `--color-surface-muted`.

## Componentes

| Componente | Clases canónicas | Fuente |
| --- | --- | --- |
| Botón secundario | `rounded-md border border-border px-3 py-1.5 text-xs font-medium` | `Frontend/src/app/page.tsx:77` |
| Input | `mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand` | `Frontend/src/shared/ui/field.tsx:4` |
| Label de campo | `block text-xs font-medium text-ink-muted`, con `htmlFor` obligatorio | `Frontend/src/shared/ui/field.tsx:23` |
| Panel de tabla | `overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface` | `Frontend/src/features/patients/components/patient-table.tsx:38` |
| Cabecera de tabla | `bg-surface-muted text-xs uppercase tracking-wide text-ink-muted`, `th` en `px-4 py-3 font-medium` | `Frontend/src/features/patients/components/patient-table.tsx:37-50` |
| Fila | `border-t border-border`; seleccionada: `bg-brand/5` | `Frontend/src/features/patients/components/patient-table.tsx:55-58` |
| Badge de estado | `rounded-full border px-2 py-0.5 text-xs` + tono (`success` activo, `border`+`ink-muted` inactivo) | `Frontend/src/features/patients/components/patient-table.tsx:72-81` |
| Aviso | `rounded-[var(--radius-panel)] border px-4 py-3 text-sm`; tonos info/warning/danger con `bg-*/5` y `border-*/30` | `Frontend/src/shared/ui/notice.tsx:4-21` |
| Frame de página | `mx-auto flex min-h-full w-full max-w-* flex-col gap-6 px-6 py-12` | `Frontend/src/app/page.tsx:36` |

## Estados de interacción

| Estado | Implementación actual | Fuente |
| --- | --- | --- |
| Foco | Sólo `focus:border-brand` en inputs; **no existe `:focus-visible` ni `ring-*` en todo `Frontend/src`** | `Frontend/src/shared/ui/field.tsx:4` |
| Deshabilitado | Sin regla de sistema; cada formulario resuelve `busy`/`writesLocked` a mano | `Frontend/src/features/patients/components/patient-form.tsx:459` |
| Selección | Fondo `bg-brand/5` en la fila activa | `Frontend/src/features/patients/components/patient-table.tsx:57` |
| Movimiento | `prefers-reduced-motion: reduce` neutraliza animaciones y transiciones globalmente | `Frontend/src/app/globals.css:38-45` |

## Contraste (calculado, no declarado)

Ratios calculados desde los valores de arriba, para detectar riesgo antes de diseñar. No son tokens ni están verificados en navegador.

| Par | Ratio | Veredicto AA texto normal (4.5:1) |
| --- | --- | --- |
| `--color-ink` sobre `--color-surface` | 16.6:1 | Pasa |
| `--color-ink-muted` sobre `--color-surface` | 6.2:1 | Pasa |
| `--color-ink-muted` sobre `--color-surface-muted` | 5.4:1 | Pasa |
| `--color-brand-ink` sobre `--color-brand` | 6.7:1 | Pasa |
| `--color-success` sobre `--color-surface` | 5.3:1 | Pasa |
| `--color-warning` sobre `--color-surface` | 5.9:1 | Pasa |
| `--color-danger` sobre `--color-surface` | 7.4:1 | Pasa |
| `--color-border` sobre `--color-surface` | 1.2:1 | Falla como borde de control (3:1) — decorativo hoy |

Lectura: la paleta es sólida para texto. El hueco real no es contraste, es que `--color-border` no alcanza 3:1 como límite de control si se usa para delimitar inputs, y que no existe ningún estado de foco visible.
