"use client";

import {
  coverageText,
  documentKindLabel,
  fiscalConditionLabel,
  formatBirthDate,
  type Patient,
} from "@/entities/patient/model";
import { Badge } from "@/shared/ui/badge";
import { EmptyState } from "@/shared/ui/states";

/**
 * The patient roster.
 *
 * A deactivated patient stays visible when the list asks for inactive rows too:
 * hiding it would look like a deletion, and there is no deletion.
 */
export function PatientTable({
  patients,
  selectedID,
  insurerNames,
  onSelect,
}: {
  patients: Patient[];
  selectedID: string | null;
  insurerNames: ReadonlyMap<string, string>;
  onSelect: (id: string) => void;
}) {
  if (patients.length === 0) {
    return (
      <EmptyState
        title="Todavía no hay pacientes"
        description="Cargá el primero con el formulario de arriba."
      />
    );
  }

  // overflow-x-auto and not overflow-hidden: the roster has seven columns and hidden
  // overflow crops them on a narrow screen instead of letting the table scroll.
  return (
    <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">Paciente</th>
            <th scope="col" className="px-4 py-3 font-medium">Documento</th>
            <th scope="col" className="px-4 py-3 font-medium">Nacimiento</th>
            <th scope="col" className="px-4 py-3 font-medium">Contacto</th>
            <th scope="col" className="px-4 py-3 font-medium">Cobertura</th>
            <th scope="col" className="px-4 py-3 font-medium">Moneda</th>
            <th scope="col" className="px-4 py-3 font-medium">Estado</th>
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
              <td className="px-4 py-3 font-medium">
                {/* A row that is only clickable is unreachable without a mouse: the name is
                    the control, and the row keeps the click as a convenience. */}
                <button
                  type="button"
                  onClick={() => onSelect(patient.id)}
                  className="text-left hover:underline"
                >
                  {patient.fullName}
                </button>
              </td>
              <td className="px-4 py-3 font-mono text-xs">
                {patient.document
                  ? `${documentKindLabel(patient.document.kind)} ${patient.document.number}`
                  : "—"}
              </td>
              <td className="px-4 py-3 text-ink-muted">{formatBirthDate(patient.birthDate) || "—"}</td>
              <td className="px-4 py-3 text-ink-muted">{patient.phone ?? "—"}</td>
              <td className="px-4 py-3 text-ink-muted">
                {coverageText(
                  patient,
                  patient.insurerId ? insurerNames.get(patient.insurerId) : undefined,
                )}
              </td>
              <td className="px-4 py-3 font-mono text-xs text-ink-muted">
                {patient.billingCurrency || "—"}
              </td>
              <td className="px-4 py-3">
                <Badge tone={patient.isActive ? "success" : "neutral"}>
                  {patient.isActive ? "Activo" : "Inactivo"}
                </Badge>
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
