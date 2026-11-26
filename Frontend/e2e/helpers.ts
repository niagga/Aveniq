import { createHash } from "node:crypto";

import { expect, type Locator, type Page } from "@playwright/test";

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
  // The accountant of E4 (S22) and the administrator who only reads billing (L46).
  accountant: { email: "contador@aveniq.local", password: DEMO.password, name: "Demo Contador" },
  admin: { email: "admin@aveniq.local", password: DEMO.password, name: "Demo Administracion" },
} as const;

export type Account = { email: string; password: string; name: string };

/**
 * The standing invitation the seed leaves in the expired practice (L58). The seed derives the
 * secret from the development password; so does this, so no secret is committed.
 */
export function expiredInvitationToken(practiceId: string): string {
  const secret = createHash("sha256")
    .update(`aveniq-e2e-expired-invitation:${DEMO.password}`)
    .digest("base64url");
  return `inv1.${practiceId}.${secret}`;
}

/**
 * The practice whose trial has ended (T48). The seed sets its end in the past on every run.
 * Its owner can read everything and write nothing.
 */
export const EXPIRED = {
  tenant: "demo-vencido",
  owner: { email: "vencido@aveniq.local", password: DEMO.password, name: "Demo Vencido" },
  // The accountant of that practice (L59): no tenant:read, so only the status route tells it.
  accountant: {
    email: "contador-vencido@aveniq.local",
    password: DEMO.password,
    name: "Demo Contador Vencido",
  },
} as const;

/** Signs in as the given account through the product form and waits for the dashboard. */
export async function signInAs(page: Page, account: Account, tenant: string = DEMO.tenant): Promise<void> {
  await page.goto("/login");
  const consultorio = page.getByLabel("Consultorio");
  if (await consultorio.isVisible()) {
    await consultorio.fill(tenant);
  }
  await page.getByLabel("Email").fill(account.email);
  await page.getByLabel("Contraseña").fill(account.password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("button", { name: "Cerrar sesión" })).toBeVisible({
    timeout: 15_000,
  });
}

/**
 * Searches the roster and waits for the list that answers this exact search (a negative
 * check has to run on a loaded list).
 *
 * The fill can lose to the re-render that follows the create popup closing (T75): the
 * controlled "Buscar" input belongs to that render and reverts to what its state had, so
 * the submit then asks for an empty search. Retrying until the request carries the text is
 * the deterministic fact here; a missing patient still fails, it just cannot fail by race.
 */
export async function searchRoster(page: Page, text: string): Promise<void> {
  for (let attempt = 0; attempt < 5; attempt++) {
    const answered = page.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/patients?") &&
        response.status() === 200 &&
        new URL(response.url()).searchParams.get("search") === text,
      { timeout: 3_000 },
    );
    const box = page.getByLabel("Buscar", { exact: true });
    await box.fill(text);
    await page.getByRole("button", { name: "Buscar", exact: true }).click();
    const ok = await answered.then(
      () => true,
      () => false,
    );
    if (ok && (await box.inputValue()) === text) return;
  }
  throw new Error(`the roster never answered the search for "${text}"`);
}

/** Signs in as the demo owner. */
export async function signIn(page: Page): Promise<void> {
  await signInAs(page, ACCOUNTS.owner);
}

/**
 * Opens the agenda's booking popup (T68, S25): the booking form no longer lives inline
 * under the filter bar, it opens with "Nuevo turno". Each refused hour keeps the popup
 * open with its error, so a caller that walks free hours retries inside the same dialog.
 */
export async function openBookingPopup(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: "Nuevo turno" }).click();
  const dialog = page.getByRole("dialog", { name: "Nuevo turno" });
  await expect(dialog).toBeVisible();
  return dialog;
}

/** The zone the demo practice runs in: it is the week the agenda grid renders. */
const PRACTICE_ZONE = "America/Argentina/Buenos_Aires";

/** The whole hours the booking walk tries on every candidate day: 8:00 through 19:00. */
const firstWalkHour = 8;
const lastWalkHour = 19;

const pad = (value: number) => String(value).padStart(2, "0");

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Monday 00:00 of the week containing `anchor`, in the practice zone — the same instant
 * the agenda's `startOfWeek` computes, so the helper can match the `from` of the week the
 * grid loads when it navigates.
 *
 * Standalone copies of `civilDate` and `zonedDate` (entities/schedule/model.ts): the e2e
 * suite cannot import the "@/" aliased app source.
 */
function civilPartsInZone(date: Date, timeZone: string): { year: number; month: number; day: number } {
  const formatted = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const lookup = Object.fromEntries(formatted.map((part) => [part.type, part.value]));
  return { year: Number(lookup.year), month: Number(lookup.month), day: Number(lookup.day) };
}

