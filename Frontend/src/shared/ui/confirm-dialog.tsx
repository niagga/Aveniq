"use client";

import { useEffect, useId, useRef, type ReactNode } from "react";

import { Button } from "./button";

/**
 * The one confirmation dialog (S5).
 *
 * The app had three ways of asking the same question: the voucher void asked twice and
 * demanded a reason, tariffs called `window.confirm`, and six destructive actions asked
 * nothing at all. This is the single idiom: a native `<dialog>` opened with `showModal`,
 * so the focus trap, the Escape key and the backdrop come from the platform instead of
 * from hand-written key handlers.
 *
 * The caller owns `open`; confirming does not close the dialog by itself, because only the
 * caller knows whether the request succeeded.
 */

type Tone = "danger" | "default";

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel = "Cancelar",
  tone = "danger",
  busy = false,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  tone?: Tone;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  // Two dialogs can be mounted at once, so the ids cannot be constants.
  const baseId = useId();
  const titleId = `${baseId}-title`;
  const descriptionId = `${baseId}-description`;

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

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      onCancel={(event) => {
        // Escape goes through the caller, so screen state and the dialog cannot disagree.
        event.preventDefault();
        if (!busy) onCancel();
      }}
      className="m-auto w-full max-w-md rounded-panel border border-border bg-surface p-5 text-ink backdrop:bg-ink/40"
    >
      <h2 id={titleId} className="text-base font-semibold tracking-tight">
        {title}
      </h2>
      {description ? (
        <div id={descriptionId} className="mt-2 text-sm text-ink-muted">
          {description}
        </div>
      ) : null}

      <div className="mt-5 flex justify-end gap-2">
        <Button onClick={onCancel} disabled={busy} variant="secondary">
          {cancelLabel}
        </Button>
        <Button
          onClick={onConfirm}
          busy={busy}
          variant={tone === "danger" ? "danger" : "primary"}
        >
          {confirmLabel}
        </Button>
      </div>
    </dialog>
  );
}
