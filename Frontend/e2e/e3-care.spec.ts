import { expect, test, type BrowserContext, type Page } from "@playwright/test";

import { ACCOUNTS, openBookingPopup, searchRoster as searchLoadedRoster, signInAs } from "./helpers";

/**
 * Instance-level isolation (T38) through the product screens, with one browser context
 * per account so each one holds its own session.
 *
 * Every negative check is paired with a positive one in the same view: a list that has
 * not loaded yet would otherwise look like a list that hides the patient.
 */

test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const treatedName = `E3 Tratado ${stamp}`;
const ownName = `E3 Propio ${stamp}`;
// The shared stamp is a substring of both names, unlike "E3 <stamp>", which the middle word breaks.
const fragment = `${stamp}`;

let professionalContext: BrowserContext;
let otherContext: BrowserContext;
let assistantContext: BrowserContext;
let professional: Page;
let other: Page;
let assistant: Page;

test.beforeAll(async ({ browser }) => {
  professionalContext = await browser.newContext();
  otherContext = await browser.newContext();
  assistantContext = await browser.newContext();

  professional = await professionalContext.newPage();
  other = await otherContext.newPage();
  assistant = await assistantContext.newPage();

  await signInAs(professional, ACCOUNTS.professional);
  await signInAs(other, ACCOUNTS.otherProfessional);
  await signInAs(assistant, ACCOUNTS.assistant);
});

test.afterAll(async () => {
  await Promise.all([professionalContext?.close(), otherContext?.close(), assistantContext?.close()]);
});

/** Opens the roster, so a negative check runs on an already loaded list. */
async function openRoster(page: Page) {
  await page.goto("/patients");
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
}

/** Navigates to the roster and searches it, waiting for the answer to that exact search. */
async function searchRoster(page: Page, text: string) {
  await openRoster(page);
  await searchLoadedRoster(page, text);
}
async function registerPatient(page: Page, fullName: string) {
  await openRoster(page);
  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(fullName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByLabel("Moneda de facturación").selectOption("ARS");
  await page.getByRole("button", { name: "Crear paciente" }).click();
  // The roster shows one page of 50 in name order and the development database keeps
  // accumulating runs, so the new patient is found by the roster's search (as e4/e5/e6).
  await searchLoadedRoster(page, fullName);
  await expect(page.getByRole("cell", { name: fullName })).toBeVisible({ timeout: 10_000 });
}

async function openPatient(page: Page, fullName: string) {
  await page.getByRole("cell", { name: fullName }).click();
  await expect(page.getByRole("heading", { name: fullName })).toBeVisible();
}

test("a professional sees only the patients it treats", async () => {
  await registerPatient(professional, treatedName);
  await registerPatient(other, ownName);

  await searchRoster(other, fragment);
  await expect(other.getByRole("cell", { name: ownName })).toBeVisible({ timeout: 10_000 });
  await expect(other.getByRole("cell", { name: treatedName })).toHaveCount(0);

  await searchRoster(professional, fragment);
  await expect(professional.getByRole("cell", { name: treatedName })).toBeVisible({ timeout: 10_000 });
  await expect(professional.getByRole("cell", { name: ownName })).toHaveCount(0);
});

test("the assistant reads the roster but never the clinical record", async () => {
  await searchRoster(assistant, treatedName);
  await expect(assistant.getByRole("cell", { name: treatedName })).toBeVisible({ timeout: 10_000 });
  await openPatient(assistant, treatedName);

  await expect(assistant.getByRole("tab", { name: "Datos" })).toBeVisible();
  await expect(assistant.getByRole("tab", { name: "Historia clínica" })).toHaveCount(0);
});

test("a granted professional reads the record, cannot write or book it, and loses it on revoke", async () => {
  await searchRoster(professional, treatedName);
  await openPatient(professional, treatedName);

  await professional.getByLabel("Dar acceso a").selectOption({ label: ACCOUNTS.otherProfessional.name });
  await professional.getByRole("button", { name: "Dar acceso" }).click();
  await expect(professional.getByRole("button", { name: `Revocar acceso de ${ACCOUNTS.otherProfessional.name}` })).toBeVisible({
    timeout: 10_000,
  });

  // The grantee reads the file and the record, and gets no write control.
  await searchRoster(other, treatedName);
  await expect(other.getByRole("cell", { name: treatedName })).toBeVisible({ timeout: 10_000 });
  await openPatient(other, treatedName);
  await other.getByRole("tab", { name: "Historia clínica" }).click();
  await expect(other.getByText("Tenés acceso de lectura.")).toBeVisible({ timeout: 10_000 });
  await expect(other.getByRole("button", { name: "Guardar ficha" })).toHaveCount(0);
  await expect(other.getByRole("button", { name: "Registrar evolución" })).toHaveCount(0);

  // It cannot book the patient: the agenda offers only its own patients. The select lives
  // in the booking popup since T68, and the search answers with patients the practice
  // has (not only the ones the role books), so the check runs on the searched popup (T80):
  // the rule the screen applies to the search results must be the same one it applies to
  // the page it loads.
  await other.goto("/agenda");
  await expect(other.getByRole("heading", { name: "Agenda" })).toBeVisible({ timeout: 15_000 });
  const booking = await openBookingPopup(other);
  const searchBox = booking.getByLabel("Buscar paciente");
  const answered = other.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/patients?") &&
      response.status() === 200 &&
      new URL(response.url()).searchParams.get("search") === fragment,
  );
  await searchBox.fill(fragment);
  await answered;
  const patientOptions = booking.getByRole("combobox", { name: "Paciente" }).locator("option");
  await expect(patientOptions.filter({ hasText: ownName })).toHaveCount(1, { timeout: 10_000 });
  await expect(patientOptions.filter({ hasText: treatedName })).toHaveCount(0);

  // Revoking ends the read at once.
  await searchRoster(professional, treatedName);
  await openPatient(professional, treatedName);
  await professional
    .getByRole("button", { name: `Revocar acceso de ${ACCOUNTS.otherProfessional.name}` })
    .click();

  // Clinical access is the most sensitive thing on the screen, so the revoke asks first.
  const revokeDialog = professional.getByRole("dialog");
  await expect(revokeDialog).toBeVisible();
  await revokeDialog.getByRole("button", { name: "Revocar acceso", exact: true }).click();
  await expect(professional.getByText("Ningún otro profesional tiene acceso.")).toBeVisible({
    timeout: 10_000,
  });

  await searchRoster(other, ownName);
  await expect(other.getByRole("cell", { name: ownName })).toBeVisible({ timeout: 10_000 });
  await searchRoster(other, treatedName);
  await expect(other.getByRole("cell", { name: treatedName })).toHaveCount(0);
});
