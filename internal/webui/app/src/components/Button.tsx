/**
 * The shared action button.
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
import { Link, type LinkProps } from "react-router";
import styles from "./Button.module.css";

/**
 * How much weight the action carries where it sits.
 *
 * There are two, and there is no third. A view has at most one primary — the
 * thing that view is for — and everything else is an outlined control. A middle
 * weight would only ever be used to argue with the primary.
 */
export type ButtonVariant = "primary" | "standard";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "type"> {
  variant?: ButtonVariant;
}

export function Button({ variant = "standard", className, ...props }: ButtonProps) {
  const classes = [styles.button, styles[variant], className].filter(Boolean).join(" ");

  return <button type="button" className={classes} {...props} />;
}

export interface ButtonLinkProps extends LinkProps {
  variant?: ButtonVariant;
}

/**
 * A navigation that looks like an action.
 *
 * It is a link, and stays one — middle-click, copy the address, open in a new
 * tab all still work — because "Open route" goes somewhere rather than does
 * something. Only the appearance is shared, which is why it borrows the same
 * two variants rather than defining a third.
 */
export function ButtonLink({ variant = "standard", className, ...props }: ButtonLinkProps) {
  const classes = [styles.button, styles[variant], className].filter(Boolean).join(" ");

  return <Link className={classes} {...props} />;
}
