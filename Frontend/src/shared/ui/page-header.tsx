import type { ReactNode } from "react";

/**
 * The header of a screen (E8): the title, a line of context, and the one primary action of the
 * screen ("Nuevo paciente", "Emitir comprobante"). The action sits on the right of the title
 * from `sm` up, and under it on a phone, where a row of two would not fit.
 */
export function PageHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description ? <div className="mt-1 text-sm text-ink-muted">{description}</div> : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </header>
  );
}
