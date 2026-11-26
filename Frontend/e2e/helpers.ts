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
