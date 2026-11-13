"use client";

import { useId, useState } from "react";

import type { Patient } from "@/entities/patient/model";
import { careFailureMessage } from "@/entities/care/model";
import { browserApi } from "@/shared/api/client";
import { staffName, type StaffMember } from "@/features/patients/staff";

/**
 * Who treats the patient, and for owner and admin, the control to change it.
 *
 * The treating professional is the one who writes the clinical record and may book the
 * patient. Only the owner and admin reassign it (T38); the API refuses anyone else.
 */
export function TreatingProfessional({
  patient,
  staff,
  canAssign,
  onChanged,
}: {
  patient: Patient;
  staff: StaffMember[];
  canAssign: boolean;
  onChanged: () => void;
}) {
  const selectId = useId();
  const professionals = staff.filter((member) => member.role === "professional");
  const [choice, setChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const current = patient.treatingProfessionalId;
  const canSubmit = choice !== "" && choice !== current && !busy;

  async function reassign() {
    setError(null);
    setBusy(true);
    try {
      const result = await browserApi.PUT("/api/v1/patients/{id}/treating-professional", {
        params: { path: { id: patient.id } },
        body: { professionalId: choice },
      });
      if (result.error || !result.data) {
        setError(careFailureMessage(result.response.status));
        return;
      }
      setChoice("");
      onChanged();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-2">
      <p>{current ? staffName(staff, current) : "Sin asignar"}</p>

      {canAssign ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit) void reassign();
          }}
          className="flex flex-wrap items-end gap-2"
        >
          <div>
            <label htmlFor={selectId} className="block text-xs font-medium text-ink-muted">
              Reasignar a
            </label>
            <select
              id={selectId}
              value={choice}
              onChange={(event) => setChoice(event.target.value)}
              className="mt-1 rounded-md border border-border bg-surface px-3 py-1.5 text-sm"
            >
              <option value="">Elegí un profesional</option>
              {professionals.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.fullName}
                </option>
              ))}
            </select>
          </div>
          <button
            type="submit"
            disabled={!canSubmit}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium disabled:opacity-60"
          >
            {busy ? "Guardando…" : "Reasignar tratante"}
          </button>
          {professionals.length === 0 ? (
            <p className="text-xs text-ink-muted">No hay profesionales activos para asignar.</p>
          ) : null}
        </form>
      ) : null}

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
