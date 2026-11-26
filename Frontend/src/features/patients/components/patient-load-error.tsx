"use client";

import { Notice } from "@/shared/ui/notice";

/**
 * What the patient screens say when reading one patient, or the roster, fails.
 *
 * It lived in `patient-detail.tsx` because the detail was the only screen that could fail;
 * since T75 the roster and the record are separate pages and both need it, so it has a home
 * of its own instead of riding along with a component neither imports.
 */
export function PatientLoadError({ status }: { status: number }) {
  return (
    <Notice tone="danger" title="No pudimos leer los pacientes">
      {status === 0
        ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
        : `La API respondió ${status}.`}
    </Notice>
  );
}
