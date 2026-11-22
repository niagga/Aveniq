import Link from "next/link";
import type { ReactNode } from "react";

import type { PublicLanding } from "@/entities/landing/model";

function ContactRow({ label, children }: { label: string; children: ReactNode }) {
  if (!children) return null;
  return (
    <div>
      <dt className="text-xs font-medium text-ink-muted">{label}</dt>
      <dd className="mt-0.5 text-sm text-ink">{children}</dd>
    </div>
  );
}

/**
 * The public home of one practice, rendered on the server from `GET /public/landing`.
 */
export function PracticeLanding({ landing }: { landing: PublicLanding }) {
  const hasContact =
    landing.address.trim() !== "" ||
    landing.phone.trim() !== "" ||
    landing.email.trim() !== "" ||
    landing.schedule.trim() !== "";

  return (
    <main className="mx-auto flex min-h-full w-full max-w-lg flex-col gap-8 px-6 py-16">
      <header className="space-y-2">
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <h1 className="text-2xl font-semibold tracking-tight text-ink">{landing.displayName}</h1>
        {landing.tagline.trim() !== "" ? (
          <p className="text-sm text-ink-muted">{landing.tagline}</p>
        ) : null}
      </header>

      {landing.about.trim() !== "" ? (
        <section className="text-sm leading-relaxed text-ink whitespace-pre-line">{landing.about}</section>
      ) : null}

      {hasContact ? (
        <section aria-labelledby="contact-heading">
          <h2 id="contact-heading" className="text-sm font-semibold text-ink">
            Contacto
          </h2>
          <dl className="mt-3 grid gap-3">
            <ContactRow label="Dirección">{landing.address.trim() || null}</ContactRow>
            <ContactRow label="Teléfono">
              {landing.phone.trim() !== "" ? (
                <a href={`tel:${landing.phone.replace(/\s/g, "")}`} className="text-brand underline">
                  {landing.phone}
                </a>
              ) : null}
            </ContactRow>
            <ContactRow label="Email">
              {landing.email.trim() !== "" ? (
                <a href={`mailto:${landing.email}`} className="text-brand underline">
                  {landing.email}
                </a>
              ) : null}
            </ContactRow>
            <ContactRow label="Horarios">
              {landing.schedule.trim() !== "" ? (
                <span className="whitespace-pre-line">{landing.schedule}</span>
              ) : null}
            </ContactRow>
          </dl>
        </section>
      ) : null}

      <div>
        <Link
          href="/login"
          className="inline-flex w-fit items-center rounded-control border border-brand bg-brand px-4 py-2 text-sm font-medium text-brand-ink hover:bg-brand/90"
        >
          Entrar
        </Link>
        <p className="mt-3 text-xs text-ink-muted">Acceso para el equipo del consultorio.</p>
      </div>
    </main>
  );
}