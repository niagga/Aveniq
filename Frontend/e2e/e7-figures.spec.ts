import { readFile } from "node:fs/promises";

import { expect, test, type Browser, type Page } from "@playwright/test";

import { ACCOUNTS, signInAs, type Account } from "./helpers";

/**
 * The figures of E7 end to end (T61): what the accountant bills moves the report, a voided
 * voucher does not, the dashboard and the CSV say what the report says, and each role reads the
 * scope it is entitled to. The saved reports have their own journey in e7-reports.spec.ts.
 *
 * Nothing here depends on the data already in the practice: every check is a difference against
 * a figure read just before, or a comparison between two views of the same figure.
 */
test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const keptConcept = `E7 Cifras vigente ${stamp}`;
const voidedConcept = `E7 Cifras anulado ${stamp}`;

// 1.500,00 is voided, 800,50 stays: the report must move by the second one only.
const KEPT_MINOR = 80_050;

type Billed = { minor: number; vouchers: number; text: string };

async function openAs(browser: Browser, account: Account): Promise<{ page: Page; close: () => Promise<void> }> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signInAs(page, account);
  return { page, close: () => context.close() };
}

/**
 * The access token the application itself sends, taken from its own requests: the token lives in
 * memory, so a test that wants to talk to the API directly borrows it instead of logging in twice.
 */
function watchBearer(page: Page): () => string {
  let token = "";
  page.on("request", (request) => {
    const header = request.headers()["authorization"];
    if (request.url().includes("/api/v1/") && header?.startsWith("Bearer ")) token = header;
  });
  return () => {
    if (token === "") throw new Error("the page has not sent an authenticated request yet");
    return token;
  };
}

/** "ARS 1.500,00" → 150000. */
function toMinor(text: string): number {
  const match = /ARS (-?[\d.]+),(\d{2})/.exec(text);
  if (match === null) throw new Error(`no ARS amount in "${text}"`);
  return Number((match[1] ?? "").replaceAll(".", "")) * 100 + Number(match[2]);
}

/** Opens the reports page with the month so far and no cut, and reads the billed total in ARS. */
async function readBilled(page: Page): Promise<Billed> {
  await page.goto("/informes");
  await expect(page.getByRole("heading", { name: "Informes", level: 1 })).toBeVisible({ timeout: 15_000 });
  await page
    .getByRole("group", { name: "Período" })
    .getByRole("button", { name: "Este mes" })
    .click();
  await page
    .getByRole("group", { name: "Cortar por" })
    .getByRole("button", { name: "Sin corte" })
    .click();

  const results = page.getByRole("region", { name: "Resultado del informe" });
  // With no cut the only row is the total of the range, labelled with its days.
  const total = results.getByRole("row", { name: /\d{4}-\d{2}-\d{2} a \d{4}-\d{2}-\d{2}/ });
  await expect(total).toBeVisible({ timeout: 15_000 });

  const cells = total.getByRole("cell");
  const text = (await cells.nth(0).innerText()).trim();
  return { minor: toMinor(text), vouchers: Number(await cells.nth(1).innerText()), text };
}

test("what the accountant issues moves the billed figure, and a voided voucher does not", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.accountant);
  try {
    const before = await readBilled(page);

    for (const [concept, amount] of [
      [voidedConcept, "1500"],
      [keptConcept, "800,50"],
    ] as const) {
      await page.goto("/facturacion");
      await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
      await page.getByLabel("Concepto").fill(concept);
      await page.getByLabel("Importe (ARS)").fill(amount);
      await page.getByRole("button", { name: "Emitir comprobante" }).click();
      await expect(page.getByText(/Emitimos el comprobante N° \d{6}/)).toBeVisible({ timeout: 10_000 });
    }

    // Both are in the figure while both are issued.
    const both = await readBilled(page);
    expect(both.minor - before.minor).toBe(150_000 + KEPT_MINOR);
    expect(both.vouchers - before.vouchers).toBe(2);

    // Voiding one takes it out of the figure and nothing else.
    await page.goto("/facturacion");
    const row = page.getByRole("row").filter({ hasText: voidedConcept });
    await expect(row).toBeVisible({ timeout: 10_000 });
    await row.getByRole("link", { name: /^Ver el comprobante/ }).click();
    const detail = page.getByRole("region", { name: /Detalle del comprobante/ });
    await detail.getByRole("button", { name: "Anular comprobante" }).click();
    await detail.getByLabel("Motivo de la anulación").fill("Prueba de cifras E7");
    await detail.getByRole("button", { name: "Confirmar anulación" }).click();
    await expect(detail.getByText(/^Anulado el /)).toBeVisible({ timeout: 10_000 });

    const after = await readBilled(page);
    expect(after.minor - before.minor).toBe(KEPT_MINOR);
    expect(after.vouchers - before.vouchers).toBe(1);
  } finally {
    await close();
  }
});

