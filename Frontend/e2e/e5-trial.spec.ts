import { expect, test, type Page } from "@playwright/test";

import { ACCOUNTS, DEMO, EXPIRED, signInAs } from "./helpers";

/**
 * The trial of a practice (E5, T48). Inside the trial a practice writes. Once the trial has ended
 * it is read-only: the screens say so, the write controls are off, and the API refuses the write
 * with 403 trial_expired while reads still answer.
 *
 * The expired practice is seeded by scripts/local-seed.ps1 with its trial ended yesterday. Every
 * negative check sits next to a positive one in the same view, so a page that has not loaded yet
 * cannot pass for a page that blocks the control.
 */

test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const patientInsideTrial = `E5 Dentro del trial ${stamp}`;

const bannerTitle = "La prueba de este consultorio venció";

// The other journeys write into the demo practice. If its trial had ended, they would fail for the
// wrong reason, so this runs first and says so.
test("the demo practice is inside its trial before the journeys write into it", async ({ request }) => {
  const login = await request.post("/api/v1/auth/login", {
    data: { tenantSlug: DEMO.tenant, email: DEMO.email, password: DEMO.password },
  });
  expect(login.ok()).toBeTruthy();
  const { accessToken } = (await login.json()) as { accessToken: string };
  const tenants = await request.get("/api/v1/tenants", { headers: { Authorization: `Bearer ${accessToken}` } });
  expect(tenants.ok()).toBeTruthy();
  const body = (await tenants.json()) as { items: { slug: string; status: string; trialEndsAt: string }[] };
  const demo = body.items.find((item) => item.slug === DEMO.tenant);
  expect(demo, "demo practice is listed").toBeDefined();
  expect(demo?.status).toBe("trial");
  expect(Date.parse(demo?.trialEndsAt ?? "")).toBeGreaterThan(Date.now());
});

async function openRoster(page: Page) {
  await page.goto("/patients");
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
}

test("inside the trial the owner writes: a patient is created and no banner shows", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await signInAs(page, ACCOUNTS.owner);
    await openRoster(page);

    // Positive control: the write control is enabled, so the absence of a lock is real.
    const newPatient = page.getByRole("button", { name: "Nuevo paciente" });
    await expect(newPatient).toBeEnabled();
    await expect(page.getByText(bannerTitle)).toHaveCount(0);

    await newPatient.click();
    await page.getByLabel("Nombre y apellido").fill(patientInsideTrial);
    await page.getByLabel("Fecha de nacimiento").fill("1988-03-04");
    await page.getByLabel("Moneda de facturación").selectOption("ARS");
    await page.getByRole("button", { name: "Crear paciente" }).click();

    // The roster shows 50 patients by name and the development database keeps growing, so the
    // new patient is found by its exact name, not by its place in the list.
    await page.getByLabel("Buscar", { exact: true }).fill(patientInsideTrial);
    await page.getByRole("button", { name: "Buscar", exact: true }).click();
    await expect(page.getByRole("cell", { name: patientInsideTrial })).toBeVisible({ timeout: 10_000 });
  } finally {
    await context.close();
  }
});

test("with the trial ended the owner reads the roster and the write controls are off", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await signInAs(page, EXPIRED.owner, EXPIRED.tenant);

    // Positive control: the dashboard is up and says the trial has ended, with the date.
    await expect(page.getByRole("link", { name: "Pacientes" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(bannerTitle)).toBeVisible();

    // Reads still work: the roster loads.
    const loaded = page.waitForResponse(
      (response) => response.url().includes("/api/v1/patients?") && response.ok(),
    );
    await openRoster(page);
    await loaded;

    // The create control is there and disabled, not hidden: the owner sees why it is off.
    await expect(page.getByRole("button", { name: "Nuevo paciente" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Nuevo paciente" })).toBeVisible();
  } finally {
    await context.close();
  }
});

test("the API refuses a write from the expired practice with 403 trial_expired and still reads", async ({
  request,
}) => {
  // Positive control first: the sign-in itself is not a write and must work for an expired practice.
  const login = await request.post("/api/v1/auth/login", {
    data: { tenantSlug: EXPIRED.tenant, email: EXPIRED.owner.email, password: EXPIRED.owner.password },
  });
  expect(login.ok()).toBeTruthy();
  const { accessToken } = (await login.json()) as { accessToken: string };
  const bearer = { Authorization: `Bearer ${accessToken}` };

  const read = await request.get("/api/v1/patients", { headers: bearer });
  expect(read.status()).toBe(200);

  const write = await request.post("/api/v1/patients", {
    headers: bearer,
    data: { fullName: `E5 Vencido ${stamp}`, coverageKind: "private", billingCurrency: "ARS" },
  });
  expect(write.status()).toBe(403);
  const problem = (await write.json()) as { code?: string };
  expect(problem.code).toBe("trial_expired");
});

test("the expired practice's billing screen reads the history and offers no issue", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await signInAs(page, EXPIRED.owner, EXPIRED.tenant);
    await page.goto("/facturacion");

    // Positive control: the screen has loaded for the owner, who may issue in a live practice.
    await expect(page.getByRole("heading", { name: "Facturación" })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("heading", { name: "Emitir un comprobante" })).toBeVisible();
    await expect(page.getByText(bannerTitle)).toBeVisible();

    await expect(page.getByLabel("Concepto")).toBeDisabled();
    await expect(page.getByRole("button", { name: "Emitir comprobante" })).toBeDisabled();
  } finally {
    await context.close();
  }
});

test("the expired practice's accountant is told too, and cannot issue", async ({ browser }) => {
  // L59: the accountant holds no tenant:read, so it used to find out only from a 403 after
  // typing. The status route is open to every role, and the shell and the forms read it.
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await signInAs(page, EXPIRED.accountant, EXPIRED.tenant);

    // Positive control: the accountant lands on billing, which loaded for it.
    await expect(page).toHaveURL(/\/facturacion$/);
    await expect(page.getByRole("heading", { name: "Emitir un comprobante" })).toBeVisible({
      timeout: 15_000,
    });

    await expect(page.getByText(bannerTitle)).toBeVisible();
    await expect(page.getByLabel("Concepto")).toBeDisabled();
    await expect(page.getByRole("button", { name: "Emitir comprobante" })).toBeDisabled();
  } finally {
    await context.close();
  }
});
