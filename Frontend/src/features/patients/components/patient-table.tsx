"use client";

import {
  documentKindLabel,
  fiscalConditionLabel,
  formatBirthDate,
  type Patient,
} from "@/entities/patient/model";
import { Notice } from "@/shared/ui/notice";

/**
 * The patient roster.
 *
 * A deactivated patient stays visible when the list asks for inactive rows too:
 * hiding it would look like a deletion, and there is no deletion.
 */
export function PatientTable({
  patients,
  selectedID,
  onSelect,
}: {
  patients: Patient[];
  selectedID: string | null;
  onSelect: (id: string) => void;
}) {
  if (patients.length === 0) {
    return (
      <Notice tone="info" title="Todavía no hay pacientes">
        Cargá el primero con el formulario de arriba.
      </Notice>
    );
  }

  return (
    <div className="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th className="px-4 py-3 font-medium">Paciente</th>
            <th className="px-4 py-3 font-medium">Documento</th>
            <th className="px-4 py-3 font-medium">Nacimiento</th>
            <th className="px-4 py-3 font-medium">Contacto</th>
            <th className="px-4 py-3 font-medium">Estado</th>
          </tr>
        </thead>
        <tbody>
          {patients.map((patient) => (
            <tr
              key={patient.id}
              onClick={() => onSelect(patient.id)}
              className={`cursor-pointer border-t border-border ${
                selectedID === patient.id ? "bg-brand/5" : ""
              }`}
            >
              <td className="px-4 py-3 font-medium">{patient.fullName}</td>
              <td className="px-4 py-3 font-mono text-xs">
                {patient.document
                  ? `${documentKindLabel(patient.document.kind)} ${patient.document.number}`
                  : "—"}
              </td>
              <td className="px-4 py-3 text-ink-muted">{formatBirthDate(patient.birthDate) || "—"}</td>
              <td className="px-4 py-3 text-ink-muted">{patient.phone ?? "—"}</td>
              <td className="px-4 py-3">
                <span
                  className={`rounded-full border px-2 py-0.5 text-xs ${
                    patient.isActive
                      ? "border-success/30 bg-success/5 text-success"
                      : "border-border bg-surface-muted text-ink-muted"
                  }`}
                >
                  {patient.isActive ? "Activo" : "Inactivo"}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <p className="border-t border-border px-4 py-2 text-xs text-ink-muted">
        {patients.length} paciente{patients.length === 1 ? "" : "s"} · fiscales:{" "}
        {patients.filter((patient) => patient.taxId).length} con datos cargados
      </p>
    </div>
  );
}

/** Shows the fiscal condition next to a patient, used by the detail view. */
export function fiscalConditionText(patient: Patient): string {
  return fiscalConditionLabel(patient.fiscalCondition) || "Sin condición fiscal";
}
