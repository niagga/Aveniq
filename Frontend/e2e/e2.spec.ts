import { expect, test } from "@playwright/test";

import { ACCOUNTS, signIn, signInAs } from "./helpers";

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

test("a professional registers a patient, keeps its clinical record, and books it", async ({ page }) => {
  // Only a professional opens the clinical record, and registering a patient makes that
  // professional its treating one (T38). The owner has no clinical tab.
  await signInAs(page, ACCOUNTS.professional);
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
  // The professional keeps earlier appointments from previous runs, so try whole hours
  // until one is free. Each slot is on the hour and lasts 45 minutes, so no two can overlap.
  const day = new Date();
  if (day.getDay() === 0) {
    day.setHours(day.getHours() + 1, 0, 0, 0);
  } else {
    day.setDate(day.getDate() + 1);
  }
  const pad = (n: number) => String(n).padStart(2, "0");
  const booked = page.getByRole("button", { name: new RegExp(patientName) });
  let isBooked = false;
  for (let hour = 8; hour < 20 && !isBooked; hour++) {
    const start = new Date(day);
    start.setHours(hour, 0, 0, 0);
    if (start < new Date()) continue;
    const local = `${start.getFullYear()}-${pad(start.getMonth() + 1)}-${pad(start.getDate())}T${pad(start.getHours())}:00`;
    await page.getByLabel("Inicio").fill(local);
    await page.getByRole("button", { name: "Reservar" }).click();
    isBooked = await booked.waitFor({ state: "visible", timeout: 5_000 }).then(
      () => true,
      () => false,
    );
  }
  expect(isBooked, "no free hour left on the next day").toBe(true);
});
