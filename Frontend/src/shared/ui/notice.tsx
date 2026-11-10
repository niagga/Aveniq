import type { ReactNode } from "react";

type Tone = "info" | "danger" | "warning";

const TONE_CLASSES: Record<Tone, string> = {
  info: "border-border bg-surface text-ink-muted",
  warning: "border-warning/30 bg-warning/5 text-warning",
  danger: "border-danger/30 bg-danger/5 text-danger",
};

export function Notice({
  tone = "info",
  title,
  children,
}: {
  tone?: Tone;
  title: string;
  children?: ReactNode;
}) {
  return (
    <div
      role={tone === "danger" ? "alert" : "status"}
      className={`rounded-[var(--radius-panel)] border px-4 py-3 text-sm ${TONE_CLASSES[tone]}`}
    >
      <p className="font-medium">{title}</p>
      {children ? <div className="mt-1">{children}</div> : null}
    </div>
  );
}
