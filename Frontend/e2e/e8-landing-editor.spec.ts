import { expect, test } from "@playwright/test";

import { signIn } from "./helpers";

/**
 * T66: the owner edits the public landing and it shows on the practice host.
 */

const TAGLINE = `E2E landing ${Date.now()}`;

test("the owner saves the landing and demo.localhost shows the new text", async ({ page }) => {
  await signIn(page);

  await page.goto("/mi-consultorio");
  await expect(page.getByRole("heading", { name: "Mi consultorio" })).toBeVisible();

  await page.getByLabel("Frase").fill(TAGLINE);
  await page.getByRole("button", { name: "Guardar landing" }).click();
  await expect(page.getByRole("button", { name: "Guardar landing" })).toBeEnabled({ timeout: 15_000 });

  await page.goto(`http://demo.localhost:3000/`);
  await expect(page.getByText(TAGLINE)).toBeVisible();
});