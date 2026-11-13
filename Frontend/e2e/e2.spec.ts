import { expect, test } from "@playwright/test";

import { signIn } from "./helpers";

test.describe.configure({ mode: "serial" });

const stamp = Date.now();
const patientName = `E2 Paciente ${stamp}`;

test("login reaches the dashboard", async ({ page }) => {
  await signIn(page);
  await expect(page.getByRole("heading", { name: "Aveniq" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Pacientes" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Agenda" })).toBeVisible();
});

test("two tabs can refresh at once without killing the session", async ({ context, page }) => {
  await signIn(page);

  const other = await context.newPage();
  await other.goto("/");
  await expect(other.getByRole("button", { name: "Cerrar sesión" })).toBeVisible({
    timeout: 15_000,
  });

  const [first, second] = await Promise.all([
    page.evaluate(() => window.__aveniqEnsureSession?.() ?? false),
    other.evaluate(() => window.__aveniqEnsureSession?.() ?? false),
  ]);

  expect(first).toBe(true);
  expect(second).toBe(true);
  // /auth/me needs the in-memory access token, not the cookie; the product
  // proof that neither tab spent the other's refresh is that both still show
  // a live session.
  await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible();
  await expect(other.getByRole("button", { name: "Cerrar sesión" })).toBeVisible();

  await other.close();
});

test("a patient can be registered, given a clinical record, and booked", async ({ page }) => {
  await signIn(page);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(patientName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByRole("button", { name: "Crear paciente" }).click();
  await expect(page.getByRole("cell", { name: patientName })).toBeVisible({ timeout: 10_000 });

  await page.getByRole("cell", { name: patientName }).click();
  await expect(page.getByRole("heading", { name: patientName })).toBeVisible();
  await page.getByRole("tab", { name: "Historia clínica" }).click();
  await expect(page.getByRole("heading", { name: "Ficha clínica" })).toBeVisible();

  await page.getByLabel("Motivo de consulta").fill("Insomnio de E2");
  await page.getByRole("button", { name: "Guardar ficha" }).click();
  await expect(page.getByText("Última escritura")).toBeVisible({ timeout: 10_000 });

  await page.getByLabel("Nota", { exact: true }).fill("Primera sesión de verificación E2.");
  await page.getByRole("button", { name: "Registrar evolución" }).click();
  await expect(page.getByText("Primera sesión de verificación E2.")).toBeVisible({
    timeout: 10_000,
  });

  await page.getByRole("link", { name: "Agenda" }).click();
  await expect(page.getByRole("heading", { name: "Agenda" })).toBeVisible();
  await page.getByLabel("Paciente").selectOption({ label: patientName });

  // The week grid is Monday–Sunday. Booking +3 days from Friday lands next week
  // and never appears; book tomorrow (or later today on Sunday).
  const start = new Date();
  if (start.getDay() === 0) {
    start.setHours(start.getHours() + 1, stamp % 50, 0, 0);
  } else {
    start.setDate(start.getDate() + 1);
    start.setHours(16, stamp % 50, 0, 0);
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  const local = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())}T${pad(start.getHours())}:${pad(start.getMinutes())}`;
  await page.getByLabel("Inicio").fill(local);
  await page.getByRole("button", { name: "Reservar" }).click();
  await expect(page.getByRole("button", { name: new RegExp(patientName) })).toBeVisible({
    timeout: 10_000,
  });
});
