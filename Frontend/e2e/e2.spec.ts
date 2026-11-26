import { expect, test } from "@playwright/test";

import { ACCOUNTS, bookFirstFreeHour, openBookingPopup, searchRoster, signIn, signInAs } from "./helpers";

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
  await other.goto("/panel");
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
  // Registration plus the booking walk over the coming days go past the 30 s default.
  test.setTimeout(120_000);

  // Only a professional opens the clinical record, and registering a patient makes that
  // professional its treating one (T38). The owner has no clinical tab.
  await signInAs(page, ACCOUNTS.professional);
  await page.getByRole("link", { name: "Pacientes" }).click();
  await expect(page.getByRole("heading", { name: "Pacientes" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo paciente" }).click();
  await page.getByLabel("Nombre y apellido").fill(patientName);
  await page.getByLabel("Fecha de nacimiento").fill("1990-05-12");
  await page.getByLabel("Moneda de facturación").selectOption("ARS");
  await page.getByRole("button", { name: "Crear paciente" }).click();
  // The roster shows one page of 50 in name order and the development database keeps
  // accumulating runs, so the new patient is found by the roster's search (T77, as e4/e5/e6).
  await searchRoster(page, patientName);
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

  // The booking form opens in a popup since T68: the fields live inside "Nuevo turno". The
  // helper names the patient and the modality — no default (L61) — and walks the whole hours
  // of the coming days (T77), moving to the next day when one is full, instead of being
  // pinned to tomorrow and giving up once it fills.
  const booking = await openBookingPopup(page);
  await bookFirstFreeHour(page, booking, { patientName, modality: "presencial" });

  // The booked slot shows the modality the professional named.
  const booked = page.getByRole("button", { name: new RegExp(patientName) });
  await expect(booked.first()).toContainText("Presencial");
});
