/**
 * The shared ordinary action button.
 *
 * It carries the visual rules only: everything else — the label, the disabled
 * state while a request is in flight, the accessible name where the visible
 * words are ambiguous — belongs to the feature that presses it, and is passed
 * straight through to the native element.
 *
 * `type` is not among the props it accepts. This UI has no forms, so the only
 * honest value is `button`, and a shared control that could quietly become a
 * submit button is a trap rather than a convenience.
 */

import type { ButtonHTMLAttributes } from "react";
import styles from "./Button.module.css";

/** How much weight the action carries where it sits. */
export type ButtonVariant = "standard" | "quiet";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "type"> {
  variant?: ButtonVariant;
}

export function Button({ variant = "standard", className, ...props }: ButtonProps) {
  const classes = [styles.button, styles[variant], className].filter(Boolean).join(" ");

  return <button type="button" className={classes} {...props} />;
}
