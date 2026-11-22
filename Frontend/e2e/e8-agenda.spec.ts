import { expect, test } from "@playwright/test";

import { signIn } from "./helpers";

/**
 * T68: agenda filter bar, booking and detail popups.
 */

test("the owner opens Nuevo turno in a popup and an empty day offers agendar", async ({ page }) => {
  await signIn(page);
  await page.goto("/agenda");

  await expect(page.getByRole("search", { name: "Filtros de la agenda" })).toBeVisible();
  await page.getByRole("button", { name: "Nuevo turno" }).click();
  await expect(page.getByRole("dialog", { name: "Nuevo turno" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "Nuevo turno" })).toBeHidden();

  const emptyDay = page.getByRole("button", { name: /Sin turnos — agendar/ }).first();
  await emptyDay.click();
  await expect(page.getByRole("dialog", { name: "Nuevo turno" })).toBeVisible();
});