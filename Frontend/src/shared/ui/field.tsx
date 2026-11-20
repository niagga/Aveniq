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

/** Add on top of `inputClass` when the field carries an error. */
export const inputInvalidClass = "border-danger";

/**
 * A labelled control. The label is bound to the control by `htmlFor`.
 *
 * An error belongs next to its field, not only in one banner at the top of the form: the
 * forms used to surface every failure through a single global message, so the user had to
 * find which field was wrong. When `error` is set, wire the control with
 * `aria-invalid` and `aria-describedby={fieldErrorId(htmlFor)}`.
 */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  children,
}: {
  label: string;
  htmlFor: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label htmlFor={htmlFor} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {children}
      {error ? (
        <p id={fieldErrorId(htmlFor)} role="alert" className="mt-1 text-xs text-danger">
          {error}
        </p>
      ) : hint ? (
        <p className="mt-1 text-xs text-ink-muted">{hint}</p>
      ) : null}
    </div>
  );
}

/** The id a control points at with `aria-describedby` when it carries an error. */
export function fieldErrorId(htmlFor: string): string {
  return `${htmlFor}-error`;
}
