"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { canManageAccounts, type Invitation, type User } from "@/entities/iam/model";
import { InviteForm } from "@/features/users/components/invite-form";
import { PendingInvitations } from "@/features/users/components/pending-invitations";
import { UserTable } from "@/features/users/components/user-table";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";

/** The API's maximum page. The practice is small; a second page is not needed yet. */
const PAGE = { limit: 100, offset: 0 } as const;

/**
 * Accounts and invitations of the practice (T37).
 *
 * Owner and admin manage accounts. Every other role gets a notice instead of a list:
 * the screen hides what the matrix forbids, and the API refuses it again if asked.
 */
export function UsersScreen() {
  const viewer = getSessionUser();
  const viewerID = viewer?.id ?? "";
  const viewerRole = viewer?.role ?? "";
  const viewerTenantID = viewer?.tenantId ?? null;
  const allowed = canManageAccounts(viewerRole);

  const [users, setUsers] = useState<User[] | null>(null);
  const [invitations, setInvitations] = useState<Invitation[] | null>(null);
  const [invitationsError, setInvitationsError] = useState<string | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [practiceSlug, setPracticeSlug] = useState<string | null>(null);

  const load = useCallback(async () => {
    const [accounts, pending] = await Promise.all([
      browserApi.GET("/api/v1/users", { params: { query: PAGE } }),
      browserApi.GET("/api/v1/invitations", { params: { query: PAGE } }),
    ]);

    if (!accounts.response.ok) {
      setFailedStatus(accounts.response.status);
      return;
    }
    setFailedStatus(null);
    setUsers(accounts.data?.items ?? []);

    if (pending.response.ok) {
      setInvitationsError(null);
      setInvitations(pending.data?.items ?? []);
    } else {
      setInvitationsError(`No pudimos leer las invitaciones (${pending.response.status}).`);
    }
  }, []);

  useEffect(() => {
    if (!allowed) return;
    let active = true;
    void (async () => {
      if (active) await load();
    })();
    return () => {
      active = false;
    };
  }, [allowed, load]);

  // The practice slug is not in the session, only its id. It is needed for the invitation
  // link, so the invitee can be told which practice to sign in to.
  useEffect(() => {
    if (!allowed || viewerTenantID === null) return;
    let active = true;
    void (async () => {
      const { data } = await browserApi.GET("/api/v1/tenants");
      if (!active) return;
      setPracticeSlug(data?.items?.find((tenant) => tenant.id === viewerTenantID)?.slug ?? null);
    })();
    return () => {
      active = false;
    };
  }, [allowed, viewerTenantID]);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Link href="/" className="text-xs text-ink-muted underline">
            ← Volver al consultorio
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Usuarios</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Cuentas del consultorio, sus roles y el acceso de cada una.
          </p>
        </div>
      </header>

      {!allowed ? (
        <Notice tone="warning" title="Esta pantalla es para el titular y la administración">
          Tu rol no administra cuentas. Pedile al titular del consultorio si necesitás cambiar algo.
        </Notice>
      ) : failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer las cuentas">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : users === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando usuarios…
        </p>
      ) : (
        <>
          <section className="grid gap-3">
            <h2 className="text-sm font-semibold">Cuentas</h2>
            <UserTable
              users={users}
              viewerID={viewerID}
              viewerRole={viewerRole}
              onChanged={() => void load()}
            />
          </section>

          <InviteForm
            viewerRole={viewerRole}
            practiceSlug={practiceSlug}
            onIssued={() => void load()}
          />

          <section className="grid gap-3">
            <h2 className="text-sm font-semibold">Invitaciones pendientes</h2>
            {invitationsError !== null ? (
              <Notice tone="danger" title="Invitaciones no disponibles">
                {invitationsError}
              </Notice>
            ) : (
              <PendingInvitations
                invitations={invitations ?? []}
                onChanged={() => void load()}
              />
            )}
          </section>
        </>
      )}
    </>
  );
}
