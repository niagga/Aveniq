import { expect, test, type Browser, type Page } from "@playwright/test";

import { ACCOUNTS, signInAs, type Account } from "./helpers";

/**
 * Internal vouchers through the product screens (E4, T44): the accountant issues and voids,
 * the owner and the admin read, and the clinical roles are refused.
 *
 * Every negative check sits next to a positive one in the same view, so a page that has not
 * loaded yet cannot pass for a page that hides the control.
 */

test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const voidedConcept = `E4 Anulado ${stamp}`;
const issuedConcept = `E4 Vigente ${stamp}`;
const patientName = `E4 Paciente ${stamp}`;
const patientConcept = `E4 Sesion con paciente ${stamp}`;

/** Opens a fresh browser context signed in as the account, and returns how to close it. */
async function openAs(browser: Browser, account: Account): Promise<{ page: Page; close: () => Promise<void> }> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signInAs(page, account);
  return { page, close: () => context.close() };
}

async function openBilling(page: Page) {
  await page.goto("/facturacion");
  await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
}

/** Issues a loose voucher (no patient) from the form and waits for the server's answer. */
async function issueLoose(page: Page, concept: string, amount: string) {
  await openBilling(page);
  await page.getByLabel("Concepto").fill(concept);
  await page.getByLabel("Importe (ARS)").fill(amount);
  await page.getByRole("button", { name: "Emitir comprobante" }).click();
  await expect(page.getByText(/Emitimos el comprobante N° \d{6}/)).toBeVisible({ timeout: 10_000 });
}

function rowFor(page: Page, concept: string) {
  return page.getByRole("row").filter({ hasText: concept });
}

test("the accountant lands on billing, issues a voucher and voids it", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.accountant);
  try {
    // The sign-in lands the accountant on billing, not on the dashboard.
    await expect(page).toHaveURL(/\/facturacion$/);

    await issueLoose(page, voidedConcept, "1500");
    await issueLoose(page, issuedConcept, "800,50");

    // The voucher is in the history, and the detail shows what was issued.
    const row = rowFor(page, voidedConcept);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await row.getByRole("link", { name: /^Ver el comprobante/ }).click();

    const detail = page.getByRole("region", { name: /Detalle del comprobante/ });
    await expect(detail.getByText("ARS 1.500,00")).toBeVisible();
    await expect(detail.getByText("Cobro sin paciente")).toBeVisible();

    // Voiding asks for a reason and a second click.
    await detail.getByRole("button", { name: "Anular comprobante" }).click();
    await expect(detail.getByRole("button", { name: "Confirmar anulación" })).toBeVisible();
    await detail.getByLabel("Motivo de la anulación").fill("Error de carga E4");
    await detail.getByRole("button", { name: "Confirmar anulación" }).click();

    // "Anulado el …" is the void notice; the bare badge "Anulado" would match both.
    await expect(detail.getByText(/^Anulado el /)).toBeVisible({ timeout: 10_000 });
    await expect(detail.getByText("Motivo: Error de carga E4")).toBeVisible();
    await expect(rowFor(page, voidedConcept).getByText("Anulado", { exact: true })).toBeVisible();
  } finally {
    await close();
  }
});

test("the owner reads the same history and sees the voided voucher as voided", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.owner);
  try {
    await openBilling(page);
    await expect(page.getByRole("heading", { name: "Emitir un comprobante" })).toBeVisible();

    await expect(rowFor(page, voidedConcept)).toBeVisible({ timeout: 10_000 });
    await expect(rowFor(page, voidedConcept).getByText("Anulado", { exact: true })).toBeVisible();
    await expect(rowFor(page, issuedConcept).getByText("Emitido", { exact: true })).toBeVisible();
  } finally {
    await close();
  }
});

test("the admin reads the history and can neither issue nor void", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.admin);
  try {
    await openBilling(page);
    await expect(page.getByText("Solo lectura")).toBeVisible();
    await expect(page.getByRole("button", { name: "Emitir comprobante" })).toHaveCount(0);

    // Positive control: the history is there.
    const row = rowFor(page, issuedConcept);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await row.getByRole("link", { name: /^Ver el comprobante/ }).click();

    const detail = page.getByRole("region", { name: /Detalle del comprobante/ });
    await expect(detail.getByText("ARS 800,50")).toBeVisible();
    await expect(detail.getByRole("button", { name: "Anular comprobante" })).toHaveCount(0);
  } finally {
    await close();
  }
});

