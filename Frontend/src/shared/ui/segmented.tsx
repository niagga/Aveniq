"use client";

/**
 * A short list of mutually exclusive choices drawn as one control: the period of a report, the
 * cut of a chart, the currency of a figure.
 *
 * Each option is a button that states whether it is the chosen one (`aria-pressed`), so the
 * choice is announced and never carried by colour alone. The base `:focus-visible` rule owns the
 * focus ring.
 */
export function Segmented<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: readonly { value: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="inline-flex flex-wrap gap-0.5 rounded-control border border-border bg-surface p-0.5"
    >
      {options.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(option.value)}
            className={`rounded-control px-2.5 py-1 text-xs font-medium transition-colors ${
              active
                ? "bg-brand text-brand-ink"
                : "text-ink-muted hover:bg-surface-muted hover:text-ink"
            }`}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
