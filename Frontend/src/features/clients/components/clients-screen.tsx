"use client";

import { useMemo, useState } from "react";

import type { Client } from "@/entities/client/model";
import { canManageClients, fiscalDocumentText } from "@/entities/client/model";
import { currencyLabel, fiscalConditionLabel } from "@/entities/patient/model";
import { ClientFormModal } from "@/features/clients/components/client-form";
import { useClients } from "@/features/clients/use-clients";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { useWritesLocked } from "@/shared/auth/practice";
import { Button } from "@/shared/ui/button";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { FilterBar } from "@/shared/ui/filter-bar";
import { inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";
import { PageHeader } from "@/shared/ui/page-header";
import { Segmented } from "@/shared/ui/segmented";

/**
 * The practice's billing clients (T74, S26): who pays, which is not always who is treated.
 *
 * The fiscal data of a receipt is chosen here and frozen on the voucher, so the list shows the
 * document and the condition as they are typed and not as they would be derived from the
 * patient. Filtering happens over the loaded page: the API has no search field for clients yet.
 */
export function ClientsScreen() {
  const user = getSessionUser();
  const canWrite = canManageClients(user?.role);
  const writesLocked = useWritesLocked();
  const { clients, failedStatus, reload } = useClients(true);

  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "inactive">("all");
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Client | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyID, setBusyID] = useState<string | null>(null);
  const [confirmingDowngrade, setConfirmingDowngrade] = useState<Client | null>(null);

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (clients ?? []).filter((client) => {
      if (status === "active" && !client.isActive) return false;
      if (status === "inactive" && client.isActive) return false;
      if (needle === "") return true;
      return [client.displayName, client.legalName, client.taxIdNumber]
        .filter((field): field is string => Boolean(field))
        .some((field) => field.toLowerCase().includes(needle));
    });
  }, [clients, query, status]);

  function openCreate() {
    setEditing(null);
    setFormOpen(true);
  }

  function openEdit(client: Client) {
    setEditing(client);
    setFormOpen(true);
  }

  async function toggleActive(client: Client) {
    setActionError(null);
    setBusyID(client.id);
    try {
      const result = client.isActive
        ? await browserApi.POST("/api/v1/clients/{id}/deactivate", {
            params: { path: { id: client.id } },
          })
        : await browserApi.POST("/api/v1/clients/{id}/activate", {
            params: { path: { id: client.id } },
          });

      if (result.error || !result.data) {
        setActionError(apiErrorMessage(result.error));
        return;
      }
      void reload();
    } catch {
      setActionError("No pudimos contactar al servidor.");
    } finally {
      setBusyID(null);
    }
  }

  return (
    <>
      <PageHeader
        title="Clientes"
        description="Quién paga las sesiones. Cada paciente tiene un cliente: por defecto, él mismo; puede ser un tercero."
        action={
          canWrite ? (
            <Button size="sm" onClick={openCreate} disabled={writesLocked}>
              Nuevo cliente
            </Button>
          ) : null
        }
      />

      {canWrite ? (
        <ClientFormModal
          open={formOpen}
          client={editing}
          onClose={() => setFormOpen(false)}
          onSaved={() => void reload()}
        />
      ) : null}

      {failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer los clientes">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : clients === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando clientes…
        </p>
      ) : (
        <>
          <FilterBar label="Filtros de clientes">
            <div>
              <label htmlFor="clients-query" className="block text-xs font-medium text-ink-muted">
                Buscar
              </label>
              <input
                id="clients-query"
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Nombre o documento fiscal"
                className={inputClass}
              />
            </div>
            <div>
              <span className="block text-xs font-medium text-ink-muted">Estado</span>
              <Segmented
                label="Estado del cliente"
                value={status}
                onChange={setStatus}
                options={[
                  { value: "all", label: "Todos" },
                  { value: "active", label: "Activos" },
                  { value: "inactive", label: "Dados de baja" },
                ]}
              />
            </div>
          </FilterBar>

          {visible.length === 0 ? (
            <Notice tone="info" title={clients.length === 0 ? "Todavía no hay clientes" : "Ningún cliente coincide"}>
              {clients.length === 0
                ? "Cargá el primero con el botón de arriba. Cada paciente nuevo crea el suyo si no elegís otro."
                : "Probá con otro nombre, otro documento o cambiá el filtro de estado."}
            </Notice>
          ) : (
            <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
              {actionError ? (
                <p role="alert" className="border-b border-border px-4 py-2 text-sm text-danger">
                  {actionError}
                </p>
              ) : null}
              <table className="w-full text-left text-sm">
                <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
                  <tr>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Cliente
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium max-md:hidden">
                      Documento fiscal
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium max-md:hidden">
                      Condición fiscal
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Moneda
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Estado
                    </th>
                    {canWrite ? (
                      <th scope="col" className="px-4 py-3 font-medium">
                        <span className="sr-only">Acciones</span>
                      </th>
                    ) : null}
                  </tr>
                </thead>
                <tbody>
                  {visible.map((client) => (
                    <tr key={client.id} className="border-t border-border">
                      <td className="px-4 py-3">
                        <span className="font-medium">{client.displayName}</span>
                        {client.legalName ? (
                          <span className="block text-xs text-ink-muted">{client.legalName}</span>
                        ) : null}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-ink-muted max-md:hidden">
                        {fiscalDocumentText(client)}
                      </td>
                      <td className="px-4 py-3 max-md:hidden">
                        {fiscalConditionLabel(client.fiscalCondition) || "—"}
                      </td>
                      <td className="px-4 py-3">{currencyLabel(client.billingCurrency) || "—"}</td>
                      <td className="px-4 py-3">
                        <span
                          className={`rounded-full border px-2 py-0.5 text-xs ${
                            client.isActive
                              ? "border-success/30 bg-success/5 text-success"
                              : "border-border bg-surface-muted text-ink-muted"
                          }`}
                        >
                          {client.isActive ? "Activo" : "Dado de baja"}
                        </span>
                      </td>
                      {canWrite ? (
                        <td className="px-4 py-3 text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => openEdit(client)}
                              disabled={writesLocked}
                            >
                              Editar
                            </Button>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => {
                                if (client.isActive) setConfirmingDowngrade(client);
                                else void toggleActive(client);
                              }}
                              disabled={busyID === client.id || writesLocked}
                            >
                              {busyID === client.id
                                ? "Guardando…"
                                : client.isActive
                                  ? "Dar de baja"
                                  : "Reactivar"}
                            </Button>
                            <ConfirmDialog
                              open={confirmingDowngrade?.id === client.id}
                              title={`¿Dar de baja ${client.displayName}?`}
                              description="Deja de ofrecerse al crear un paciente. Los comprobantes ya emitidos conservan sus datos fiscales y se puede reactivar cuando haga falta."
                              confirmLabel="Dar de baja"
                              tone="danger"
                              busy={busyID === client.id}
                              onCancel={() => setConfirmingDowngrade(null)}
                              onConfirm={() => {
                                const target = confirmingDowngrade;
                                setConfirmingDowngrade(null);
                                if (target !== null) void toggleActive(target);
                              }}
                            />
                          </div>
                        </td>
                      ) : null}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </>
  );
}
