import { expect, test } from "@playwright/test";

import { ACCOUNTS, signInAs } from "./helpers";

/**
 * The narrow layout of the private screens (S3).
 *
 * Written because the responsive work was reviewed by reading CSS, and reading CSS cannot
 * tell you whether a seven-column table is still pushing the page sideways. These assertions
 * are the measurements that used to be an opinion: no horizontal overflow, and the rail
 * really turns into a bottom bar.
 */

const NARROW = { width: 390, height: 844 };

const PRIVATE_PATHS = [
  "/",
  "/patients",
  "/agenda",
  "/facturacion",
  "/informes",
  "/tarifario",
  "/mutuales",
  "/usuarios",
] as const;

/**
 * How much the application overflows the viewport, if at all.
 *
 * Measured on `body` and not on `documentElement` on purpose: in development Next mounts its
dev-tools portal outside the body, and that portal is wider than a phone viewport. Measuring the
root would make this test fail for a reason that does not exist in production. The app's own
content lives inside the body, which is what a user actually scrolls.
 */
async function horizontalOverflow(page: import("@playwright/test").Page): Promise<number> {
  return page.evaluate(() => document.body.scrollWidth - window.innerWidth);
}

/**
 * The widest elements inside the body, so a failure says what to fix and not only that
 * something is wrong.
 */
async function widestOffenders(page: import("@playwright/test").Page): Promise<string[]> {
  return page.evaluate(() => {
    const limit = window.innerWidth + 1;
    return Array.from(document.querySelectorAll("body *"))
      .map((element) => ({ element, box: element.getBoundingClientRect() }))
      .filter(({ box }) => box.width > limit)
      .slice(0, 4)
      .map(({ element, box }) => {
        const classes = typeof element.className === "string" ? element.className : "";
        return `${element.tagName.toLowerCase()}.${classes.split(" ").slice(0, 2).join(".")} = ${Math.round(
          box.width,
        )}px`;
      });
  });
}

test("no private screen scrolls sideways on a phone-width viewport", async ({ page }) => {
  await page.setViewportSize(NARROW);
  await signInAs(page, ACCOUNTS.owner);

  // Every screen is measured before asserting, so one failure lists them all instead of
  // hiding the next offender behind the first.
  const overflows: string[] = [];
  for (const path of PRIVATE_PATHS) {
    await page.goto(path);
    // Wait for the screen to settle: the shell renders immediately, the data does not.
    await page.waitForLoadState("networkidle");

    const overflow = await horizontalOverflow(page);
    if (overflow > 1) {
      const offenders = await widestOffenders(page);
      overflows.push(`${path}: +${overflow}px (${offenders.join(" | ")})`);
    }
  }

  expect(overflows, `pantallas que desbordan a ${NARROW.width}px`).toEqual([]);
});

test("the rail becomes a bottom bar and still marks the current section", async ({ page }) => {
  await page.setViewportSize(NARROW);
  await signInAs(page, ACCOUNTS.owner);
  await page.goto("/facturacion");

  const rail = page.getByRole("navigation", { name: "Secciones del consultorio" });
  await expect(rail).toBeVisible();

  const box = await rail.boundingBox();
  expect(box).not.toBeNull();
  // A bottom bar starts in the lower half of a phone viewport; a left rail starts at the top.
  expect(box!.y).toBeGreaterThan(NARROW.height / 2);

  await expect(page.getByRole("link", { name: "Facturación" })).toHaveAttribute(
    "aria-current",
    "page",
  );
});

test("a wide table scrolls inside its own panel instead of clipping", async ({ page }) => {
  await page.setViewportSize(NARROW);
  await signInAs(page, ACCOUNTS.owner);
  await page.goto("/patients");
  await page.waitForLoadState("networkidle");

  const panel = page.locator("table").first().locator("xpath=..");
  const canScroll = await panel.evaluate((element) => {
    const style = window.getComputedStyle(element);
    return style.overflowX === "auto" || style.overflowX === "scroll";
  });

  expect(canScroll).toBe(true);
});

/**
 * The measurement above is only worth something if it can fail.
 *
 * This took several wrong attempts to get right - the first version measured the root element
 * and reported the Next dev-tools portal as if it were the app - so the check now proves it
 * still notices a genuinely too-wide element.
 */
test("the overflow check itself notices a too-wide element", async ({ page }) => {
  await page.setViewportSize(NARROW);
  await page.goto("/login");

  expect(await horizontalOverflow(page)).toBeLessThanOrEqual(1);

  await page.evaluate(() => {
    const wide = document.createElement("div");
    wide.style.width = "900px";
    wide.style.height = "8px";
    document.body.appendChild(wide);
  });

  expect(await horizontalOverflow(page)).toBeGreaterThan(100);
});
