"use client";

import Link from "next/link";
import type { Route } from "next";
import { usePathname } from "next/navigation";
import { useState, type ReactNode } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { canReadClients } from "@/entities/client/model";
import {
  canManageAccounts,
  canSeePatients,
  canUseAgenda,
  roleLabel,
} from "@/entities/iam/model";
import { canManageLanding } from "@/entities/landing/model";
import { canManageInsurers } from "@/entities/insurer/model";
import { canReadReports } from "@/entities/report/model";
import { canReadTariffs } from "@/entities/tariff/model";
import { getSessionUser } from "@/shared/auth/session";
import { usePractice } from "@/shared/auth/practice";
import { canCountUnbilled, unbilledLabel, useUnbilledCount } from "@/features/billing/use-unbilled";
import { TrialExpiredBanner } from "@/features/tenants/components/trial-expired-banner";
import { Modal } from "@/shared/ui/modal";
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
    href: "/panel",
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
    href: "/clientes",
    label: "Clientes",
    icon: <ClientsIcon />,
    allow: canReadClients,
  },
  {
    href: "/facturacion",
    label: "Facturación",
    icon: <BillingIcon />,
    allow: canReadBilling,
  },
  {
    href: "/liquidaciones",
    label: "Liquidaciones",
    icon: <SettlementsIcon />,
    allow: canReadBilling,
  },
  {
    href: "/informes",
    label: "Informes",
    icon: <ReportsIcon />,
    allow: canReadReports,
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
  {
    href: "/mi-consultorio",
    label: "Mi consultorio",
    icon: <PracticeIcon />,
    allow: canManageLanding,
  },
];

/**
 * How many destinations the bottom bar keeps on a narrow screen. The rest sit behind "Más", a
 * popup with the same links: before T63 they were simply hidden and a phone could not reach them.
 */
