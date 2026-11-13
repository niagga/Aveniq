"use client";

import { useEffect, useState } from "react";

import {
  documentKindLabel,
  fiscalConditionLabel,
  formatBirthDate,
  taxIDKindLabel,
  type Patient,
} from "@/entities/patient/model";
import {
  canAssignTreating,
  canManageGrants,
  canOpenClinicalRecord,
  canWriteClinicalRecord,
} from "@/entities/care/model";
import { ClinicalPanel } from "@/features/patients/components/clinical-panel";
import { GrantsPanel } from "@/features/patients/components/grants-panel";
import { TreatingProfessional } from "@/features/patients/components/treating-professional";
import { useStaff } from "@/features/patients/staff";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";

/**
 * The read view of one patient.
 *
 * Deactivation lives here and not in the table: it is a deliberate act on a person's
 * record, and a button per row is a button somebody clicks by accident while
 * scrolling. There is no delete, and nothing in this screen suggests one.
 */
export function PatientDetail({
  patient,
  onClose,
  onChanged,
  onEdit,
}: {
  patient: Patient;
  onClose: () => void;
  onChanged: () => void;
  onEdit: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setIsBusy] = useState(false);
  const [tab, setTab] = useState<"admin" | "clinical">("admin");

  // The viewer decides what this screen offers; the API decides what is allowed.
  const viewer = getSessionUser();
  const role = viewer?.role;
  const userId = viewer?.id ?? "";
  const staff = useStaff(role);

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
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Editar
          </button>
          <button
            type="button"
            onClick={toggleActive}
            disabled={isBusy}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink-muted disabled:opacity-60"
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

      <div className="mt-4 flex gap-1 border-b border-border" role="tablist" aria-label="Ficha del paciente">
        <TabButton active={tab === "admin"} onClick={() => setTab("admin")}>
          Datos
        </TabButton>
        {canOpenClinicalRecord(role) ? (
          <TabButton active={tab === "clinical"} onClick={() => setTab("clinical")}>
            Historia clínica
          </TabButton>
        ) : null}
      </div>

      {tab === "clinical" && canOpenClinicalRecord(role) ? (
        <ClinicalPanel
          key={patient.id}
          patientID={patient.id}
          readOnly={!canWriteClinicalRecord(role, patient, userId)}
        />
      ) : null}

      {tab === "admin" ? (
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
        <Row label="Identificador fiscal">
          {patient.taxId ? `${taxIDKindLabel(patient.taxId.kind)} ${patient.taxId.number}` : "—"}
        </Row>
        <Row label="Condición fiscal">
          {fiscalConditionLabel(patient.fiscalCondition) || "—"}
        </Row>
        <Row label="Dirección fiscal">{patient.fiscalAddress ?? "—"}</Row>
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
      ) : null}

      {tab === "admin" && canManageGrants(role, patient, userId) ? (
        <GrantsPanel patient={patient} role={role} userId={userId} staff={staff} />
      ) : null}
    </section>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
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

/** Renders the API complaint when loading the roster fails, instead of an empty table. */
export function PatientLoadError({ status }: { status: number }) {
  return (
    <Notice tone="danger" title="No pudimos leer los pacientes">
      {status === 0
        ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
        : `La API respondió ${status}.`}
    </Notice>
  );
}
