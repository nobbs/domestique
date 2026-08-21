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

import type { AnchorHTMLAttributes, ButtonHTMLAttributes } from "react";
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

export interface ExternalButtonLinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  variant?: ButtonVariant;
}

/**
 * The same, for a destination that is not this application.
 *
 * A plain anchor rather than the router's link, because the router has no route
 * to match and nothing to do with an address it does not own — and because the
 * attributes an outbound link needs, `target` and `rel`, belong to the element
 * itself rather than to a component pretending to navigate.
 *
 * It shares the two weights so that a control which leaves and a control which
 * acts are not two different-looking things sitting in one row. Where it goes
 * is said by its own label, not by dressing it down.
 */
export function ExternalButtonLink({
  variant = "standard",
  className,
  ...props
}: ExternalButtonLinkProps) {
  const classes = [styles.button, styles[variant], className].filter(Boolean).join(" ");

  return <a className={classes} {...props} />;
}
