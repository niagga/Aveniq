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
} as const;

export type Account = { email: string; password: string; name: string };

/** Signs in as the given account through the product form and waits for the dashboard. */
export async function signInAs(page: Page, account: Account): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Consultorio").fill(DEMO.tenant);
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
