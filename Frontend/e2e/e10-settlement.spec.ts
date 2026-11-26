import { expect, test, type Page } from "@playwright/test";

import { ACCOUNTS, openBookingPopup, openIssuePopup, signInAs, submitIssuePopup } from "./helpers";

// T56: the whole of E6 in one story. The owner adds a mutual and prices the professional for it, the
// professional registers a covered patient and attends the session, the accountant bills it at the
// proposed amount and settles the month to the mutual. Serial: every step builds on the one before.
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const insurerName = `E10 Mutual ${stamp}`;
const patientName = `E10 Afiliado ${stamp}`;
const memberNumber = `AF-${stamp}`;

// The mutual pays 12.000,00 and the patient adds a copago of 3.000,00: the voucher bills the whole
// session (decision (d) of L66) and the settlement bills the mutual its share only (L67).
const insurerShare = "12000,00";
const copago = "3000,00";
const proposedTotal = "15000,00";

// The price rules from today: it is in force for any session booked from tomorrow on.
const today = new Date().toISOString().slice(0, 10);

// Month of the session as the settlement reads it, in the practice's timezone. Set when the
// session is booked, because tomorrow can already be the next month.
let sessionMonth = "";
const practiceZone = "America/Argentina/Buenos_Aires";

/** The month (YYYY-MM) of an instant in the practice's timezone. */
function monthInPractice(instant: Date): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: practiceZone,
    year: "numeric",
    month: "2-digit",
  }).formatToParts(instant);
  const part = (type: string) => parts.find((item) => item.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}`;
}

/** Opens the settlements screen and waits for its heading. */
async function openSettlements(page: Page) {
  await page.getByRole("link", { name: "Liquidaciones" }).click();
  await expect(page.getByRole("heading", { name: "Liquidaciones" })).toBeVisible({ timeout: 15_000 });
}

/**
 * Opens the issue popup and names the patient whose session is billed, waiting for the
 * search answer to this exact text before choosing.
 */
async function pickPatient(page: Page, dialog: Awaited<ReturnType<typeof openIssuePopup>>, patientName: string) {
  const answered = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/billing/subjects") &&
      new URL(response.url()).searchParams.get("search") === patientName,
  );
  await dialog.getByLabel("Paciente (opcional)").fill(patientName);
  await answered;
  await dialog.getByRole("list", { name: "Pacientes encontrados" }).getByRole("button", { name: patientName }).click();
}

/** Chooses the mutual and the month and asks for the sessions to settle. */
async function previewSessions(page: Page) {
  const issue = page.getByRole("region", { name: "Nueva liquidación" });
  await issue.getByLabel("Mutual", { exact: true }).selectOption({ label: insurerName });
  await issue.getByLabel("Mes", { exact: true }).fill(sessionMonth);
  await issue.getByRole("button", { name: "Ver sesiones" }).click();
  return issue;
}

test("the owner adds a mutual with its CUIT and prices the professional for it", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);

  await page.getByRole("link", { name: "Mutuales" }).click();
  await expect(page.getByRole("heading", { name: "Mutuales" })).toBeVisible();
  await page.getByRole("button", { name: "Nueva mutual" }).click();
  await page.getByLabel("Nombre", { exact: true }).fill(insurerName);
  await page.getByLabel("CUIT (opcional)").fill("30-12345678-9");
  await page.getByRole("button", { name: "Crear mutual" }).click();
  await expect(page.getByRole("cell", { name: insurerName })).toBeVisible({ timeout: 10_000 });

  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();
  await page.getByRole("button", { name: "Nuevo valor" }).click();
  await page.getByLabel("Profesional", { exact: true }).selectOption({ label: ACCOUNTS.professional.name });
  await page.getByLabel("Cobertura", { exact: true }).selectOption("insurer");
  await page.getByLabel("Mutual", { exact: true }).selectOption({ label: insurerName });
  await page.getByLabel("Modalidad", { exact: true }).selectOption("presencial");
  await page.getByLabel("Moneda", { exact: true }).selectOption("ARS");
  await page.getByLabel("Vigente desde").fill(today);
  await page.getByLabel("Lo que paga la mutual").fill(insurerShare);
  await page.getByLabel("Copago del paciente").fill(copago);
  await page.getByRole("button", { name: "Cargar valor" }).click();

  // The two parts and their sum are listed: the mutual's share, the copago, and the whole session.
  const row = page.getByRole("row").filter({ hasText: insurerName });
  await expect(row).toContainText("ARS 12.000,00", { timeout: 10_000 });
  await expect(row).toContainText("ARS 3.000,00");
  await expect(row).toContainText("ARS 15.000,00");
});

test("the professional registers a patient of the mutual, books the session and attends it", async ({
  page,
}) => {
  // The booking loop walks the hours of tomorrow past the appointments earlier runs left behind.
  test.setTimeout(120_000);

  await signInAs(page, ACCOUNTS.professional);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(patientName);
  await page.getByLabel("Fecha de nacimiento").fill("1988-03-21");
  await page.getByLabel("Cobertura").selectOption("insurer");
  await page.getByLabel("Mutual", { exact: true }).selectOption({ label: insurerName });
  await page.getByLabel("Número de afiliado").fill(memberNumber);
  await page.getByLabel("Moneda de facturación").selectOption("ARS");
  await page.getByRole("button", { name: "Crear paciente" }).click();
  await expect(page.getByRole("cell", { name: patientName })).toBeVisible({ timeout: 10_000 });

  await page.getByRole("link", { name: "Agenda" }).click();
  await expect(page.getByRole("heading", { name: "Agenda" })).toBeVisible();

  // The booking form opens in a popup since T68: the fields live inside "Nuevo turno".
  const booking = await openBookingPopup(page);
  await booking.getByLabel("Paciente").selectOption({ label: patientName });
  await booking.getByLabel("Modalidad").selectOption("presencial");

  // Whole hours of tomorrow, one until a slot is free. Sundays book later today, as in E8.
  const day = new Date();
  if (day.getDay() === 0) {
    day.setHours(day.getHours() + 1, 0, 0, 0);
  } else {
    day.setDate(day.getDate() + 1);
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  const booked = page.getByRole("button", { name: new RegExp(patientName) });
  let isBooked = false;
  for (let hour = 8; hour < 20 && !isBooked; hour++) {
    const start = new Date(day);
    start.setHours(hour, 0, 0, 0);
    if (start < new Date()) continue;
    const local = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())}T${pad(start.getHours())}:00`;
    await booking.getByLabel("Inicio").fill(local);
    await booking.getByRole("button", { name: "Reservar" }).click();
    isBooked = await booked.waitFor({ state: "visible", timeout: 5_000 }).then(
      () => true,
      () => false,
    );
    if (isBooked) sessionMonth = monthInPractice(start);
  }
  expect(isBooked, "no free hour left on the next day").toBe(true);
  expect(sessionMonth).toMatch(/^\d{4}-\d{2}$/);

  // Only an attended session is settled.
  await booked.click();
  await page.getByRole("button", { name: "Marcar atendido" }).click();
  await expect(page.getByRole("button", { name: "Marcar atendido" })).toHaveCount(0, { timeout: 10_000 });
});

