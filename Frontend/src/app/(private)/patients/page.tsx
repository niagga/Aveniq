"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import type { Route } from "next";

import type { Patient } from "@/entities/patient/model";
import { canSeePatients } from "@/entities/iam/model";
import { PatientLoadError } from "@/features/patients/components/patient-detail";
import { PatientFormModal } from "@/features/patients/components/patient-form";
import { PatientTable } from "@/features/patients/components/patient-table";
import { useInsurers } from "@/features/insurers/use-insurers";
import { browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { RoleGate } from "@/shared/auth/role-gate";
import { Button } from "@/shared/ui/button";
import { FilterBar } from "@/shared/ui/filter-bar";
import { inputClass } from "@/shared/ui/field";
import { PageHeader } from "@/shared/ui/page-header";
import { LoadingState } from "@/shared/ui/states";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * Patient roster.
 *
 * A Client Component, like the dashboard: the access token lives
 * in browser memory, so the server has no credential to fetch with on the user's
 * behalf (T19). The auth gate and the shell come from the private layout.
 *
 * Since T75 the roster is a list again: the record lives on its own page
 * (`/patients/[id]`) and creating opens the popup. The filters stay visible
 * above the list while it loads or fails (S25).
 */
export default function PatientsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canSeePatients}>
        <PatientsScreen />
      </RoleGate>
    </ScreenFrame>
  );
}

function PatientsScreen() {
  const router = useRouter();
  const [patients, setPatients] = useState<Patient[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const [draftSearch, setDraftSearch] = useState("");
  const [search, setSearch] = useState("");

  // One request per pause, not one per keystroke. The list used to reload on every
  // character, which is what the redundant submit button was working around.
  useEffect(() => {
    const timer = setTimeout(() => setSearch(draftSearch.trim()), 300);
    return () => clearTimeout(timer);
  }, [draftSearch]);

  const [activeOnly, setActiveOnly] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const writesLocked = useWritesLocked();

  // The roster shows a mutual by name, and the patient form offers the catalog.
  const { insurers } = useInsurers(true);
  const insurerNames = useMemo(
    () => new Map((insurers ?? []).map((insurer) => [insurer.id, insurer.name])),
    [insurers],
  );

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

  return (
    <>
      <PageHeader
        title="Pacientes"
        description="La ficha del paciente con su historia clínica."
        action={
          <Button
            size="sm"
            variant="primary"
            onClick={() => setCreateOpen(true)}
            disabled={writesLocked}
          >
            Nuevo paciente
          </Button>
        }
      />

      <PatientFormModal
        open={createOpen}
        insurers={insurers}
        onClose={() => setCreateOpen(false)}
        onSaved={() => {
          setCreateOpen(false);
          void load();
        }}
      />

      <FilterBar label="Filtros de pacientes">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            // Enter applies the text at once instead of waiting for the debounce.
            setSearch(draftSearch.trim());
          }}
          className="flex flex-wrap items-end gap-3"
        >
          <div className="min-w-56 flex-1">
            <label htmlFor="patient-search" className="block text-xs font-medium text-ink-muted">
              Buscar
            </label>
            <input
              id="patient-search"
              value={draftSearch}
              onChange={(event) => setDraftSearch(event.target.value)}
              placeholder="Nombre o documento exacto"
              className={inputClass}
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

          <Button type="submit">Buscar</Button>
        </form>
      </FilterBar>

      {failedStatus !== null ? (
        <PatientLoadError status={failedStatus} />
      ) : patients === null ? (
        <LoadingState label="Cargando pacientes…" />
      ) : (
        <PatientTable
          patients={patients}
          selectedID={null}
          insurerNames={insurerNames}
          // The record is a page since T75: the popup of S25 belongs to create and edit.
          onSelect={(id) => router.push(`/patients/${id}` as Route)}
        />
      )}
    </>
  );
}
