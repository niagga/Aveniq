"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { canManageAccounts, roleLabel, ROLES, type Invitation, type Role, type User } from "@/entities/iam/model";
import { InviteFormModal } from "@/features/users/components/invite-form";
import { PendingInvitations } from "@/features/users/components/pending-invitations";
import { UserEditModal } from "@/features/users/components/user-edit-modal";
import { UserTable } from "@/features/users/components/user-table";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Button } from "@/shared/ui/button";
import { FilterBar } from "@/shared/ui/filter-bar";
import { inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";
import { PageHeader } from "@/shared/ui/page-header";

/** The API's maximum page. The practice is small; a second page is not needed yet. */
const PAGE = { limit: 100, offset: 0 } as const;

type StatusFilter = "all" | "active" | "inactive";

function matchesQuery(text: string, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  return text.toLowerCase().includes(q);
}

/**
 * Accounts and invitations of the practice (T37, T70): search and filters, invite and edit in popups.
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

  const [query, setQuery] = useState("");
  const [roleFilter, setRoleFilter] = useState<"" | Role>("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");

  const [inviteOpen, setInviteOpen] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);

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

  const filteredUsers = useMemo(() => {
    if (users === null) return null;
    return users.filter((user) => {
      if (!matchesQuery(`${user.fullName} ${user.email}`, query)) return false;
      if (roleFilter !== "" && user.role !== roleFilter) return false;
      if (statusFilter === "active" && !user.isActive) return false;
      if (statusFilter === "inactive" && user.isActive) return false;
      return true;
    });
  }, [users, query, roleFilter, statusFilter]);

  const filteredInvitations = useMemo(() => {
    if (invitations === null) return null;
    if (statusFilter === "inactive") return [];
    return invitations.filter((invitation) => {
      if (!matchesQuery(invitation.email, query)) return false;
      if (roleFilter !== "" && invitation.role !== roleFilter) return false;
      return true;
    });
  }, [invitations, query, roleFilter, statusFilter]);

  return (
    <>
      <PageHeader
        title="Usuarios"
        description="Cuentas del consultorio, sus roles y el acceso de cada una."
      />

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
        <div className="space-y-6">
          <FilterBar
            label="Buscar y filtrar cuentas"
            actions={
              <Button size="sm" onClick={() => setInviteOpen(true)}>
                Invitar
              </Button>
            }
          >
            <label className="flex min-w-[12rem] flex-1 flex-col gap-0.5 text-xs font-medium text-ink-muted">
              Buscar
              <input
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Nombre o email"
                className={inputClass}
              />
            </label>

            <label className="flex flex-col gap-0.5 text-xs font-medium text-ink-muted">
              Rol
              <select
                value={roleFilter}
                onChange={(event) => setRoleFilter(event.target.value as "" | Role)}
                className="rounded-control border border-border-strong bg-surface px-2 py-1.5 text-sm text-ink"
              >
                <option value="">Todos</option>
                {ROLES.map((code) => (
                  <option key={code} value={code}>
                    {roleLabel(code)}
                  </option>
                ))}
              </select>
            </label>

            <label className="flex flex-col gap-0.5 text-xs font-medium text-ink-muted">
              Estado
              <select
                value={statusFilter}
                onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
                className="rounded-control border border-border-strong bg-surface px-2 py-1.5 text-sm text-ink"
              >
                <option value="all">Todas</option>
                <option value="active">Activas</option>
                <option value="inactive">Dadas de baja</option>
              </select>
            </label>
          </FilterBar>

          <section className="grid gap-3">
            <h2 className="text-sm font-semibold">Cuentas</h2>
            <UserTable
              users={filteredUsers ?? []}
              viewerID={viewerID}
              viewerRole={viewerRole}
              onEdit={(user) => setEditing(user)}
              onChanged={() => void load()}
            />
          </section>

          <InviteFormModal
            open={inviteOpen}
            viewerRole={viewerRole}
            practiceSlug={practiceSlug}
            onClose={() => setInviteOpen(false)}
            onIssued={() => void load()}
          />

          <UserEditModal
            open={editing !== null}
            user={editing}
            viewerID={viewerID}
            viewerRole={viewerRole}
            onClose={() => setEditing(null)}
            onSaved={() => void load()}
          />

          <section className="grid gap-3">
            <h2 className="text-sm font-semibold">Invitaciones pendientes</h2>
            {invitationsError !== null ? (
              <Notice tone="danger" title="Invitaciones no disponibles">
                {invitationsError}
              </Notice>
            ) : statusFilter === "inactive" ? (
              <Notice tone="info" title="Sin invitaciones en este filtro">
                El filtro &quot;Dadas de baja&quot; aplica solo a cuentas. Cambiá el estado para ver invitaciones
                pendientes.
              </Notice>
            ) : filteredInvitations !== null &&
              filteredInvitations.length === 0 &&
              (invitations?.length ?? 0) > 0 ? (
              <Notice tone="info" title="Ninguna invitación coincide con los filtros">
                Probá otra búsqueda o limpiá los filtros.
              </Notice>
            ) : (
              <PendingInvitations
                invitations={filteredInvitations ?? []}
                onChanged={() => void load()}
              />
            )}
          </section>
        </div>
      )}
    </>
  );
}