test("the accountant bills the session at the mutual's share plus the copago", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await page.goto("/facturacion");
  await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });

  // The issue form is a popup since T76; every control below lives inside "Emitir un comprobante".
  const dialog = await openIssuePopup(page);
  await pickPatient(page, dialog, patientName);

  const sessions = dialog.getByLabel("Turno (opcional)");
  await expect(sessions).toBeVisible({ timeout: 10_000 });
  await sessions.selectOption({ index: 1 });

  await expect(dialog.getByLabel("Importe (ARS)")).toHaveValue(proposedTotal);
  await expect(dialog.getByText(/Propuesto por el tarifario/)).toBeVisible();
  const detail = await submitIssuePopup(page, dialog);
  // The detail popup is the issuance confirmation (T76): the number in its title, the amount
  // and the buyer stamped by the server.
  await expect(detail.getByText("ARS 15.000,00")).toBeVisible();
  await expect(detail.getByText(patientName)).toBeVisible();
});

test("the accountant settles the month to the mutual: its share is the total, the copago only shows", async ({
  page,
}) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openSettlements(page);

  const issue = await previewSessions(page);
  const row = issue.getByRole("row", { name: new RegExp(patientName) });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await expect(row).toContainText(memberNumber);
  await expect(row).toContainText(ACCOUNTS.professional.name);
  await expect(row).toContainText("ARS 12.000,00");
  await expect(row).toContainText("ARS 3.000,00");

  // The total is what the mutual owes; the copago is shown on the line and never summed into it.
  const total = issue.locator("p").filter({ hasText: "Total a liquidar" });
  await expect(total).toContainText("ARS 12.000,00");
  await expect(total).not.toContainText("15.000,00");

  await issue.getByRole("button", { name: "Emitir liquidación" }).click();
  await expect(issue.getByText(/Liquidación emitida/)).toBeVisible({ timeout: 10_000 });

  const history = page.getByRole("region", { name: "Historial de liquidaciones" });
  const entry = history.getByRole("row", { name: new RegExp(insurerName) });
  await expect(entry).toContainText("Emitida", { timeout: 10_000 });
  await expect(entry).toContainText("ARS 12.000,00");
  await entry.getByRole("button", { name: /^Ver la liquidación de/ }).click();

  const detail = page.getByRole("region", { name: /Detalle de la liquidación/ });
  const line = detail.getByRole("row", { name: new RegExp(patientName) });
  await expect(line).toContainText("Liquidada");
  await expect(line).toContainText("ARS 12.000,00");
  await expect(line).toContainText("ARS 3.000,00");
});

