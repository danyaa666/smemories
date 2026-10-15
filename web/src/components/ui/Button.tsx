import type { ButtonHTMLAttributes, Ref } from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

/** Class names for a link or label that must look like a button. */
export const buttonClass = (variant: ButtonVariant = "secondary") => `btn btn-${variant}`;

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  /** Work in progress: the button is disabled and announced as busy. */
  busy?: boolean;
  ref?: Ref<HTMLButtonElement>;
};

/** Tomato (primary) is for the one main action on a screen. 44 px minimum, 52 px for primary on mobile. */
export function Button({
  variant = "secondary",
  busy,
  disabled,
  className,
  type = "button",
  ...rest
}: Props) {
  return (
    <button
      type={type}
      className={[buttonClass(variant), className].filter(Boolean).join(" ")}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      {...rest}
    />
  );
}
