import type { ReactNode } from "react";

/**
 * The filters of a list, drawn once above it (E8, S25): the period, the professional, the
 * status. The controls are passed as children; the actions (for example "Nuevo" or
 * "Emitir comprobante") sit at the end, so a filter never pushes the primary action off screen.
 *
 * `sticky` keeps the bar in view while a long list scrolls. It is opt-in: only the screens with
 * a long list ask for it, so a short page does not carry a bar that floats over nothing.
 */
export function FilterBar({
  label,
  sticky = false,
  children,
  actions,
}: {
  label: string;
  sticky?: boolean;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div
      role="search"
      aria-label={label}
      className={`flex flex-wrap items-end gap-3 rounded-[var(--radius-panel)] border border-border bg-surface p-3 ${
        sticky ? "sticky top-0 z-10" : ""
      }`}
    >
      <div className="flex flex-wrap items-end gap-3">{children}</div>
      {actions ? <div className="ml-auto flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}
