import { expect, test } from "@playwright/test";

import { ACCOUNTS, signInAs } from "./helpers";

// T53: the fee schedule. Serial, because the later tests edit and remove the prices the first
// ones create.
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const mutualName = `E7 Mutual ${stamp}`;
// A start day unique to this run, so a re-run never collides with a price from an earlier one.
const validFrom = new Date(Date.UTC(1990, 0, 1) + (stamp % 9000) * 86_400_000).toISOString().slice(0, 10);
const shownFrom = validFrom.split("-").reverse().join("/");

test("the owner adds a mutual to the catalog the fee schedule prices", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Mutuales" }).click();
  await expect(page.getByRole("heading", { name: "Mutuales" })).toBeVisible();

  await page.getByRole("button", { name: "Nueva mutual" }).click();
  await page.getByLabel("Nombre", { exact: true }).fill(mutualName);
  await page.getByRole("button", { name: "Crear mutual" }).click();

  await expect(page.getByRole("cell", { name: mutualName })).toBeVisible({ timeout: 10_000 });
});

test("the owner prices a particular session for a professional", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo valor" }).click();
  await page.getByLabel("Profesional", { exact: true }).selectOption({ label: ACCOUNTS.professional.name });
  await page.getByLabel("Cobertura", { exact: true }).selectOption("private");
  await page.getByLabel("Modalidad", { exact: true }).selectOption("presencial");
  await page.getByLabel("Moneda", { exact: true }).selectOption("ARS");
  await page.getByLabel("Vigente desde").fill(validFrom);
  await page.getByLabel("Valor de la sesión").fill("15000,00");
  await page.getByRole("button", { name: "Cargar valor" }).click();

  const row = page
    .getByRole("row")
    .filter({ hasText: ACCOUNTS.professional.name })
    .filter({ hasText: shownFrom })
    .filter({ hasText: "Particular" });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await expect(row).toContainText("Particular");
  await expect(row).toContainText("Presencial");
  await expect(row).toContainText("ARS 15.000,00");
});

test("a second value for the same professional, coverage, modality, currency and day is refused", async ({
  page,
}) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo valor" }).click();
  await page.getByLabel("Profesional", { exact: true }).selectOption({ label: ACCOUNTS.professional.name });
  await page.getByLabel("Cobertura", { exact: true }).selectOption("private");
  await page.getByLabel("Modalidad", { exact: true }).selectOption("presencial");
  await page.getByLabel("Moneda", { exact: true }).selectOption("ARS");
  await page.getByLabel("Vigente desde").fill(validFrom);
  await page.getByLabel("Valor de la sesión").fill("20000,00");
  await page.getByRole("button", { name: "Cargar valor" }).click();

  await expect(page.getByRole("alert")).toBeVisible({ timeout: 10_000 });
});

test("the owner edits the amounts of a price, and nothing else", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  const row = page
    .getByRole("row")
    .filter({ hasText: ACCOUNTS.professional.name })
    .filter({ hasText: shownFrom })
    .filter({ hasText: "Particular" });
  await row.getByRole("button", { name: "Editar importes" }).click();
  await expect(page.getByRole("heading", { name: "Editar importes" })).toBeVisible();
  await expect(page.getByLabel("Modalidad", { exact: true })).toHaveCount(0);

  await page.getByLabel("Valor de la sesión").fill("16000,00");
  await page.getByRole("button", { name: "Guardar importes" }).click();

  await expect(row).toContainText("ARS 16.000,00", { timeout: 10_000 });
});

test("the owner prices a mutual session: what the mutual pays and the copago", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo valor" }).click();
  await page.getByLabel("Profesional", { exact: true }).selectOption({ label: ACCOUNTS.professional.name });
  await page.getByLabel("Cobertura", { exact: true }).selectOption("insurer");
  await page.getByLabel("Mutual", { exact: true }).selectOption({ label: mutualName });
  await page.getByLabel("Modalidad", { exact: true }).selectOption("online");
  await page.getByLabel("Moneda", { exact: true }).selectOption("EUR");
  await page.getByLabel("Vigente desde").fill(validFrom);
  await page.getByLabel("Lo que paga la mutual").fill("8000,00");
  await page.getByLabel("Copago del paciente").fill("1500,00");
  await page.getByRole("button", { name: "Cargar valor" }).click();

  const row = page.getByRole("row").filter({ hasText: mutualName });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await expect(row).toContainText("Online");
  await expect(row).toContainText("EUR 8.000,00");
  await expect(row).toContainText("EUR 1.500,00");
  await expect(row).toContainText("EUR 9.500,00");
});

test("the accountant reads the fee schedule and cannot change it", async ({ page }) => {
  // The accountant lands on billing after signing in; the dashboard is where the link lives.
  await signInAs(page, ACCOUNTS.accountant);
  await page.goto("/");
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  const row = page
    .getByRole("row")
    .filter({ hasText: ACCOUNTS.professional.name })
    .filter({ hasText: shownFrom })
    .filter({ hasText: "Particular" });
  await expect(row).toContainText("ARS 16.000,00", { timeout: 10_000 });

  await expect(page.getByRole("button", { name: "Nuevo valor" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Editar importes" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Quitar" })).toHaveCount(0);
});

test("the owner removes the mutual price and the table no longer lists it", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Tarifario" }).click();
  await expect(page.getByRole("heading", { name: "Tarifario" })).toBeVisible();

  const row = page.getByRole("row").filter({ hasText: mutualName });
  await expect(row).toBeVisible({ timeout: 10_000 });

  page.once("dialog", (dialog) => void dialog.accept());
  await row.getByRole("button", { name: "Quitar" }).click();

  await expect(page.getByRole("row").filter({ hasText: mutualName })).toHaveCount(0, { timeout: 10_000 });
});