test("a session is never settled twice", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openSettlements(page);

  const issue = await previewSessions(page);
  await expect(issue.getByText("No hay sesiones para liquidar")).toBeVisible({ timeout: 10_000 });
  await expect(issue.getByRole("button", { name: "Emitir liquidación" })).toBeDisabled();
});

test("the administrator reads the settlement and can neither issue nor void it", async ({ page }) => {
  await signInAs(page, ACCOUNTS.admin);
  await openSettlements(page);

  await expect(page.getByText("Solo lectura")).toBeVisible();
  await expect(page.getByRole("button", { name: "Ver sesiones" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Emitir liquidación" })).toHaveCount(0);

  const history = page.getByRole("region", { name: "Historial de liquidaciones" });
  const entry = history.getByRole("row", { name: new RegExp(insurerName) });
  await expect(entry).toBeVisible({ timeout: 10_000 });
  await entry.getByRole("button", { name: /^Ver la liquidación de/ }).click();

  const detail = page.getByRole("region", { name: /Detalle de la liquidación/ });
  await expect(detail.getByRole("row", { name: new RegExp(patientName) })).toBeVisible();
  await expect(page.getByRole("button", { name: "Anular liquidación" })).toHaveCount(0);
});

for (const account of [ACCOUNTS.assistant, ACCOUNTS.professional]) {
  test(`${account.name} is not offered the settlements`, async ({ page }) => {
    await signInAs(page, account);
    await expect(page.getByRole("link", { name: "Liquidaciones" })).toHaveCount(0);
  });
}

test("voiding the settlement frees its sessions to be settled again", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openSettlements(page);

  const history = page.getByRole("region", { name: "Historial de liquidaciones" });
  const entry = history.getByRole("row", { name: new RegExp(insurerName) });
  await expect(entry).toBeVisible({ timeout: 10_000 });
  await entry.getByRole("button", { name: /^Ver la liquidación de/ }).click();

  const detail = page.getByRole("region", { name: /Detalle de la liquidación/ });
  await detail.getByRole("button", { name: "Anular liquidación" }).click();
  await detail.getByLabel("Motivo de la anulación").fill("E2E: liquidación de prueba");
  await detail.getByRole("button", { name: "Confirmar anulación" }).click();

  await expect(detail).toContainText("Anulada", { timeout: 10_000 });
  await expect(detail).toContainText("quedaron libres");
  await expect(detail.getByRole("row", { name: new RegExp(patientName) })).toContainText("Liberada");

  // The session is settlable again, and the voided settlement stays in the history.
  const issue = await previewSessions(page);
  await expect(issue.getByRole("row", { name: new RegExp(patientName) })).toBeVisible({ timeout: 10_000 });
  await expect(history.getByRole("row", { name: new RegExp(insurerName) })).toContainText("Anulada");
});
