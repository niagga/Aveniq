"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import type { Patient } from "@/entities/patient/model";
import { PatientDetail, PatientLoadError } from "@/features/patients/components/patient-detail";
import { PatientForm } from "@/features/patients/components/patient-form";
import { PatientTable } from "@/features/patients/components/patient-table";
import { canSeePatients } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * Patient roster.
 *
 * A Client Component inside SessionGate, like the dashboard: the access token lives
 * in browser memory, so the server has no credential to fetch with on the user's
 * behalf (T19).
 */
export default function PatientsPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canSeePatients}>
          <PatientsScreen />
        </RoleGate>
      </SessionGate>
    </main>
  );
}

function PatientsScreen() {
  const [patients, setPatients] = useState<Patient[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const [search, setSearch] = useState("");
  const [activeOnly, setActiveOnly] = useState(true);

  const [selected, setSelected] = useState<Patient | null>(null);
  const [mode, setMode] = useState<"idle" | "create" | "edit">("idle");

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/patients", {
      params: { query: { search, activeOnly, limit: 50, offset: 0 } },
    });

    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    setPatients(data?.items ?? []);
  }, [search, activeOnly]);

  useEffect(() => {
    let active = true;
    void (async () => {
      if (active) await load();
    })();
    return () => {
      active = false;
    };
  }, [load]);

  // The selected patient is refreshed from the list so the detail view cannot show
  // stale data after an edit or a deactivation.
  useEffect(() => {
    if (!selected || !patients) return;
    const current = patients.find((patient) => patient.id === selected.id);
    if (current && current !== selected) setSelected(current);
  }, [patients, selected]);

  function afterSave(id: string) {
    setMode("idle");
    void load();
    setSelected((current) => current ?? patients?.find((patient) => patient.id === id) ?? null);
  }

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Link href="/" className="text-xs text-ink-muted underline">
            ← Volver al consultorio
          </Link>
          {" · "}
          <Link href="/agenda" className="text-xs text-ink-muted underline">
            Agenda
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Pacientes</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Ficha del paciente con sus datos fiscales y la historia clínica.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setMode((current) => (current === "create" ? "idle" : "create"))}
          className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink"
        >
          {mode === "create" ? "Cerrar formulario" : "Nuevo paciente"}
        </button>
      </header>

      {mode === "create" ? <PatientForm onSaved={afterSave} onCancel={() => setMode("idle")} /> : null}

      {mode === "edit" && selected ? (
        <PatientForm
          patient={selected}
          onSaved={afterSave}
          onCancel={() => setMode("idle")}
        />
      ) : null}

      <form
        onSubmit={(event) => {
          event.preventDefault();
          void load();
        }}
        className="flex flex-wrap items-end gap-3"
      >
        <div className="min-w-56 flex-1">
          <label htmlFor="patient-search" className="block text-xs font-medium text-ink-muted">
            Buscar
          </label>
          <input
            id="patient-search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Nombre o documento exacto"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </div>

        <label className="flex items-center gap-2 text-xs text-ink-muted">
          <input
            type="checkbox"
            checked={!activeOnly}
            onChange={(event) => setActiveOnly(!event.target.checked)}
          />
          Incluir dados de baja
        </label>

        <button
          type="submit"
          className="rounded-md border border-border px-3 py-2 text-sm font-medium"
        >
          Buscar
        </button>
      </form>

      {failedStatus !== null ? (
        <PatientLoadError status={failedStatus} />
      ) : patients === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando pacientes…
        </p>
      ) : (
        <PatientTable
          patients={patients}
          selectedID={selected?.id ?? null}
          onSelect={(id) => {
            setSelected(patients.find((patient) => patient.id === id) ?? null);
            setMode("idle");
          }}
        />
      )}

      {selected ? (
        <PatientDetail
          patient={selected}
          onClose={() => setSelected(null)}
          onChanged={() => void load()}
          onEdit={() => setMode("edit")}
        />
      ) : null}
    </>
  );
}
