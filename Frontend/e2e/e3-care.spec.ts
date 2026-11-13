import { expect, test, type BrowserContext, type Page } from "@playwright/test";

import { ACCOUNTS, signInAs } from "./helpers";

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

async function openRoster(page: Page) {
  await page.goto("/patients");
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
}

/** Searches the roster and waits for the answer to that exact search, so a negative check runs on a loaded list. */
async function searchRoster(page: Page, text: string) {
  await openRoster(page);
  const answered = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/patients?") &&
      new URL(response.url()).searchParams.get("search") === text,
  );
  await page.getByLabel("Buscar").fill(text);
  await page.getByRole("button", { name: "Buscar", exact: true }).click();
  await answered;
}

async function registerPatient(page: Page, fullName: string) {
  await openRoster(page);
  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(fullName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByRole("button", { name: "Crear paciente" }).click();
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

  // It cannot book the patient: the agenda offers only its own patients.
  await other.goto("/agenda");
  await expect(other.getByRole("heading", { name: "Agenda" })).toBeVisible({ timeout: 15_000 });
  const patientOptions = other.getByLabel("Paciente").locator("option");
  await expect(patientOptions.filter({ hasText: ownName })).toHaveCount(1, { timeout: 10_000 });
  await expect(patientOptions.filter({ hasText: treatedName })).toHaveCount(0);

  // Revoking ends the read at once.
  await searchRoster(professional, treatedName);
  await openPatient(professional, treatedName);
  await professional
    .getByRole("button", { name: `Revocar acceso de ${ACCOUNTS.otherProfessional.name}` })
    .click();
  await expect(professional.getByText("Ningún otro profesional tiene acceso.")).toBeVisible({
    timeout: 10_000,
  });

  await searchRoster(other, ownName);
  await expect(other.getByRole("cell", { name: ownName })).toBeVisible({ timeout: 10_000 });
  await searchRoster(other, treatedName);
  await expect(other.getByRole("cell", { name: treatedName })).toHaveCount(0);
});
