"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";

import { Button } from "./button";

/**
 * The one popup of the app (E8, S25): creating, viewing and editing open here, over a page
 * that is blurred behind them.
 *
 * It is a native `<dialog>` opened with `showModal`, so the focus trap, the return of focus to
 * the control that opened it and the Escape key come from the platform. Escape, the backdrop
 * and the close button all ask to close through `onClose`, so the caller's state and the
 * dialog cannot disagree.
 *
 * Below `md` it is a full-screen sheet instead of a centred box: a phone has no room for a box
 * over the keyboard, and the sheet keeps the close button where a thumb reaches it.
 *
 * `dirty` turns on the unsaved-changes guard: closing asks before it throws typed text away.
 * `busy` refuses every close request while a save is in flight.
 */

type Size = "sm" | "md" | "lg";

const SIZES: Record<Size, string> = {
  sm: "md:max-w-sm",
  md: "md:max-w-md",
  lg: "md:max-w-2xl",
};

export function Modal({
  open,
  title,
  description,
  size = "md",
  dirty = false,
  busy = false,
  onClose,
  children,
  footer,
}: {
  open: boolean;
  title: string;
  description?: ReactNode;
  size?: Size;
  dirty?: boolean;
  busy?: boolean;
  onClose: () => void;
  children?: ReactNode;
  /** Receives `requestClose`, so a Cancel button goes through the same unsaved-changes guard. */
  footer?: (requestClose: () => void) => ReactNode;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  // Two popups can be mounted at once, so the ids cannot be constants.
  const baseId = useId();
  const titleId = `${baseId}-title`;
  const descriptionId = `${baseId}-description`;
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    if (open && !dialog.open) {
      dialog.showModal();
      return;
    }
    if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  // A closed popup forgets the discard question, so the next opening starts clean.
  useEffect(() => {
    if (!open) setConfirmingDiscard(false);
  }, [open]);

  function requestClose() {
    if (busy) return;
    if (dirty) {
      setConfirmingDiscard(true);
      return;
    }
    onClose();
  }

  // Escape is taken from the keyboard here, not from the dialog's `cancel` event. Chrome sends
  // `cancel` only after a user activation since the last cancelled request, so a second Escape
  // with no click in between closed the popup without asking, past the unsaved-changes guard.
  // Preventing the keydown keeps the platform from closing the dialog on its own.
  useEffect(() => {
    if (!open) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return;
      event.preventDefault();
      if (confirmingDiscard) setConfirmingDiscard(false);
      else requestClose();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  });

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      onCancel={(event) => {
        // Escape is handled here, not by the platform, so it follows the same rules as the
        // close button. While the discard question is up, Escape means "keep editing".
        event.preventDefault();
        if (confirmingDiscard) setConfirmingDiscard(false);
        else requestClose();
      }}
      onClick={(event) => {
        // A click on the dialog element itself is a click on the blurred backdrop: the
        // content sits in a child element that fills the box.
        if (event.target === event.currentTarget) requestClose();
      }}
      className={`m-auto max-h-[90dvh] w-full overflow-hidden rounded-panel border border-border bg-surface p-0 text-ink backdrop:bg-ink/40 backdrop:backdrop-blur-sm open:flex max-md:h-dvh max-md:max-h-none max-md:max-w-none max-md:rounded-none max-md:border-0 ${SIZES[size]}`}
    >
      <div className="flex max-h-full min-h-0 w-full flex-col">
        <header className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
          <div className="min-w-0">
            <h2 id={titleId} className="text-base font-semibold tracking-tight">
              {title}
            </h2>
            {description ? (
              <div id={descriptionId} className="mt-1 text-sm text-ink-muted">
                {description}
              </div>
            ) : null}
          </div>
          <Button variant="ghost" size="sm" onClick={requestClose} disabled={busy}>
            Cerrar
          </Button>
        </header>

        <div className={confirmingDiscard ? "hidden" : "flex min-h-0 flex-1 flex-col"}>
          {/* Without a body (a plain confirmation) the spacer keeps the footer at the bottom of a sheet. */}
          <div className={children ? "min-h-0 flex-1 overflow-y-auto px-5 py-4" : "flex-1"}>{children}</div>
          {footer ? (
            <footer className="flex flex-wrap items-center justify-end gap-2 border-t border-border px-5 py-3">
              {footer(requestClose)}
            </footer>
          ) : null}
        </div>

        {confirmingDiscard ? (
          <div className="px-5 py-4">
            <p className="text-sm font-semibold">¿Descartar los cambios?</p>
            <p className="mt-1 text-sm text-ink-muted">Lo que escribiste en este formulario no se guarda.</p>
            <div className="mt-4 flex flex-wrap justify-end gap-2">
              <Button variant="secondary" onClick={() => setConfirmingDiscard(false)} autoFocus>
                Seguir editando
              </Button>
              <Button
                variant="danger"
                onClick={() => {
                  setConfirmingDiscard(false);
                  onClose();
                }}
              >
                Descartar cambios
              </Button>
            </div>
          </div>
        ) : null}
      </div>
    </dialog>
  );
}
