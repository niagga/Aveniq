import type { ReactNode } from "react";

/**
 * The one input style of the forms: same border, focus and width everywhere.
 *
 * `border-border-strong` instead of `border-border`: the default border sits at
 * 1.2:1 against the surface, below the 3:1 a control boundary needs. The focus
 * ring itself comes from the base `:focus-visible` rule in globals.css, so this
 * class no longer opts out with `outline-none`.
 */
export const inputClass =
  "mt-1 w-full rounded-control border border-border-strong bg-surface px-3 py-2 text-sm focus:border-brand";

/** A labelled control. The label is bound to the control by `htmlFor`. */
export function Field({
  label,
  htmlFor,
  hint,
  children,
}: {
  label: string;
  htmlFor: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label htmlFor={htmlFor} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {children}
      {hint ? <p className="mt-1 text-xs text-ink-muted">{hint}</p> : null}
    </div>
  );
}
