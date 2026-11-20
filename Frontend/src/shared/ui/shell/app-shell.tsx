"use client";

import Link from "next/link";
import type { Route } from "next";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { canReadBilling } from "@/entities/billing/model";
import {
  canManageAccounts,
  canSeePatients,
  canUseAgenda,
  roleLabel,
} from "@/entities/iam/model";
import { canManageInsurers } from "@/entities/insurer/model";
import { canReadTariffs } from "@/entities/tariff/model";
import { getSessionUser } from "@/shared/auth/session";
import { usePractice } from "@/shared/auth/practice";
import { TrialExpiredBanner } from "@/features/tenants/components/trial-expired-banner";
import { Notice } from "@/shared/ui/notice";
import { ThemeToggle } from "@/shared/ui/theme-toggle";

/**
 * The one shell of the private screens (S1).
 *
 * Before this, every screen repeated its own SessionGate, its own container width and
 * a back-to-the-dashboard link, so navigating between two sections cost two trips
 * through the dashboard. The rail replaces all three.
 *
 * Sections are gated by the same helpers the screens use, so the rail never offers a
 * section the API will refuse. The helpers are UX only: the API stays the authority.
 */

type Section = {
  href: Route;
  label: string;
  icon: ReactNode;
  allow: (role: string | undefined) => boolean;
};

const SECTIONS: readonly Section[] = [
  {
    href: "/",
    label: "Inicio",
    icon: <HomeIcon />,
    allow: () => true,
  },
  {
    href: "/agenda",
    label: "Agenda",
    icon: <CalendarIcon />,
    allow: canUseAgenda,
  },
  {
    href: "/patients",
    label: "Pacientes",
    icon: <PatientsIcon />,
    allow: canSeePatients,
  },
  {
    href: "/facturacion",
    label: "Facturación",
    icon: <BillingIcon />,
    allow: canReadBilling,
  },
  {
    href: "/tarifario",
    label: "Tarifario",
    icon: <TariffIcon />,
    allow: canReadTariffs,
  },
  {
    href: "/mutuales",
    label: "Mutuales",
    icon: <InsurersIcon />,
    allow: canManageInsurers,
  },
  {
    href: "/usuarios",
    label: "Usuarios",
    icon: <UsersIcon />,
    allow: canManageAccounts,
  },
];

/** How many destinations the bottom bar keeps on a narrow screen before folding the rest. */
const NARROW_VISIBLE = 4;

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const user = getSessionUser();
  const visible = SECTIONS.filter((section) => section.allow(user?.role));
  const { practice, expired } = usePractice();

  // The trial belongs to the practice, so the shell states it once instead of every screen
  // guessing, and it no longer renders above the rail: it renders inside the content column.
  const trialDaysLeft =
    practice !== null && practice.status === "trial" && !expired
      ? Math.max(
          0,
          Math.ceil((new Date(practice.trialEndsAt).getTime() - Date.now()) / 86_400_000),
        )
      : null;
  const trialLabel =
    expired
      ? "La prueba del consultorio venció"
      : trialDaysLeft === null
        ? null
        : `La prueba vence en ${trialDaysLeft} día${trialDaysLeft === 1 ? "" : "s"}`;

  return (
    <div className="flex min-h-full flex-col md:flex-row">
      <nav
        aria-label="Secciones del consultorio"
        className="order-2 flex shrink-0 items-stretch gap-1 border-t border-border bg-surface px-1 py-1 md:order-1 md:w-16 md:flex-col md:items-center md:border-r md:border-t-0 md:px-0 md:py-3"
      >
        <Link
          href="/"
          aria-label="Aveniq"
          className="hidden rounded-control bg-brand px-2 py-1 text-xs font-bold text-brand-ink md:mb-2 md:grid md:h-8 md:w-8 md:place-items-center md:px-0 md:py-0"
        >
          A
        </Link>

        <ul className="flex flex-1 items-stretch gap-1 md:flex-none md:flex-col md:items-center">
          {visible.map((section, index) => {
            const active = pathname === section.href;

            return (
              <li
                key={section.href}
                className={`relative flex-1 md:flex-none ${
                  index >= NARROW_VISIBLE ? "hidden md:flex" : "flex"
                }`}
              >
                <Link
                  href={section.href}
                  title={section.label}
                  aria-current={active ? "page" : undefined}
                  className={`relative flex w-full flex-col items-center gap-0.5 rounded-control px-2 py-1.5 text-[10px] font-medium md:w-11 md:gap-0 md:px-0 md:py-2 ${
                    active
                      ? "bg-brand/10 text-brand"
                      : "text-ink-muted hover:bg-surface-muted hover:text-ink"
                  }`}
                >
                  {active ? (
                    <span
                      aria-hidden="true"
                      className="absolute inset-x-2 top-0 h-0.5 rounded bg-brand md:inset-x-auto md:inset-y-2 md:left-0 md:h-auto md:w-0.5"
                    />
                  ) : null}
                  {section.icon}
                  <span className="md:sr-only">{section.label}</span>
                </Link>
              </li>
            );
          })}
        </ul>

        <div className="ml-1 flex items-center gap-1 md:ml-0 md:mt-auto md:flex-col md:gap-2">
          {trialLabel === null ? null : (
            <span
              title={trialLabel}
              aria-label={trialLabel}
              className={`grid h-8 w-8 place-items-center rounded-control border ${
                expired
                  ? "border-danger/40 bg-danger/5 text-danger"
                  : "border-warning/30 bg-warning/5 text-warning"
              }`}
            >
              <ClockIcon />
            </span>
          )}
          <ThemeToggle />
          <span
            title={user ? roleLabel(user.role) : "Sin sesión"}
            aria-label={`Sesión: ${user ? roleLabel(user.role) : "sin sesión"}`}
            className="hidden h-8 w-8 place-items-center rounded-full border border-border bg-surface-muted text-ink-muted md:grid"
          >
            <PersonIcon />
          </span>
        </div>
      </nav>

      <div className="order-1 flex min-w-0 flex-1 flex-col md:order-2">
        {expired && practice !== null ? (
          <div className="px-6 pt-4 max-md:px-4">
            <TrialExpiredBanner trialEndsAt={practice.trialEndsAt} />
          </div>
        ) : trialDaysLeft !== null && trialDaysLeft <= 7 ? (
          <div className="px-6 pt-4 max-md:px-4">
            <Notice tone="warning" title={trialLabel ?? ""}>
              Pasada esa fecha el consultorio queda en sólo lectura.
            </Notice>
          </div>
        ) : null}
        {children}
      </div>
    </div>
  );
}