function zonedDate(
  year: number,
  month: number,
  day: number,
  hour: number,
  minute: number,
  timeZone: string,
): Date {
  // Approximate: construct a UTC instant and shift until the civil fields match.
  const guess = new Date(Date.UTC(year, month - 1, day, hour, minute));
  const parts = civilPartsInZone(guess, timeZone);
  const deltaDays = Date.UTC(year, month - 1, day) - Date.UTC(parts.year, parts.month - 1, parts.day);
  return new Date(guess.getTime() + deltaDays);
}

function mondayOfWeek(anchor: Date): Date {
  const parts = civilPartsInZone(anchor, PRACTICE_ZONE);
  const weekday = new Date(Date.UTC(parts.year, parts.month - 1, parts.day)).getUTCDay(); // 0 Sunday
  const daysFromMonday = (weekday + 6) % 7;
  return zonedDate(parts.year, parts.month, parts.day - daysFromMonday, 0, 0, PRACTICE_ZONE);
}

function addDays(date: Date, days: number): Date {
  return new Date(date.getTime() + days * 24 * 60 * 60 * 1000);
}

/** The local `datetime-local` value of an instant, as the booking form stores it. */
function localDateTimeValue(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:00`;
}

/**
 * Names the patient in the booking popup's picker (T80): the popup's select holds the
 * page of the roster the agenda loads when it opens, and a practice with more active
 * patients than that (and any development database with runs behind it) has patients
 * beyond it. So the test types the patient into "Buscar paciente", waits for the API
 * answer to this exact search and only then picks the patient from the narrowed options:
 * no assumption on the name's place in the roster, no `.first()`, no forced clicks.
 *
 * Clearing the input first makes the next fill a real change even when the walk reopens
 * the popup with the same name already typed in, so it is always this search's own
 * answer that is awaited.
 */
export async function pickBookingPatient(booking: Locator, patientName: string): Promise<void> {
  const search = booking.getByLabel("Buscar paciente");
  if ((await search.inputValue()) !== "") {
    await search.fill("");
  }
  const answered = booking.page().waitForResponse(
    (response) =>
      response.url().includes("/api/v1/patients?") &&
      response.status() === 200 &&
      new URL(response.url()).searchParams.get("search") === patientName,
  );
  await search.fill(patientName);
  await answered;
  // The picker's select is the one combobox in the popup named "Paciente": getByLabel with
  // exact does not match a wrapped-label select (the input picker is a searchbox, so the
  // plain label locator is ambiguous), the role locator is not.
  await booking
    .getByRole("combobox", { name: "Paciente" })
    .selectOption({ label: patientName });
}

/**
 * Books a free whole hour for the patient named inside the open booking popup (T68):
 * it walks the whole hours 8:00–19:00 of tomorrow — later today first, on a Sunday, when
 * the Monday–Sunday week the grid shows has no tomorrow — and when a day is full it moves
 * to the following day, up to `maxDays` candidate days (7 covers a week of 12 hours).
 *
 * The popup stays open with its error when an hour is refused (Conflict) and closes when
 * the hour is taken, so the walk reads the POST /api/v1/appointments answer instead of
 * probing the grid: a refusal costs no idle wait, and a day outside the shown week does
 * not hide a booking. When it does walk into a later week, it closes the popup (the walk
 * made its form dirty, so "Cancelar" asks to discard), moves the grid with "Semana
 * siguiente" and reopens it.
 *
 * It returns the instant the booked hour starts, with the agenda showing the booked day
 * so callers can assert on the slot. When the whole window is full it throws, naming what
 * it searched: an unbooked session must fail its test, never skip silently.
 */
export async function bookFirstFreeHour(
  page: Page,
  booking: Locator,
  options: { patientName: string; modality: string; maxDays?: number },
): Promise<Date> {
  const { patientName, modality, maxDays = 7 } = options;
  const slotButton = page.getByRole("button", { name: new RegExp(escapeRegExp(patientName)) });

  const now = new Date();
  const firstDay =
    now.getDay() === 0
      ? new Date(now.getFullYear(), now.getMonth(), now.getDate())
      : addDays(new Date(now.getFullYear(), now.getMonth(), now.getDate()), 1);
  const lastDay = addDays(firstDay, maxDays - 1);

  // The patient and the modality have no default (L61): the walk sets both, and again on
  // every reopening below. The patient goes through the picker's search (T80), so the
  // name's place in the roster never matters.
  async function namePatientAndModality(currentBooking: Locator): Promise<void> {
    await pickBookingPatient(currentBooking, patientName);
    await currentBooking.getByLabel("Modalidad").selectOption(modality);
  }
  await namePatientAndModality(booking);

  // The week the grid shows when the walk starts: the agenda opens on the week of today.
  let shownMonday = mondayOfWeek(new Date());
  let refused = 0;
  let lastRefusal = "";

  for (let offset = 0; offset < maxDays; offset++) {
    const day = addDays(firstDay, offset);
    const targetMonday = mondayOfWeek(day);

    if (targetMonday.getTime() !== shownMonday.getTime()) {
      // The walk filled the popup, so "Cancelar" opens the unsaved-changes guard before
      // closing: discard it. The modal covers the page, so the grid is reached only after.
      await booking.getByRole("button", { name: "Cancelar" }).click();
      await page.getByRole("button", { name: "Descartar cambios" }).click();
      await expect(booking).toBeHidden();
      while (targetMonday.getTime() > shownMonday.getTime()) {
        const nextMonday = addDays(shownMonday, 7);
        const reloaded = page.waitForResponse(
          (response) =>
            response.request().method() === "GET" &&
            response.url().includes("/api/v1/appointments") &&
            new URL(response.url()).searchParams.get("from") === nextMonday.toISOString(),
          { timeout: 10_000 },
        );
        await page.getByRole("button", { name: "Semana siguiente" }).click();
        const answer = await reloaded;
        if (!answer.ok()) {
          throw new Error(`bookFirstFreeHour: the agenda week did not load (${answer.status()}).`);
        }
        shownMonday = nextMonday;
      }
      booking = await openBookingPopup(page);
      await namePatientAndModality(booking);
    }

    for (let hour = firstWalkHour; hour <= lastWalkHour; hour++) {
      const start = new Date(day.getFullYear(), day.getMonth(), day.getDate(), hour);
      if (start < new Date()) continue; // past hours of the Sunday start were never free
      const attempted = page.waitForResponse(
        (response) =>
          response.request().method() === "POST" &&
          response.url().includes("/api/v1/appointments") &&
          !response.url().includes("appointment-series"),
        { timeout: 10_000 },
      );
      await booking.getByLabel("Inicio").fill(localDateTimeValue(start));
      await booking.getByRole("button", { name: "Reservar" }).click();
      const answer = await attempted;
      if (answer.ok()) {
        await expect(booking).toBeHidden();
        await expect(slotButton).toBeVisible();
        return start;
      }
      refused += 1;
      const body = (await answer.json().catch(() => null)) as { error?: { message?: string } } | null;
      lastRefusal = `${answer.status()} ${body?.error?.message ?? ""}`.trim();
    }
  }

  throw new Error(
    `bookFirstFreeHour: no free whole hour between ${localDateTimeValue(firstDay).slice(0, 10)} ` +
      `and ${localDateTimeValue(lastDay).slice(0, 10)} (${maxDays} days, hours ${firstWalkHour}:00–${lastWalkHour}:00) ` +
      `for "${patientName}": every hour tried was refused, the last one ${lastRefusal}.`,
  );
}

/**
 * Opens the issue popup of /facturacion (T76, S25): the issue form no longer lives between
 * the filters and the history, it opens with "Emitir comprobante".
 */
export async function openIssuePopup(page: Page): Promise<Locator> {
  await page.getByRole("button", { name: "Emitir comprobante" }).click();
  const dialog = page.getByRole("dialog", { name: "Emitir un comprobante" });
  await expect(dialog).toBeVisible();
  return dialog;
}

/**
 * Submits the issue popup and waits for the voucher detail popup the screen opens in its
 * place (T76). The inline "Emitimos el comprobante N° …" status no longer exists: the
 * detail itself is the confirmation, and it carries the number, the buyer and the amount,
 * so callers keep asserting what was issued instead of that a popup appeared.
 */
export async function submitIssuePopup(page: Page, dialog: Locator): Promise<Locator> {
  await dialog.getByRole("button", { name: "Emitir el comprobante" }).click();
  const detail = page.getByRole("dialog", { name: /Comprobante N° \d{6}/ });
  await expect(detail).toBeVisible({ timeout: 10_000 });
  return detail;
}

/**
 * Issues a loose voucher (no patient, no session) from the issue popup and returns the
 * detail popup the screen opens on success (T76).
 */
export async function issueLooseVoucher(page: Page, concept: string, amount: string): Promise<Locator> {
  const dialog = await openIssuePopup(page);
  await dialog.getByLabel("Concepto").fill(concept);
  await dialog.getByLabel("Importe (ARS)").fill(amount);
  return submitIssuePopup(page, dialog);
}

/**
 * Closes a popup with the header's own close button, so the page behind becomes clickable
 * again: a native `<dialog>` on top swallows clicks on what sits under it.
 */
export async function closeDialog(dialog: Locator): Promise<void> {
  await dialog.getByRole("button", { name: "Cerrar" }).click();
  await expect(dialog).toBeHidden();
}

/**
 * Reads the voucher detail popup the screen holds open (T76): clicking "Ver el comprobante"
 * opens it through the URL, and asserting inside it is how a test states what was issued.
 */
export function voucherDetail(page: Page): Locator {
  return page.getByRole("dialog", { name: /Comprobante N° \d{6}/ });
}

export async function openPatientRecord(page: Page, patientRow: Locator, patientName: string): Promise<void> {
  await patientRow.getByRole("button", { name: patientName }).click();
  await expect(page).toHaveURL(new RegExp(`/patients/[0-9a-f-]{36}`));
  await expect(page.getByRole("heading", { name: patientName })).toBeVisible();
}
