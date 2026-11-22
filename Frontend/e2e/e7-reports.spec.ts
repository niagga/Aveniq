import { readFile } from "node:fs/promises";

import { expect, test, type Page } from "@playwright/test";

import { ACCOUNTS, signInAs } from "./helpers";

// T60: the reports page, the saved reports the practice shares, and the CSV. Serial, because the
// later tests open and remove what the first ones save.
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const ownerReport = `E7 Informe del titular ${stamp}`;
const accountantReport = `E7 Informe del contador ${stamp}`;

async function openReports(page: Page) {
  await page.getByRole("link", { name: "Informes", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Informes", level: 1 })).toBeVisible();
  await expect(page.getByRole("region", { name: "Resultado del informe" })).toBeVisible({
    timeout: 15_000,
  });
}

function cut(page: Page, label: string) {
  return page.getByRole("group", { name: "Cortar por" }).getByRole("button", { name: label });
}

function savedItem(page: Page, name: string) {
  return page.getByRole("listitem").filter({ hasText: name });
}

async function save(page: Page, name: string) {
  await page.getByLabel("Nombre", { exact: true }).fill(name);
  await page.getByRole("button", { name: "Guardar informe" }).click();
}

test("the owner reads the report and downloads it as a CSV a spreadsheet opens", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await openReports(page);

  await cut(page, "Modalidad").click();
  await expect(cut(page, "Modalidad")).toHaveAttribute("aria-pressed", "true");
  const results = page.getByRole("region", { name: "Resultado del informe" });
  await expect(results).toContainText("Todo el consultorio.");
  await expect(results.getByRole("row", { name: /Total/ })).toBeVisible({ timeout: 10_000 });

  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Exportar CSV" }).click(),
  ]);
  expect(download.suggestedFilename()).toMatch(/^informe-por-modalidad-\d{4}-\d{2}-\d{2}_\d{4}-\d{2}-\d{2}\.csv$/);

  const path = await download.path();
  const text = await readFile(path, "utf8");
  // The byte-order mark is what makes Excel read the accents; the header uses ";" for es-AR.
  expect(text.startsWith("\uFEFF")).toBe(true);
  const lines = text.slice(1).split("\r\n");
  expect(lines[0]).toBe(
    "Grupo;Moneda;Facturado;Comprobantes;Atendido sin facturar;Sesiones valuadas;Sesiones sin tarifa",
  );
  expect(lines.some((line) => line.startsWith("Total;"))).toBe(true);
});

test("a custom period that cannot be read says so and asks for nothing", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await openReports(page);

  await page.getByRole("group", { name: "Período" }).getByRole("button", { name: "Personalizado" }).click();
  await page.getByLabel("Desde").fill("2026-11-30");
  await page.getByLabel("Hasta").fill("2026-11-01");

  await expect(page.getByText("El primer día no puede ser posterior al último.")).toBeVisible();
  await expect(page.getByRole("region", { name: "Resultado del informe" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Guardar informe" })).toBeDisabled();
});

test("the owner saves a report and the practice shares it", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await openReports(page);

  await cut(page, "Profesional").click();
  await page.getByLabel("Modalidad", { exact: true }).selectOption("presencial");
  await save(page, ownerReport);

  const item = savedItem(page, ownerReport);
  await expect(item).toBeVisible({ timeout: 10_000 });
  await expect(item).toContainText("por profesional");
  await expect(item).toContainText("Presencial");
  await expect(item).toContainText(`Guardado por ${ACCOUNTS.owner.name}`);

  // The same name again is refused, with the reason next to the field.
  await page.getByRole("button", { name: "Guardar como nuevo" }).click();
  await expect(page.getByText("A saved report with that name already exists", { exact: false })).toBeVisible({
    timeout: 10_000,
  });
});

test("the accountant opens the owner's report, cannot remove it, and saves their own", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);
  await openReports(page);

  const item = savedItem(page, ownerReport);
  await expect(item).toBeVisible({ timeout: 10_000 });
  await expect(item.getByRole("button", { name: /Quitar/ })).toHaveCount(0);

  await item.getByRole("button", { name: ownerReport }).click();
  await expect(cut(page, "Profesional")).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByLabel("Modalidad", { exact: true })).toHaveValue("presencial");
  await expect(page.getByRole("heading", { name: "Informe abierto" })).toBeVisible();

  // Changing it is not offered to someone who did not write it: only a copy.
  await expect(page.getByRole("button", { name: "Actualizar informe" })).toHaveCount(0);
  await page.getByRole("button", { name: "Soltar" }).click();
  await cut(page, "Mes").click();
  await page.getByLabel("Modalidad", { exact: true }).selectOption("");
  await save(page, accountantReport);

  const own = savedItem(page, accountantReport);
  await expect(own).toBeVisible({ timeout: 10_000 });
  await expect(own.getByRole("button", { name: /Quitar/ })).toBeVisible();
});

test("the professional opens a shared report and reads only their own figures", async ({ page }) => {
  await signInAs(page, ACCOUNTS.professional);
  await openReports(page);

  // Nobody else's professional is offered to someone who only reads their own.
  await expect(page.getByLabel("Profesional", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Resultado del informe" })).toContainText(
    "Solo tus turnos y comprobantes.",
  );

  const item = savedItem(page, ownerReport);
  await expect(item).toBeVisible({ timeout: 10_000 });
  await expect(item.getByRole("button", { name: /Quitar/ })).toHaveCount(0);
  await item.getByRole("button", { name: ownerReport }).click();
  await expect(page.getByRole("region", { name: "Resultado del informe" })).toContainText(
    "Solo tus turnos y comprobantes.",
    { timeout: 10_000 },
  );
});

test("the assistant has no reports", async ({ page }) => {
  await signInAs(page, ACCOUNTS.assistant);
  await expect(page.getByRole("link", { name: "Informes", exact: true })).toHaveCount(0);
  await page.goto("/informes");
  await expect(page.getByText("Tu rol no tiene acceso a esta pantalla.")).toBeVisible();
});

test("the owner removes any saved report, and removing asks first", async ({ page }) => {
  await signInAs(page, ACCOUNTS.owner);
  await openReports(page);

  for (const name of [accountantReport, ownerReport]) {
    const item = savedItem(page, name);
    await expect(item).toBeVisible({ timeout: 10_000 });
    await item.getByRole("button", { name: `Quitar el informe ${name}` }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toContainText("¿Quitar este informe guardado?");
    await dialog.getByRole("button", { name: "Quitar informe" }).click();
    await expect(savedItem(page, name)).toHaveCount(0, { timeout: 10_000 });
  }
});
