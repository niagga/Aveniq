import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

import { DEMO, signIn } from "./helpers";

/**
 * The popup base of E8 (T62). The confirmation dialog now sits on the shared `Modal`, so the
 * journeys here prove what every popup inherits: Escape and the backdrop close without acting,
 * the page behind is blurred, focus goes back to the control that opened the popup, and below
 * `md` the popup is a full-screen sheet. The unsaved-changes guard of `FormModal` gets its first
 * consumer in T68 and is checked there.
 */

const stamp = Date.now().toString(36);

/** A pending invitation of the demo practice, so the users screen has a confirmation to open. */
async function pendingInvitation(request: APIRequestContext, email: string): Promise<void> {
  const login = await request.post("/api/v1/auth/login", {
    data: { tenantSlug: DEMO.tenant, email: DEMO.email, password: DEMO.password },
  });
  expect(login.ok()).toBeTruthy();
  const { accessToken } = (await login.json()) as { accessToken: string };
  const issued = await request.post("/api/v1/invitations", {
    headers: { Authorization: `Bearer ${accessToken}` },
    data: { email, role: "assistant" },
  });
  expect(issued.status()).toBe(201);
}

/** Opens the retire-invitation confirmation for one invitation and returns its row. */
async function openRetire(page: Page, email: string) {
  await page.goto("/usuarios");
  const row = page.getByRole("row").filter({ hasText: email });
  await expect(row).toBeVisible({ timeout: 15_000 });
  const retire = row.getByRole("button", { name: "Retirar invitación" });
  await retire.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText(`¿Retirar la invitación de ${email}?`);
  return { row, retire, dialog };
}

test("Escape and the backdrop close the popup without acting, over a blurred page", async ({ browser, request }) => {
  const email = `e8-base-${stamp}@aveniq.local`;
  await pendingInvitation(request, email);

  const context = await browser.newContext();
  const page = await context.newPage();
  let retirements = 0;
  page.on("request", (req) => {
    if (req.method() === "DELETE" && req.url().includes("/api/v1/invitations/")) retirements += 1;
  });
  try {
    await signIn(page);
    const { row, retire, dialog } = await openRetire(page, email);

    // The page behind is blurred by the platform backdrop, not by a styled overlay.
    const blur = await page.evaluate(
      () => getComputedStyle(document.querySelector("dialog[open]")!, "::backdrop").backdropFilter,
    );
    expect(blur).toContain("blur");

    // Escape closes it, and focus goes back to the button that opened it.
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(retire).toBeFocused();

    // A click on the backdrop closes it the same way.
    await retire.click();
    await expect(dialog).toBeVisible();
    await page.mouse.click(4, 4);
    await expect(dialog).toBeHidden();

    // Closing never retires the invitation: it is still listed, and no request left the page.
    await expect(row).toBeVisible();
    expect(retirements).toBe(0);
  } finally {
    await context.close();
  }
});

test("on a phone the popup is a full-screen sheet with its close button in reach", async ({ browser, request }) => {
  const email = `e8-sheet-${stamp}@aveniq.local`;
  await pendingInvitation(request, email);

  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  try {
    await signIn(page);
    const { dialog } = await openRetire(page, email);

    const box = await dialog.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(388);
    expect(box!.height).toBeGreaterThanOrEqual(840);

    const close = dialog.getByRole("button", { name: "Cerrar" });
    await expect(close).toBeInViewport();
  } finally {
    await context.close();
  }
});
