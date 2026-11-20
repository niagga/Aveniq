/**
 * The states every list and form needs, in one place.
 *
 * The screens used to answer "nothing to show" with a `Notice` about missing records, and
 * "still loading" with nothing at all — the agenda rendered a full week of "Sin turnos"
 * while its request was in flight, which is not an empty state but a false statement.
 *
 * A skeleton in `LoadingState` also holds the space the content will take, so the page does
 * not jump when the data lands.
 */

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="rounded-panel border border-border bg-surface px-6 py-10 text-center">
      <p className="text-sm font-medium">{title}</p>
      {description ? <p className="mt-1 text-sm text-ink-muted">{description}</p> : null}
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}

export function LoadingState({
  label = "Cargando…",
  rows = 3,
}: {
  label?: string;
  rows?: number;
}) {
  return (
    <div role="status" aria-live="polite" className="grid gap-2">
      <p className="text-sm text-ink-muted">{label}</p>
      <div aria-hidden="true" className="grid gap-2">
        {Array.from({ length: rows }, (_, index) => (
          <div
            key={index}
            className="h-10 animate-pulse rounded-control border border-border bg-surface-muted"
          />
        ))}
      </div>
    </div>
  );
}
