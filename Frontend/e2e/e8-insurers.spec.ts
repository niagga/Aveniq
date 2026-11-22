import { expect, test } from "@playwright/test";

import { signIn } from "./helpers";

/** T69: create and edit open in FormModal with blur. */

test("the owner creates a mutual in a popup", async ({ page }) => {
  const name = `E8 Mutual ${Date.now()}`;
  await signIn(page);
  await page.goto("/mutuales");

  await page.getByRole("button", { name: "Nueva mutual" }).click();
  const dialog = page.getByRole("dialog", { name: "Nueva mutual" });
  await expect(dialog).toBeVisible();

  await dialog.getByLabel("Nombre", { exact: true }).fill(name);
  await dialog.getByRole("button", { name: "Crear mutual" }).click();

  await expect(dialog).toBeHidden();
  await expect(page.getByRole("cell", { name })).toBeVisible({ timeout: 10_000 });
});