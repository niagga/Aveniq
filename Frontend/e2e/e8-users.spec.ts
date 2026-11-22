import { expect, test } from "@playwright/test";

import { DEMO, signIn } from "./helpers";

/** T70: invite and edit open in FormModal; the issued link is copied from the popup. */

test("the owner invites someone from a popup and gets a copyable link", async ({ page }) => {
  const email = `e8-user-${Date.now()}@aveniq.local`;
  await signIn(page);
  await page.goto("/usuarios");

  await page.getByRole("button", { name: "Invitar" }).click();
  const invite = page.getByRole("dialog", { name: "Invitar a una persona" });
  await expect(invite).toBeVisible();

  await invite.getByLabel("Email").fill(email);
  await invite.getByRole("button", { name: "Generar enlace" }).click();

  const issued = page.getByRole("dialog", { name: "Enlace de invitación" });
  await expect(issued).toBeVisible({ timeout: 15_000 });
  await expect(issued.getByLabel("Enlace de invitación")).toHaveValue(/\/invitacion\?token=/);
  await expect(issued.getByRole("button", { name: "Copiar enlace" })).toBeVisible();

  await issued.getByRole("button", { name: "Listo" }).click();
  await expect(issued).toBeHidden();
  await expect(page.getByRole("row").filter({ hasText: email })).toBeVisible();
});

test("the owner edits a professional role in a popup", async ({ page }) => {
  await signIn(page);
  await page.goto("/usuarios");

  const row = page.getByRole("row").filter({ hasText: DEMO.email }).first();
  await expect(row).toBeVisible();

  const other = page
    .getByRole("row")
    .filter({ has: page.getByRole("cell", { name: "Profesional" }) })
    .filter({ hasNotText: DEMO.email })
    .first();

  const hasOther = await other.isVisible().catch(() => false);
  test.skip(!hasOther, "No hay otra cuenta profesional en el seed para editar");

  await other.getByRole("button", { name: "Editar" }).click();
  const dialog = page.getByRole("dialog", { name: "Editar cuenta" });
  await expect(dialog).toBeVisible();

  await dialog.getByLabel("Rol").selectOption("assistant");
  await dialog.getByRole("button", { name: "Guardar cambios" }).click();
  await expect(dialog).toBeHidden({ timeout: 15_000 });
  await expect(other.getByRole("cell", { name: "Recepción" })).toBeVisible();
});