"use client";

import Link from "next/link";

import { practiceHostHint, type PracticeLanding } from "@/entities/landing/model";
import { LandingEditorForm } from "@/features/landing/components/landing-editor-form";
import { usePracticeLanding } from "@/features/landing/use-practice-landing";
import { Notice } from "@/shared/ui/notice";
import { PageHeader } from "@/shared/ui/page-header";

/**
 * The owner edits what the practice shows on its public landing (S24, T66).
 */
export function PracticeSettingsScreen() {
  const { landing, failedStatus, reload, setLanding } = usePracticeLanding(true);

  function handleSaved(saved: PracticeLanding) {
    setLanding(saved);
    void reload();
  }

  if (failedStatus !== null) {
    return (
      <Notice tone="danger" title="No pudimos leer la landing">
        El servidor respondió {failedStatus}. Revisá que la API esté corriendo y que tu cuenta
        pueda configurar el consultorio.
      </Notice>
    );
  }

  if (landing === null) {
    return <p className="text-sm text-ink-muted">Cargando…</p>;
  }

  const hostHint = practiceHostHint(landing.slug);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Mi consultorio"
        description={
          <>
            Es la primera pantalla que ven las personas que entran por la dirección del consultorio.
            En desarrollo, probala en{" "}
            <a
              href={`http://${hostHint}:3000/`}
              className="font-mono text-brand underline"
              target="_blank"
              rel="noreferrer"
            >
              {hostHint}:3000
            </a>
            .{" "}
            <Link href="/" target="_blank" className="font-medium text-brand underline">
              Ver landing en esta pestaña
            </Link>
          </>
        }
      />

      <LandingEditorForm initial={landing} onSaved={handleSaved} />
    </div>
  );
}