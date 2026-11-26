"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import {
  coverageText,
  currencyLabel,
  documentKindLabel,
  fiscalConditionLabel,
  formatBirthDate,
  type Patient,
} from "@/entities/patient/model";
import { fiscalDocumentText } from "@/entities/client/model";
import {
  canAssignTreating,
  canManageGrants,
  canOpenClinicalRecord,
  canWriteClinicalRecord,
} from "@/entities/care/model";
import { ClinicalPanel } from "@/features/patients/components/clinical-panel";
import { useClient } from "@/features/clients/use-clients";
import { GrantsPanel } from "@/features/patients/components/grants-panel";
import { TreatingProfessional } from "@/features/patients/components/treating-professional";
import { useStaff } from "@/features/patients/staff";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";

/**
 * The read view of one patient.
 *
 * Deactivation lives here and not in the table: it is a deliberate act on a person's
 * record, and a button per row is a button somebody clicks by accident while
 * scrolling. There is no delete, and nothing in this screen suggests one.
 */
export function PatientDetail({
  patient,
  insurerNames,
  onClose,
  onChanged,
  onEdit,
}: {
  patient: Patient;
  insurerNames: ReadonlyMap<string, string>;
  onClose: () => void;
  onChanged: () => void;
  onEdit: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setIsBusy] = useState(false);
  const [tab, setTab] = useState<"admin" | "clinical">("admin");
  // Deactivating is the destructive half: it asks first. Reactivating does not.
  const [confirmingDeactivate, setConfirmingDeactivate] = useState(false);

  const tabID = (name: "admin" | "clinical") => `${patient.id}-tab-${name}`;
  const panelID = (name: "admin" | "clinical") => `${patient.id}-panel-${name}`;

  // The viewer decides what this screen offers; the API decides what is allowed.
  const viewer = getSessionUser();
  const role = viewer?.role;
  const userId = viewer?.id ?? "";
  const staff = useStaff(role);
  const writesLocked = useWritesLocked();

  useEffect(() => {
    setTab("admin");
  }, [patient.id]);

  async function toggleActive() {
    setError(null);
    setIsBusy(true);
    try {
      const result = patient.isActive
        ? await browserApi.POST("/api/v1/patients/{id}/deactivate", {
            params: { path: { id: patient.id } },
          })
        : await browserApi.POST("/api/v1/patients/{id}/activate", {
            params: { path: { id: patient.id } },
          });

      if (result.error || !result.data) {
        setError(apiErrorMessage(result.error));
        return;
      }
      onChanged();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setIsBusy(false);
    }
  }

  return (
    <section className="rounded-[var(--radius-panel)] border border-border bg-surface p-5">
      <ConfirmDialog
        open={confirmingDeactivate}
        title={`¿Dar de baja a ${patient.fullName}?`}
        description="El paciente deja de aparecer en la agenda y no se le pueden emitir comprobantes nuevos. La ficha y su historia clínica se conservan: se puede reactivar en cualquier momento."
        confirmLabel="Dar de baja"
        tone="danger"
        busy={isBusy}
        onCancel={() => setConfirmingDeactivate(false)}
        onConfirm={() => {
          setConfirmingDeactivate(false);
          void toggleActive();
        }}
      />
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{patient.fullName}</h2>
          <p className="mt-1 text-xs text-ink-muted">
            {patient.isActive ? "Activo" : "Inactivo"} · {patient.country}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onEdit}
            disabled={writesLocked}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium disabled:opacity-60"
          >
            Editar
          </button>
          <button
            type="button"
            onClick={() => {
              if (patient.isActive) setConfirmingDeactivate(true);
              else void toggleActive();
            }}
            disabled={isBusy || writesLocked}
            className="rounded-control border border-border px-3 py-1.5 text-xs font-medium text-ink-muted disabled:opacity-60"
          >
            {isBusy ? "Guardando…" : patient.isActive ? "Dar de baja" : "Reactivar"}
          </button>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md px-2 py-1.5 text-xs text-ink-muted"
            aria-label="Cerrar detalle"
          >
            ✕
          </button>
        </div>
      </header>

      {error ? (
        <p role="alert" className="mt-3 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <div
        className="mt-4 flex gap-1 border-b border-border"
        role="tablist"
        aria-label="Ficha del paciente"
        onKeyDown={(event) => {
          // The arrow keys belong to the tabs: declaring role=tab without them leaves a
          // keyboard user tabbing through controls that announce a pattern they cannot use.
          if (event.key !== "ArrowRight" && event.key !== "ArrowLeft") return;
          if (!canOpenClinicalRecord(role)) return;
          event.preventDefault();
          const next = tab === "admin" ? "clinical" : "admin";
          setTab(next);
          document.getElementById(tabID(next))?.focus();
        }}
      >
        <TabButton
          id={tabID("admin")}
          controls={panelID("admin")}
          active={tab === "admin"}
          onClick={() => setTab("admin")}
        >
          Datos
        </TabButton>
        {canOpenClinicalRecord(role) ? (
          <TabButton
            id={tabID("clinical")}
            controls={panelID("clinical")}
            active={tab === "clinical"}
            onClick={() => setTab("clinical")}
          >
            Historia clínica
          </TabButton>
        ) : null}
      </div>

      {tab === "clinical" && canOpenClinicalRecord(role) ? (
        <div
          role="tabpanel"
          id={panelID("clinical")}
          aria-labelledby={tabID("clinical")}
          tabIndex={0}
        >
          <ClinicalPanel
            key={patient.id}
            patientID={patient.id}
            readOnly={writesLocked || !canWriteClinicalRecord(role, patient, userId)}
          />
        </div>
      ) : null}

      {tab === "admin" ? (
      <div role="tabpanel" id={panelID("admin")} aria-labelledby={tabID("admin")} tabIndex={0}>
      <dl className="mt-4 grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
        <Row label="Documento">
          {patient.document
            ? `${documentKindLabel(patient.document.kind)} ${patient.document.number}`
            : "Sin documento"}
        </Row>
        <Row label="Fecha de nacimiento">{formatBirthDate(patient.birthDate) || "—"}</Row>
        <Row label="Teléfono">{patient.phone ?? "—"}</Row>
        <Row label="Email">{patient.email ?? "—"}</Row>
        <Row label="Contacto de emergencia">
          {patient.emergencyContactName
            ? `${patient.emergencyContactName}${patient.emergencyContactPhone ? ` · ${patient.emergencyContactPhone}` : ""}`
            : "—"}
        </Row>
        <Row label="Cobertura" wide>
          {coverageText(
            patient,
            patient.insurerId ? insurerNames.get(patient.insurerId) : undefined,
          )}
        </Row>
        <Row label="Cliente" wide>
          <ClientSummary clientId={patient.clientId} name={patient.clientDisplayName} />
        </Row>
        <Row label="Notas administrativas" wide>
          {patient.administrativeNotes ?? "—"}
        </Row>
        <Row label="Profesional tratante" wide>
          <TreatingProfessional
            patient={patient}
            staff={staff}
            canAssign={canAssignTreating(role)}
            onChanged={onChanged}
          />
        </Row>
      </dl>

      {canManageGrants(role, patient, userId) ? (
        <GrantsPanel patient={patient} role={role} userId={userId} staff={staff} />
      ) : null}
      </div>
      ) : null}
    </section>
  );
}

function TabButton({
  id,
  controls,
  active,
  onClick,
  children,
}: {
  id: string;
  controls: string;
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      id={id}
      aria-controls={controls}
      aria-selected={active}
      tabIndex={active ? 0 : -1}
      onClick={onClick}
      className={`-mb-px border-b-2 px-3 py-2 text-sm ${
        active
          ? "border-brand font-medium text-ink"
          : "border-transparent text-ink-muted"
      }`}
    >
      {children}
    </button>
  );
}

function Row({
  label,
  wide,
  children,
}: {
  label: string;
  wide?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={wide ? "sm:col-span-2" : undefined}>
      <dt className="text-xs font-medium text-ink-muted">{label}</dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  );
}

/**
 * Who pays for this patient and on which fiscal data (T72, T74). The patient response carries
 * only the client id and its display name, so the rest is read from the client itself: a receipt
 * freezes this data, and showing a stale copy here would be the first place to disagree with it.
 */
function ClientSummary({ clientId, name }: { clientId: string; name: string }) {
  const { client, failedStatus } = useClient(clientId);

  return (
    <div>
      <Link href="/clientes" className="hover:underline">
        {name || "—"}
      </Link>
      {client ? (
        <span className="block text-xs text-ink-muted">
          {[
            fiscalDocumentText(client),
            fiscalConditionLabel(client.fiscalCondition),
            currencyLabel(client.billingCurrency),
          ]
            .filter((part) => part !== "" && part !== "—")
            .join(" · ") || "Sin datos fiscales"}
        </span>
      ) : failedStatus !== null ? (
        <span className="block text-xs text-ink-muted">No pudimos leer sus datos fiscales.</span>
      ) : null}
    </div>
  );
}