const NARROW_VISIBLE = 4;

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const user = getSessionUser();
  const visible = SECTIONS.filter((section) => section.allow(user?.role));
  const folded = visible.slice(NARROW_VISIBLE);
  const foldedActive = folded.some((section) => section.href === pathname);
  const [moreOpen, setMoreOpen] = useState(false);
  const { status: trial, expired } = usePractice();
  const unbilled = unbilledLabel(useUnbilledCount(canCountUnbilled(user?.role)));

  // The trial belongs to the practice, so the shell states it once instead of every screen
  // guessing, and it no longer renders above the rail: it renders inside the content column.
  const trialDaysLeft =
    trial !== null && trial.status === "trial" && !expired
      ? Math.max(
          0,
          Math.ceil((new Date(trial.trialEndsAt).getTime() - Date.now()) / 86_400_000),
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
        className="order-2 flex shrink-0 items-stretch gap-1 border-t border-border bg-surface px-1 py-1 max-md:fixed max-md:inset-x-0 max-md:bottom-0 max-md:z-30 md:order-1 md:w-16 md:flex-col md:items-center md:border-r md:border-t-0 md:px-0 md:py-3 lg:w-56 lg:items-stretch lg:px-3"
      >
        <Link
          href="/panel"
          aria-label="Aveniq"
          className="hidden rounded-control bg-brand px-2 py-1 text-xs font-bold text-brand-ink md:mb-2 md:grid md:h-8 md:w-8 md:place-items-center md:px-0 md:py-0 lg:flex lg:h-auto lg:w-full lg:justify-start lg:px-3 lg:py-2 lg:text-sm"
        >
          <span className="lg:hidden">A</span>
          <span className="hidden lg:inline">Aveniq</span>
        </Link>

        <ul className="flex flex-1 items-stretch gap-1 md:flex-none md:flex-col md:items-center lg:items-stretch lg:gap-0.5">
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
                  aria-label={
                    section.href === "/facturacion" && unbilled !== ""
                      ? `${section.label}, ${unbilled} sesiones sin facturar`
                      : undefined
                  }
                  className={`relative flex w-full flex-col items-center gap-0.5 rounded-control px-2 py-1.5 text-[10px] font-medium md:w-11 md:gap-0 md:px-0 md:py-2 lg:w-full lg:flex-row lg:justify-start lg:gap-3 lg:px-3 lg:py-2 lg:text-sm ${
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
                  <span className="md:sr-only lg:not-sr-only">{section.label}</span>
                  {section.href === "/facturacion" && unbilled !== "" ? (
                    <span
                      aria-hidden="true"
                      className="absolute right-1 top-0.5 rounded-full bg-brand px-1 text-[9px] font-semibold leading-4 text-brand-ink md:right-0.5 lg:right-2 lg:top-1/2 lg:-translate-y-1/2 lg:text-[11px]"
                    >
                      {unbilled}
                    </span>
                  ) : null}
                </Link>
              </li>
            );
          })}
          {folded.length > 0 ? (
            <li className="relative flex flex-1 md:hidden">
              <button
                type="button"
                aria-haspopup="dialog"
                aria-expanded={moreOpen}
                onClick={() => setMoreOpen(true)}
                className={`relative flex w-full flex-col items-center gap-0.5 rounded-control px-2 py-1.5 text-[10px] font-medium ${
                  foldedActive
                    ? "bg-brand/10 text-brand"
                    : "text-ink-muted hover:bg-surface-muted hover:text-ink"
                }`}
              >
                {foldedActive ? (
                  <span
                    aria-hidden="true"
                    className="absolute inset-x-2 top-0 h-0.5 rounded bg-brand"
                  />
                ) : null}
                <MoreIcon />
                <span>Más</span>
              </button>
            </li>
          ) : null}
        </ul>

        <div className="ml-1 flex items-center gap-1 md:ml-0 md:mt-auto md:flex-col md:gap-2 lg:ml-0 lg:mt-auto lg:w-full lg:flex-row lg:justify-between lg:px-1">
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

      {/* The sections that do not fit the bottom bar on a phone. Only a phone opens it: from md
          the rail shows every section, and the button is hidden. */}
      <Modal
        open={moreOpen}
        title="Más secciones"
        size="sm"
        onClose={() => setMoreOpen(false)}
      >
        <ul className="grid gap-1">
          {folded.map((section) => {
            const active = pathname === section.href;

            return (
              <li key={section.href}>
                <Link
                  href={section.href}
                  aria-current={active ? "page" : undefined}
                  onClick={() => setMoreOpen(false)}
                  className={`flex items-center gap-3 rounded-control px-3 py-2.5 text-sm font-medium ${
                    active ? "bg-brand/10 text-brand" : "text-ink hover:bg-surface-muted"
                  }`}
                >
                  {section.icon}
                  {section.label}
                </Link>
              </li>
            );
          })}
        </ul>
      </Modal>

      <div className="order-1 flex min-w-0 flex-1 flex-col md:order-2">
        {expired && trial !== null ? (
          <div className="px-6 pt-4 max-md:px-4">
            <TrialExpiredBanner trialEndsAt={trial.trialEndsAt} />
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

function SettlementsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M3 2.5h10v11l-2.5-1.6L8 13.5l-2.5-1.6L3 13.5v-11Z" />
      <path d="M5.5 6h5M5.5 8.5h5" />
    </svg>
  );
}

function ReportsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M2.5 13.5h11" />
      <path d="M4.5 11V7.5M8 11V3.5M11.5 11V6" />
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

function ClientsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <rect x="1.8" y="3.5" width="12.4" height="9" rx="1.4" />
      <circle cx="5.6" cy="7.2" r="1.4" />
      <path d="M3.4 11.2c.4-1.2 1.2-1.9 2.2-1.9s1.8.7 2.2 1.9" />
      <path d="M10.2 6.6h2.4M10.2 9.2h2.4" />
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

function PracticeIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
      <path d="M3 14V4.5l5-2.5 5 2.5V14H3Z" />
      <path d="M6.5 14v-4h3v4M6.5 7.5h3" />
    </svg>
  );
}

function MoreIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <circle cx="3.5" cy="8" r="1.4" />
      <circle cx="8" cy="8" r="1.4" />
      <circle cx="12.5" cy="8" r="1.4" />
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
