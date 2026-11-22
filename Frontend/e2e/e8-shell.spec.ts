import { expect, test } from "@playwright/test";

import { ACCOUNTS, signIn, signInAs } from "./helpers";

/**
 * The shell of E8 (T63): `/` is the public front door, the dashboard lives at `/panel`, the rail
 * labels itself on a wide screen, and on a phone the sections that do not fit the bottom bar open
 * from "Más" instead of being unreachable.
 */

const RAIL = "Secciones del consultorio";

test("on the platform host the first screen is public Aveniq and leads to sign-in", async ({
  page,
}) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", { name: "Gestión de consultorios de psicología" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByLabel("Consultorio")).toBeVisible();
});

test("on demo.localhost the first screen is the practice landing and sign-in hides Consultorio", async ({
  page,
}) => {
  await page.goto("http://demo.localhost:3000/");

  await expect(page.getByRole("heading", { name: "Consultorio Demo" })).toBeVisible();
  await page.getByRole("link", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByLabel("Consultorio")).toBeHidden();
});

test("the owner lands on the dashboard at /panel, and the rail marks Inicio", async ({ page }) => {
  await signIn(page);

  await expect(page).toHaveURL(/\/panel$/);
  await expect(
    page.getByRole("navigation", { name: RAIL }).getByRole("link", { name: "Inicio" }),
  ).toHaveAttribute("aria-current", "page");
});

test("the accountant still lands on billing, not on the dashboard", async ({ page }) => {
  await signInAs(page, ACCOUNTS.accountant);

  await expect(page).toHaveURL(/\/facturacion$/);
});

test("on a wide screen the rail shows the section names next to the icons", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await signIn(page);

  const link = page.getByRole("navigation", { name: RAIL }).getByRole("link", { name: "Pacientes" });
  await expect(link).toBeVisible();
  // An icon-only rail is 64px wide and its links are 44px; with the labels a link is the width of its row.
  const box = await link.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThan(120);
});

test("on a phone the sections past the fourth open from Más, and the popup closes after one", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page);

  const rail = page.getByRole("navigation", { name: RAIL });
  await expect(rail.getByRole("link", { name: "Pacientes" })).toBeVisible();
  await expect(rail.getByRole("link", { name: "Usuarios" })).toBeHidden();

  // The bottom bar is fixed: after scrolling to the end of a long screen it is still in reach.
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  const bar = await rail.boundingBox();
  expect(bar).not.toBeNull();
  expect(bar!.y + bar!.height).toBeCloseTo(844, 0);

  await rail.getByRole("button", { name: "Más" }).click();
  const popup = page.getByRole("dialog", { name: "Más secciones" });
  await expect(popup).toBeVisible();
  for (const label of ["Liquidaciones", "Informes", "Tarifario", "Mutuales", "Usuarios"]) {
    await expect(popup.getByRole("link", { name: label })).toBeVisible();
  }

  await popup.getByRole("link", { name: "Usuarios" }).click();
  await expect(page).toHaveURL(/\/usuarios$/);
  await expect(page.getByRole("dialog", { name: "Más secciones" })).toBeHidden();
});

test("Escape closes the Más popup without leaving the screen", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page);

  await page.getByRole("navigation", { name: RAIL }).getByRole("button", { name: "Más" }).click();
  await expect(page.getByRole("dialog", { name: "Más secciones" })).toBeVisible();
  await page.keyboard.press("Escape");

  await expect(page.getByRole("dialog", { name: "Más secciones" })).toBeHidden();
  await expect(page).toHaveURL(/\/panel$/);
});
