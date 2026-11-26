import { expect, test, type Page } from "@playwright/test";

import { ACCOUNTS, bookFirstFreeHour, closeDialog, openBookingPopup, openIssuePopup, searchRoster, signInAs, submitIssuePopup } from "./helpers";

// T54: emission from a session. Serial: the professional books the session the accountant bills,
// and the accountant bills it once.
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const patientName = `E8 Paciente ${stamp}`;
const proposedAmount = "15000,00";

// The start day of the price, today: it is in force for a session booked for tomorrow, and it
// is later than every price the other runs create, so it is the one the emission proposes.
const today = new Date().toISOString().slice(0, 10);
const shownToday = today.split("-").reverse().join("/");

/** Opens the billing screen and waits for its heading. */
async function openBilling(page: Page) {
  await page.goto("/facturacion");
  await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
}

/**
 * Opens the issue popup and names the patient whose session is billed. The search asks the
 * API after a pause, so the answer to this exact text is awaited before the choice.
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

  // The chosen patient is shown, and the form offers to take it off again.
  await expect(dialog.getByRole("button", { name: "Quitar" })).toBeVisible();
}

test("the owner prices a particular session for the professional from today", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo valor" }).click();
  await page.getByLabel("Profesional", { exact: true }).selectOption({ label: ACCOUNTS.professional.name });
  await page.getByLabel("Cobertura", { exact: true }).selectOption("private");
  await page.getByLabel("Modalidad", { exact: true }).selectOption("presencial");
  await page.getByLabel("Moneda", { exact: true }).selectOption("ARS");
  await page.getByLabel("Vigente desde").fill(today);
  await page.getByLabel("Valor de la sesión").fill(proposedAmount);
  await page.getByRole("button", { name: "Cargar valor" }).click();

  // A second run on the same day finds the price already there, from the first run: the day is
  // the key, so the row is listed either way, with the same amount.
  const row = page
    .getByRole("row")
    .filter({ hasText: ACCOUNTS.professional.name })
    .filter({ hasText: shownToday })
    .filter({ hasText: "Particular" });
  await expect(row).toContainText("ARS 15.000,00", { timeout: 10_000 });
});

test("the professional registers a patient and books a session for tomorrow", async ({ page }) => {
  // Registration plus the booking walk over the coming days go past the 30 s default.
  test.setTimeout(120_000);

  await signInAs(page, ACCOUNTS.professional);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(patientName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByLabel("Moneda de facturación").selectOption("ARS");
  await page.getByRole("button", { name: "Crear paciente" }).click();
  // The roster shows one page of 50 in name order and the development database keeps
  // accumulating runs, so the new patient is found by the roster's search (T77, as e4/e5/e6).
  await searchRoster(page, patientName);
  await expect(page.getByRole("cell", { name: patientName })).toBeVisible({ timeout: 10_000 });

  await page.getByRole("link", { name: "Agenda" }).click();
  await expect(page.getByRole("heading", { name: "Agenda" })).toBeVisible();

  // The booking form opens in a popup since T68: the fields live inside "Nuevo turno". The
  // helper names the patient and the modality and walks the whole hours of the coming days
  // (T77), moving to the next day when one is full, instead of being pinned to tomorrow.
  const booking = await openBookingPopup(page);
  await bookFirstFreeHour(page, booking, { patientName, modality: "presencial" });
});

test("the accountant bills a session at the proposed amount, once", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openBilling(page);

  // The issue form is a popup since T76; every control below lives inside "Emitir un comprobante".
  const dialog = await openIssuePopup(page);
  await pickPatient(page, dialog, patientName);

  // The session of the patient is in the picker; the tariff proposes its amount.
  const sessions = dialog.getByLabel("Turno (opcional)");
  await expect(sessions).toBeVisible({ timeout: 10_000 });
  await sessions.selectOption({ index: 1 });

  await expect(dialog.getByLabel("Importe (ARS)")).toHaveValue(proposedAmount);
  await expect(dialog.getByText(/Propuesto por el tarifario/)).toBeVisible();
  const professional = dialog.getByLabel("Profesional", { exact: true });
  await expect(professional).toHaveValue(ACCOUNTS.professional.name);

  const detail = await submitIssuePopup(page, dialog);
  // The detail popup is the issuance confirmation (T76): the number in its title, the amount
  // and the buyer stamped by the server.
  await expect(detail.getByText("ARS 15.000,00")).toBeVisible();
  await expect(detail.getByText(patientName)).toBeVisible();
  await closeDialog(detail);

  // The same session is billed already: the picker lists it, disabled, with the reason.
  const again = await openIssuePopup(page);
  await pickPatient(page, again, patientName);
  await expect(again.getByRole("option", { name: /Ya facturado/ })).toBeDisabled({ timeout: 10_000 });
});

test("the voucher of the session shows the photo it was issued with", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openBilling(page);

  const row = page.getByRole("row").filter({ hasText: patientName });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await row.getByRole("link", { name: /^Ver el comprobante/ }).click();

  const detail = page.getByRole("dialog", { name: /Comprobante N° \d{6}/ });
  await expect(detail.getByText("ARS 15.000,00")).toBeVisible();
  await expect(detail.getByText("Particular")).toBeVisible();
  await expect(detail.getByText("Presencial")).toBeVisible();
  await expect(detail.getByText(ACCOUNTS.professional.name)).toBeVisible();
});
