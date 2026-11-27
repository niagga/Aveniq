# v1 — Facturación electrónica (ARCA)

Borrador de diseño, **no** un compromiso de implementación. Existe para que las decisiones
fiscales se tomen antes de escribir código, y para que quien las tome vea qué piezas ya existen
y cuáles hay que construir.

El MVP emite **comprobantes internos** desde E4 (migración `0014`): numeración correlativa sin
huecos por consultorio, snapshot del comprador al emitir, anulación sin reutilizar número. La
facturación electrónica quedó fuera desde S7 (`Facturación electrónica decidida más adelante; el
MVP solo registra comprobantes internos`). Esto es lo que viene después.

## Specs

- **S1 — El primer comprobante electrónico es Factura C.** Decisión del dueño: `Factura C
  (monotributo) primero`. En términos fiscales: sin discriminación de IVA, emisor monotributo, y
  el receptor puede ser consumidor final o estar identificado. **A confirmar con quien factura**:
  si el emisor es responsable inscripto en algún consultorio, C no aplica y hace falta A/B.
- **S2 — Los comprobantes internos ya emitidos quedan internos para siempre.** Decisión del
  dueño: `Quedan internos para siempre (recomendado)`. Conservan número y snapshot; no se
  re-emiten ni se migran. Sólo lo que se emita desde v1 sale electrónico.
- **S3 — El emisor necesita datos fiscales que hoy no existen.** `tenants` no tiene CUIT, punto de
  venta, condición frente al IVA ni inicio de actividades. Sin eso no hay comprobante: son datos
  del consultorio, no del paciente ni del cliente, y se editan con el mismo permiso que el resto
  de la configuración del consultorio.
- **S4 — Cada consultorio es un emisor distinto.** CUIT propio, certificado propio, punto de venta
  propio. El certificado es un secreto: no vive en el repositorio, no se sirve por HTTP y no se
  escribe en logs.
- **S5 — Todo intento contra ARCA queda registrado.** Pedido y respuesta, con su resultado, para
  poder responder qué se mandó y qué contestó el organismo. El comprobante guarda el CAE y su
  vencimiento.
- **S6 — Contingencia: decidir antes de codificar.** Si ARCA no responde, ¿se bloquea la emisión o
  se emite en contingencia y se regulariza después? Cambia el modelo de datos, no sólo el flujo.

## Lo que el código ya tiene

| Pieza | Estado |
| --- | --- |
| Comprobante con snapshot del comprador | `vouchers.buyer_*`, inmutable por trigger desde `0014`/`0020` |
| Numeración correlativa por consultorio | `voucher_counters`, sin huecos |
| Comprador con datos fiscales | `clients`: `tax_id`, `tax_id_kind`, `fiscal_condition`, `fiscal_address` (T71/T72) |
| Moneda de facturación | `clients.billing_currency` (ARS/EUR/USD) |
| Tarifario y liquidación | `tariffs`, `settlements` (E6) |
| Auditoría | `audit_log` append-only (migración `0008`) |
| Aislamiento por consultorio | RLS forzado en todas las tablas de negocio |

## Lo que falta

| Pieza | Nota |
| --- | --- |
| Datos fiscales del emisor | S3. Migración nueva sobre `tenants` |
| Certificado y clave privada por consultorio | S4. Cifrado en reposo, con rotación y con un camino de homologación |
| Cliente ARCA | WSAA (token y sign) y el servicio de facturación (CAE). **Los detalles del protocolo se verifican contra la documentación oficial antes de escribir el cliente**: acá no se transcriben de memoria |
| Tipo de comprobante, punto de venta y CAE en el comprobante | Hoy el comprobante no distingue interno de fiscal |
| Moneda | ARCA factura en pesos; el producto permite EUR/USD. **Decisión abierta**: ¿se restringe la emisión electrónica a ARS o se convierte con cotización? |
| Contingencia | S6 |

## Decisiones abiertas (para el dueño y, sobre todo, para quien factura)

1. **Tipo de comprobante por consultorio**: ¿todos los consultorios son monotributo, o alguno es
   responsable inscripto y necesita A/B?
2. **Moneda**: ¿qué pasa con un cliente en EUR o USD al emitir electrónicamente?
3. **Contingencia**: bloquear la emisión o emitir y regularizar.
4. **Anulaciones**: el producto anula comprobantes internos; fiscalmente eso es una nota de
   crédito. ¿Se emite NC desde el principio o la anulación electrónica queda para después?
5. **Certificados**: quién los sube, dónde se generan y si se empieza por homologación (debería).
6. **Numeración**: el punto de venta de ARCA tiene su propia numeración; la interna actual no
   tiene por qué coincidir. ¿Se conserva la interna como referencia o se adopta la del organismo?

## Tasks

Skeleton, ninguna empezada. El orden respeta las dependencias: sin datos del emisor no hay
certificado, y sin certificado no hay cliente.

| ID | Spec | Tarea | Depende de |
| --- | --- | --- | --- |
| T1 | S3 | Datos fiscales del emisor: migración sobre `tenants`, dominio, HTTP y pantalla de configuración del consultorio | — |
| T2 | S4 | Certificado y clave por consultorio: almacenamiento cifrado, rotación y prohibición explícita de servirlo | T1 |
| T3 | S4 | Cliente ARCA contra homologación: WSAA y el servicio de facturación, con el protocolo verificado contra la documentación oficial | T2 |
| T4 | S1, S5 | Dominio: tipo de comprobante, punto de venta, CAE y vencimiento; el comprobante distingue interno de fiscal | T3 |
| T5 | S1, S2 | Emisión: interruptor por consultorio entre interno y electrónico; los internos existentes no se tocan | T4 |
| T6 | S5 | Registro de intentos: pedido, respuesta y resultado, en la misma transacción que el comprobante | T4 |
| T7 | S6 | Contingencia, según la decisión que se tome | T5 |
| T8 | S1 | Notas de crédito para las anulaciones, si se decide incluirlas | T5 |
| T9 | S5 | e2e contra homologación, con credenciales fuera del repositorio | T5 |

## Log

- **L1** — Pedido del dueño: `Bom dia, continua`, tras cerrar la observabilidad y con el board
  indicando este documento como siguiente paso.
- **L2** — Decisiones ya tomadas en la sesión anterior (L103 del board principal): `Factura C
  (monotributo) primero` y `Quedan internos para siempre (recomendado)` para los comprobantes
  internos ya emitidos.
- **L3** — Estado del código verificado al escribir esto: `tenants` no tiene ninguna columna
  fiscal, `vouchers` no distingue interno de fiscal ni guarda CAE, y no hay una sola línea de
  código de ARCA en el repositorio. Lo que sí está: snapshot del comprador, numeración sin
  huecos, datos fiscales del cliente y auditoría append-only.
