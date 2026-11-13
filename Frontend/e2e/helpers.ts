import { expect, type Page } from "@playwright/test";

export const DEMO = {
  tenant: process.env.E2E_TENANT ?? "demo",
  email: process.env.E2E_EMAIL ?? "demo@aveniq.local",
  password: process.env.E2E_PASSWORD ?? "demo-aveniq-2026",
};

/** Signs in through the product form and waits until the dashboard is up. */
export async function signIn(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Consultorio").fill(DEMO.tenant);
  await page.getByLabel("Email").fill(DEMO.email);
  await page.getByLabel("Contraseña").fill(DEMO.password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible({
    timeout: 15_000,
  });
}
