"use client";

import type { ButtonHTMLAttributes, ReactNode } from "react";

/**
 * The one button of the app.
 *
 * Every screen used to spell out its own `rounded-md border border-border px-3 py-1.5
 * text-xs`, in three slightly different sizes and with three different disabled
 * treatments. This keeps one set of variants on the design tokens.
 *
 * Focus is not handled here on purpose: the base `:focus-visible` rule in globals.css
 * owns it, so no button can opt out of a visible focus ring.
 */

type Variant = "primary" | "secondary" | "danger" | "ghost";
type Size = "sm" | "md";

const VARIANTS: Record<Variant, string> = {
  primary: "border border-brand bg-brand text-brand-ink hover:bg-brand/90",
  secondary: "border border-border-strong bg-surface text-ink hover:bg-surface-muted",
  danger: "border border-danger/40 bg-surface text-danger hover:bg-danger/5",
  ghost: "border border-transparent bg-transparent text-ink-muted hover:bg-surface-muted hover:text-ink",
};

const SIZES: Record<Size, string> = {
  sm: "px-2.5 py-1.5 text-xs",
  md: "px-3.5 py-2 text-sm",
};

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  /**
   * Busy is not disabled: a control that silently drops clicks is worse than one that
   * says what it is doing. It stays disabled for the click, and announces the state.
   */
  busy?: boolean;
  children: ReactNode;
};

export function Button({
  variant = "secondary",
  size = "md",
  busy = false,
  children,
  className = "",
  disabled,
  type = "button",
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`inline-flex items-center justify-center gap-2 rounded-control font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${VARIANTS[variant]} ${SIZES[size]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
}