test("the accountant's voucher can name a patient, and the buyer comes from the patient record", async ({
  browser,
}) => {
  // The owner registers the patient; the accountant has no roster to register it in.
  const owner = await openAs(browser, ACCOUNTS.owner);
  try {
    await owner.page.goto("/patients");
    await expect(owner.page.getByRole("heading", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
    await owner.page.getByRole("button", { name: "Nuevo paciente" }).click();
    await owner.page.getByLabel("Nombre y apellido").fill(patientName);
    await owner.page.getByLabel("Fecha de nacimiento").fill("1988-03-04");
    await owner.page.getByLabel("Moneda de facturación").selectOption("ARS");
    await owner.page.getByRole("button", { name: "Crear paciente" }).click();

    // The roster shows 50 patients by name and the development database keeps growing, so the
    // new patient is found by its exact name, not by its place in the list.
    await owner.page.getByLabel("Buscar", { exact: true }).fill(patientName);
    await owner.page.getByRole("button", { name: "Buscar", exact: true }).click();
    await expect(owner.page.getByRole("cell", { name: patientName })).toBeVisible({ timeout: 10_000 });
  } finally {
    await owner.close();
  }

  const { page, close } = await openAs(browser, ACCOUNTS.accountant);
  try {
    await openBilling(page);

    // Search by the patient's full name: at least three characters, answered after a short pause.
    const answered = page.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/billing/subjects") &&
        new URL(response.url()).searchParams.get("search") === patientName,
    );
    await page.getByLabel("Paciente (opcional)").fill(patientName);
    await answered;
    await page.getByRole("button", { name: patientName }).click();

    // The chosen patient is shown, and the form offers to take it off again.
    await expect(page.getByRole("button", { name: "Quitar" })).toBeVisible();

    await page.getByLabel("Concepto").fill(patientConcept);
    await page.getByLabel("Importe (ARS)").fill("2500");
    await page.getByRole("button", { name: "Emitir comprobante" }).click();
    await expect(page.getByText(/Emitimos el comprobante N° \d{6}/)).toBeVisible({ timeout: 10_000 });

    // The buyer is the patient's name, stamped by the server; nothing was typed for it.
    await expect(rowFor(page, patientConcept).getByRole("cell", { name: patientName })).toBeVisible({
      timeout: 10_000,
    });
  } finally {
    await close();
  }
});

test("the accountant's dashboard shows billing only, and the clinical screens refuse it", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.accountant);
  try {
    await page.goto("/panel");
    // Positive control first: the dashboard has loaded and offers billing.
    await expect(page.getByRole("link", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("link", { name: "Pacientes" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Agenda" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Usuarios" })).toHaveCount(0);

    await page.goto("/patients");
    await expect(page.getByText("Tu rol no tiene acceso a esta pantalla.")).toBeVisible({ timeout: 15_000 });
    await page.goto("/agenda");
    await expect(page.getByText("Tu rol no tiene acceso a esta pantalla.")).toBeVisible({ timeout: 15_000 });
    await page.goto("/usuarios");
    await expect(page.getByText("Esta pantalla es para el titular y la administración")).toBeVisible({
      timeout: 15_000,
    });
  } finally {
    await close();
  }
});

test("a voucher is reachable by its URL and GET, even after a reload", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.accountant);
  try {
    await openBilling(page);
    const row = rowFor(page, issuedConcept);
    await expect(row).toBeVisible({ timeout: 10_000 });
    const gotOne = page.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        /\/api\/v1\/billing\/vouchers\/[0-9a-f-]{36}/i.test(response.url()),
    );
    await row.getByRole("link", { name: /^Ver el comprobante/ }).click();
    expect((await gotOne).ok()).toBeTruthy();

    await expect(page).toHaveURL(/\/facturacion\?voucher=/);
    const detail = page.getByRole("region", { name: /Detalle del comprobante/ });
    await expect(detail.getByText("ARS 800,50")).toBeVisible();

    const url = page.url();
    await page.reload();
    await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
    await expect(page).toHaveURL(url);
    await expect(page.getByRole("region", { name: /Detalle del comprobante/ }).getByText("ARS 800,50")).toBeVisible({
      timeout: 10_000,
    });

    await page.goto("/facturacion?voucher=00000000-0000-4000-8000-000000000099");
    await expect(page.getByText("Ese comprobante no está en este consultorio")).toBeVisible({ timeout: 10_000 });
  } finally {
    await close();
  }
});

for (const [label, account] of [
  ["the professional", ACCOUNTS.professional],
  ["the assistant", ACCOUNTS.assistant],
] as const) {
  test(`${label} has no billing link and is refused the billing screen`, async ({ browser }) => {
    const { page, close } = await openAs(browser, account);
    try {
      await page.goto("/panel");
      // Positive control: the dashboard has loaded and shows the patients link.
      await expect(page.getByRole("link", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
      await expect(page.getByRole("link", { name: "Facturación" })).toHaveCount(0);

      await page.goto("/facturacion");
      await expect(page.getByText("Tu rol no tiene acceso a esta pantalla.")).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText(issuedConcept)).toHaveCount(0);
    } finally {
      await close();
    }
  });
}
