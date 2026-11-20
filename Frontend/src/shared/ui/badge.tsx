/**
 * Status pills.
 *
 * The patient roster already drew these by hand with `rounded-full border px-2 py-0.5
 * text-xs` plus a tone, and every other screen reinvented a slightly different one. The
 * classes match the roster's, so migrating is visually neutral.
 *
 * A badge is never the only signal: the label spells the state out, because colour alone
 * is not a status.
 */

type Tone = "neutral" | "success" | "warning" | "danger";

const TONES: Record<Tone, string> = {
  neutral: "border-border bg-surface-muted text-ink-muted",
  success: "border-success/30 bg-success/5 text-success",
  warning: "border-warning/30 bg-warning/5 text-warning",
  danger: "border-danger/30 bg-danger/5 text-danger",
};

export function Badge({
  tone = "neutral",
  children,
}: {
  tone?: Tone;
  children: React.ReactNode;
}) {
  return (
    <span
      className={`inline-block whitespace-nowrap rounded-full border px-2 py-0.5 text-xs ${TONES[tone]}`}
    >
      {children}
    </span>
  );
}
