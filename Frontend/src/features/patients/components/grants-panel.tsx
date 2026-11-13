"use client";

import { useCallback, useEffect, useId, useState } from "react";

import {
  canGrantClinicalRecord,
  careFailureMessage,
  type ClinicalGrant,
} from "@/entities/care/model";
import type { Patient } from "@/entities/patient/model";
import { browserApi } from "@/shared/api/client";
import { staffName, type StaffMember } from "@/features/patients/staff";

/**
 * Who else can read this patient's clinical record (S23, T38).
 *
 * The treating professional grants and withdraws. The owner sees the list and can
 * withdraw a grant, because a grant must not outlive a treating professional who was
 * replaced. A granted professional reads the file and the record; it cannot book the
 * patient or write the record, and it cannot pass the grant on.
 */
export function GrantsPanel({
  patient,
  role,
  userId,
  staff,
}: {
  patient: Patient;
  role: string | undefined;
  userId: string;
  staff: StaffMember[];
}) {
  const selectId = useId();
  const [grants, setGrants] = useState<ClinicalGrant[] | null>(null);
  const [choice, setChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canGrant = canGrantClinicalRecord(role, patient, userId);
  const live = (grants ?? []).filter((grant) => !grant.revokedAt);
  const eligible = staff.filter(
    (member) =>
      member.role === "professional" &&
      member.id !== patient.treatingProfessionalId &&
      !live.some((grant) => grant.professionalId === member.id),
  );

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/patients/{id}/clinical-grants", {
      params: { path: { id: patient.id } },
    });
    if (!response.ok || !data) {
      setError(careFailureMessage(response.status));
      return;
    }
    setGrants(data.items ?? []);
  }, [patient.id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function grant() {
    if (choice === "") return;
    setError(null);
    setBusy(true);
    try {
      const result = await browserApi.POST("/api/v1/patients/{id}/clinical-grants", {
        params: { path: { id: patient.id } },
        body: { professionalId: choice },
      });
      if (result.error || !result.data) {
        setError(careFailureMessage(result.response.status));
        return;
      }
      setChoice("");
      await load();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  async function revoke(grantID: string) {
    setError(null);
    setBusy(true);
    try {
      const result = await browserApi.POST("/api/v1/patients/{id}/clinical-grants/{grantId}/revoke", {
        params: { path: { id: patient.id, grantId: grantID } },
      });
      if (result.error || !result.data) {
        setError(careFailureMessage(result.response.status));
        return;
      }
      await load();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="mt-6 space-y-3 border-t border-border pt-5">
      <div>
        <h3 className="text-sm font-semibold">Acceso a la historia clínica</h3>
        <p className="mt-0.5 text-xs text-ink-muted">
          Un profesional con acceso lee la ficha y la historia clínica. No puede agendar al
          paciente ni escribir en la historia.
        </p>
      </div>

      {grants === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando accesos…
        </p>
      ) : live.length === 0 ? (
        <p className="text-sm text-ink-muted">Ningún otro profesional tiene acceso.</p>
      ) : (
        <ul className="space-y-2">
          {live.map((grant) => {
            const name = staffName(staff, grant.professionalId);
            return (
              <li
                key={grant.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-surface-muted px-3 py-2 text-sm"
              >
                <span>{name}</span>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void revoke(grant.id)}
                  aria-label={`Revocar acceso de ${name}`}
                  className="rounded-md border border-border px-3 py-1 text-xs font-medium disabled:opacity-60"
                >
                  Revocar acceso
                </button>
              </li>
            );
          })}
        </ul>
      )}

      {canGrant ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void grant();
          }}
          className="flex flex-wrap items-end gap-2"
        >
          <div>
            <label htmlFor={selectId} className="block text-xs font-medium text-ink-muted">
              Dar acceso a
            </label>
            <select
              id={selectId}
              value={choice}
              onChange={(event) => setChoice(event.target.value)}
              className="mt-1 rounded-md border border-border bg-surface px-3 py-1.5 text-sm"
            >
              <option value="">Elegí un profesional</option>
              {eligible.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.fullName}
                </option>
              ))}
            </select>
          </div>
          <button
            type="submit"
            disabled={busy || choice === ""}
            className="rounded-md bg-brand px-3 py-1.5 text-xs font-medium text-brand-ink disabled:opacity-60"
          >
            {busy ? "Guardando…" : "Dar acceso"}
          </button>
        </form>
      ) : null}

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </section>
  );
}
