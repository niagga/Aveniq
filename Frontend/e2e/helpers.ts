import { expect, type Page } from "@playwright/test";

// The development password is never written into the repository. It is the value the seed
// was run with (AVENIQ_SEED_PASSWORD in scripts/local-seed.ps1), passed in here as E2E_PASSWORD.
const password = process.env.E2E_PASSWORD;
if (!password) {
  throw new Error(
    "E2E_PASSWORD is not set. Use the same value as AVENIQ_SEED_PASSWORD, the one the local seed was run with.",
  );
}

export const DEMO = {
  tenant: process.env.E2E_TENANT ?? "demo",
  email: process.env.E2E_EMAIL ?? "demo@aveniq.local",
  password,
};

/**
 * The accounts the seed creates in the demo practice (scripts/local-seed.ps1). The names
 * are what the screens show, so tests can pick a professional from a select by its label.
 */
export const ACCOUNTS = {
  owner: { email: DEMO.email, password: DEMO.password, name: "Demo Owner" },
  professional: { email: "profesional@aveniq.local", password: DEMO.password, name: "Demo Profesional" },
  otherProfessional: {
    email: "profesional2@aveniq.local",
    password: DEMO.password,
    name: "Demo Profesional 2",
  },
  assistant: { email: "recepcion@aveniq.local", password: DEMO.password, name: "Demo Recepcion" },
  // The accountant of E4 (S22) and the administrator who only reads billing (L46).
  accountant: { email: "contador@aveniq.local", password: DEMO.password, name: "Demo Contador" },
  admin: { email: "admin@aveniq.local", password: DEMO.password, name: "Demo Administracion" },
} as const;

export type Account = { email: string; password: string; name: string };

/**
 * The practice whose trial has ended (T48). The seed sets its end in the past on every run.
 * Its owner can read everything and write nothing.
 */
export const EXPIRED = {
  tenant: "demo-vencido",
  owner: { email: "vencido@aveniq.local", password: DEMO.password, name: "Demo Vencido" },
  // The accountant of that practice (L59): no tenant:read, so only the status route tells it.
  accountant: {
    email: "contador-vencido@aveniq.local",
    password: DEMO.password,
    name: "Demo Contador Vencido",
  },
} as const;

/** Signs in as the given account through the product form and waits for the dashboard. */
export async function signInAs(page: Page, account: Account, tenant: string = DEMO.tenant): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Consultorio").fill(tenant);
  await page.getByLabel("Email").fill(account.email);
  await page.getByLabel("Contraseña").fill(account.password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible({
    timeout: 15_000,
  });
}

/** Signs in as the demo owner. */
export async function signIn(page: Page): Promise<void> {
  await signInAs(page, ACCOUNTS.owner);
}
