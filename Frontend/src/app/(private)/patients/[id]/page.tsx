"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";

import type { Patient } from "@/entities/patient/model";
import { canSeePatients } from "@/entities/iam/model";
import { PatientDetail, PatientLoadError } from "@/features/patients/components/patient-detail";
import { PatientFormModal } from "@/features/patients/components/patient-form";
import { useInsurers } from "@/features/insurers/use-insurers";
import { browserApi } from "@/shared/api/client";
import { RoleGate } from "@/shared/auth/role-gate";
import { Notice } from "@/shared/ui/notice";
import { LoadingState } from "@/shared/ui/states";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * The record of one patient, on its own page (T75, S25).
 *
 * A Client Component, like the roster: the access token lives in browser memory
 * (T19). The auth gate and the shell come from the private layout.
 *
 * Creating and editing open the popup; the page only ever shows the record, so
 * its tabs and the panels that hang off them keep the behavior they had when
 * they lived under the roster.
 */
export default function PatientRecordPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canSeePatients}>
        <PatientRecord />
      </RoleGate>
    </ScreenFrame>
  );
}

function PatientRecord() {
  const params = useParams<{ id: string }>();
  const id = params.id;
  const router = useRouter();

  const [patient, setPatient] = useState<Patient | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [editOpen, setEditOpen] = useState(false);

  // The record shows a mutual by name, and the patient form offers the catalog.
  const { insurers } = useInsurers(true);
  const insurerNames = useMemo(
    () => new Map((insurers ?? []).map((insurer) => [insurer.id, insurer.name])),
    [insurers],
  );

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/patients/{id}", {
      params: { path: { id } },
    });

    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    if (data) setPatient(data);
  }, [id]);

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
      {failedStatus === 404 ? (
        <Notice tone="danger" title="No encontramos el paciente">
          El paciente que buscás no existe en este consultorio.
        </Notice>
      ) : failedStatus !== null ? (
        <PatientLoadError status={failedStatus} />
      ) : patient === null ? (
        <LoadingState label="Cargando paciente…" />
      ) : (
        <>
          <PatientDetail
            patient={patient}
            insurerNames={insurerNames}
            onClose={() => router.push("/patients")}
            onChanged={() => void load()}
            onEdit={() => setEditOpen(true)}
          />

          <PatientFormModal
            open={editOpen}
            patient={patient}
            insurers={insurers}
            onClose={() => setEditOpen(false)}
            onSaved={() => {
              setEditOpen(false);
              void load();
            }}
          />
        </>
      )}
    </>
  );
}