/**
 * The body of a private screen: one container width for every section, plus the slot
 * the contextual panel takes in T4. Pages render their content inside this, so no
 * screen declares its own `max-w-*` again.
 *
 * `max-w-6xl` is the width five of the seven screens already used. The agenda's week grid
 * gets its extra room back together with its own panel, in T6.
 */
export function ScreenFrame({
  panel,
  children,
}: {
  panel?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-1">
      {panel}
      <main className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-6 py-8 max-md:pb-20">
        {children}
      </main>
    </div>
  );
}

function HomeIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M2 7.5 8 2.5l6 5V13a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V7.5Z" />
    </svg>
  );
}

function CalendarIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <rect x="2" y="3" width="12" height="11" rx="1" />
      <path d="M2 6.5h12M5.5 1.5v3M10.5 1.5v3" />
    </svg>
  );
}

function PatientsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <circle cx="8" cy="5.5" r="2.8" />
      <path d="M2.5 14c.8-2.6 2.9-4 5.5-4s4.7 1.4 5.5 4" />
    </svg>
  );
}

function BillingIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <rect x="2" y="3.5" width="12" height="9" rx="1" />
      <path d="M2 6.5h12M5 10h3" />
    </svg>
  );
}

function TariffIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M2.5 4h11M2.5 8h11M2.5 12h6" />
      <circle cx="12.5" cy="12" r="1.6" />
    </svg>
  );
}

function InsurersIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M8 2.5 13.5 5v3.2c0 2.6-2.2 4.6-5.5 5.3-3.3-.7-5.5-2.7-5.5-5.3V5L8 2.5Z" />
    </svg>
  );
}

function UsersIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <circle cx="6" cy="5.5" r="2.2" />
      <path d="M1.8 13.5c.6-2.1 2.2-3.3 4.2-3.3s3.6 1.2 4.2 3.3" />
      <path d="M11 5.5h3M12.5 4v3" />
    </svg>
  );
}

function ClockIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <circle cx="8" cy="8" r="5.6" />
      <path d="M8 4.8V8l2.2 1.4" />
    </svg>
  );
}

function PersonIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <circle cx="8" cy="5.5" r="2.6" />
      <path d="M3 14c.7-2.4 2.6-3.7 5-3.7s4.3 1.3 5 3.7" />
    </svg>
  );
}
