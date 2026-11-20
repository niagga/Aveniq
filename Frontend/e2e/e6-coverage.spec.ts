import { expect, test } from "@playwright/test";

import { ACCOUNTS, signInAs } from "./helpers";

// T51: the mutual catalog, and a patient covered by a mutual with its member number and
// billing currency. Serial, because the patient form offers the mutual the test above
// created.
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const insurerName = `E6 Mutual ${stamp}`;
const patientName = `E6 Cubierto ${stamp}`;
const memberNumber = `AF-${stamp}`;

test("the owner adds a mutual to the practice's catalog", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Mutuales" }).click();
  await expect(page.getByRole("heading", { name: "Mutuales" })).toBeVisible();

  await page.getByRole("button", { name: "Nueva mutual" }).click();
  await page.getByLabel("Nombre", { exact: true }).fill(insurerName);
  await page.getByLabel("CUIT (opcional)").fill("30-12345678-9");
  await page.getByRole("button", { name: "Crear mutual" }).click();

  await expect(page.getByRole("cell", { name: insurerName })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("row", { name: new RegExp(insurerName) })).toContainText("30123456789");
});

test("a patient covered by the mutual carries its member number and billing currency", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(patientName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByLabel("Cobertura").selectOption("insurer");
  await page.getByLabel("Mutual", { exact: true }).selectOption({ label: insurerName });
  await page.getByLabel("Número de afiliado").fill(memberNumber);
  await page.getByLabel("Moneda de facturación").selectOption("EUR");
  await page.getByRole("button", { name: "Crear paciente" }).click();

  // The roster shows 50 patients by name and the development database keeps growing, so the
  // new patient is found by its exact name, not by its place in the list.
  await page.getByLabel("Buscar", { exact: true }).fill(patientName);
  await page.getByRole("button", { name: "Buscar", exact: true }).click();

  const row = page.getByRole("row", { name: new RegExp(patientName) });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await expect(row).toContainText(insurerName);
  await expect(row).toContainText(memberNumber);
  await expect(row).toContainText("EUR");
});

test("the form refuses a patient without a billing currency, and never preselects one", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await expect(page.getByLabel("Moneda de facturación")).toHaveValue("");
  await page.getByLabel("Nombre y apellido").fill(`E6 Sin moneda ${stamp}`);
  await page.getByRole("button", { name: "Crear paciente" }).click();

  await expect(page.getByText("Elegí la moneda de facturación.")).toBeVisible();
});