test("the dashboard and the CSV say what the report says", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.owner);
  try {
    const report = await readBilled(page);

    // The CSV is built from the figures on screen: its total line must be the same number.
    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "Exportar CSV" }).click(),
    ]);
    const text = await readFile(await download.path(), "utf8");
    const line = text
      .slice(1)
      .split("\r\n")
      .find((candidate) => candidate.startsWith("Total;ARS;"));
    expect(line, "the CSV has no ARS total line").toBeDefined();
    const cells = (line ?? "").split(";");
    // "8005,50" is the decimal comma of the file, without thousands separators.
    expect(Number((cells[2] ?? "").replace(",", "."))).toBeCloseTo(report.minor / 100, 2);
    expect(Number(cells[3])).toBe(report.vouchers);

    // The dashboard reads the same month with the same rule.
    await page.goto("/panel");
    await expect(page.getByRole("heading", { name: "Cifras del consultorio" })).toBeVisible({ timeout: 15_000 });
    // The headline card comes before the charts, which repeat the word in their legends.
    const card = page.getByText("Facturado", { exact: true }).first().locator("xpath=..");
    await expect(card).toContainText(report.text, { timeout: 15_000 });
    await expect(card).toContainText(report.vouchers === 1 ? "1 comprobante emitido" : `${report.vouchers} comprobantes emitidos`);
  } finally {
    await close();
  }
});

test("the accountant and the administration read the whole practice from the dashboard", async ({ browser }) => {
  for (const account of [ACCOUNTS.accountant, ACCOUNTS.admin]) {
    const { page, close } = await openAs(browser, account);
    try {
      await page.goto("/panel");
      await expect(page.getByRole("heading", { name: "Cifras del consultorio" })).toBeVisible({
        timeout: 15_000,
      });
      await expect(page.getByText("Facturado", { exact: true }).first()).toBeVisible();
      await expect(page.getByRole("link", { name: "Informes", exact: true })).toBeVisible();
    } finally {
      await close();
    }
  }
});

test("the professional reads only their own figures, never more than the practice", async ({ browser }) => {
  const owner = await openAs(browser, ACCOUNTS.owner);
  const professional = await openAs(browser, ACCOUNTS.professional);
  const token = watchBearer(professional.page);
  try {
    const practice = await readBilled(owner.page);
    const own = await readBilled(professional.page);

    await expect(
      professional.page.getByRole("region", { name: "Resultado del informe" }),
    ).toContainText("Solo tus turnos y comprobantes.");
    // The loose vouchers of the previous test belong to nobody: they are in the practice's
    // figure and cannot be in this professional's.
    expect(own.minor).toBeLessThanOrEqual(practice.minor - KEPT_MINOR);
    expect(own.vouchers).toBeLessThan(practice.vouchers);

    await professional.page.goto("/panel");
    await expect(professional.page.getByRole("heading", { name: "Tus cifras" })).toBeVisible({
      timeout: 15_000,
    });
    await expect(professional.page.getByRole("heading", { name: "Cifras del consultorio" })).toHaveCount(0);
    // Asking the API for the whole practice cut by professional is not a way around the scope.
    const response = await professional.page.request.get(
      "/api/v1/reports/billing?from=2023-01-01&to=2026-12-31&groupBy=professional",
      { headers: { Authorization: token() } },
    );
    expect(response.status()).toBe(200);
    const body = (await response.json()) as { scope: string; rows: unknown[] | null };
    expect(body.scope).toBe("own");
    expect(body.rows?.length ?? 0).toBeLessThanOrEqual(1);
  } finally {
    await owner.close();
    await professional.close();
  }
});

test("the assistant has neither the figures nor the report", async ({ browser }) => {
  const { page, close } = await openAs(browser, ACCOUNTS.assistant);
  const token = watchBearer(page);
  try {
    await page.goto("/panel");
    await expect(page.getByRole("heading", { name: /^Cifras|^Tus cifras/ })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Informes", exact: true })).toHaveCount(0);

    // The agenda is a screen the assistant uses, so it sends authenticated requests to borrow from.
    await page.goto("/agenda");
    await page.waitForLoadState("networkidle");
    const response = await page.request.get("/api/v1/reports/billing?from=2026-11-01&to=2026-11-30", {
      headers: { Authorization: token() },
    });
    expect(response.status()).toBe(403);;
  } finally {
    await close();
  }
});
