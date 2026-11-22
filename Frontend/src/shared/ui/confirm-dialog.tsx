"use client";

import type { ReactNode } from "react";

import { Button } from "./button";
import { Modal } from "./modal";

/**
 * The one confirmation dialog (S5).
 *
 * The app had three ways of asking the same question: the voucher void asked twice and
 * demanded a reason, tariffs called `window.confirm`, and six destructive actions asked
 * nothing at all. This is the single idiom, and since E8 (T62) it is a `Modal` at the small
 * size, so the blurred backdrop, the full-screen sheet on a phone and the Escape key behave the
 * same here as in every popup.
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
  return (
    <Modal
      open={open}
      title={title}
      description={description}
      size="sm"
      busy={busy}
      onClose={onCancel}
      footer={(requestClose) => (
        <>
          <Button onClick={requestClose} disabled={busy} variant="secondary">
            {cancelLabel}
          </Button>
          <Button onClick={onConfirm} busy={busy} variant={tone === "danger" ? "danger" : "primary"}>
            {confirmLabel}
          </Button>
        </>
      )}
    />
  );
}
