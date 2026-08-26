/**
 * The one button this application reaches for.
 *
 * It is a thin naming layer over `ui/button`: the shadcn primitive owns the
 * shape, the focus treatment and the icon handling, and this file decides
 * which of its many variants and sizes this application actually uses. Nothing
 * outside `components/ui` imports the primitive directly — a single vocabulary
 * is the whole point, and the last time there were two, half the application
 * was 28 pixels tall and the other half 32.
 *
 * Text, a glyph, or both: the label is the children and the glyph is `icon`.
 * Splitting them is what lets this decide the shape — a button with no label is
 * the square — and lets the glyph be hidden from assistive technology once,
 * here, rather than correctly at every call site.
 */

import { IconExternalLink } from "@tabler/icons-react";
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode } from "react";
import { Link, type LinkProps } from "react-router";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * `primary` is the filled one action a card is about; `standard` is every
 * other action; `panel` floats over the map, where the page's own ground would
 * show the map through it; `ghost` sits inside something that already has a
 * ground and shows itself by filling on hover.
 *
 * Both tones follow the theme without a `dark:` utility anywhere: the tokens
 * themselves are what a theme swaps.
 *
 * `danger` and `warning` are tinted rather than filled, and take the two tones
 * this application already says things in — `--alert` for what cannot be undone,
 * `--hold` for what is waiting on somebody. A tone is a claim about the action,
 * not decoration: reach for `danger` where a press destroys something, and let
 * everything else stay `standard`, or the two stop meaning anything.
 */
export type ButtonVariant = "primary" | "standard" | "panel" | "ghost" | "danger" | "warning";

const PRIMITIVE_VARIANT = {
  primary: "default",
  standard: "outline",
  panel: "outline",
  ghost: "ghost",
  danger: "destructive",
  warning: "ghost",
} as const;

/**
 * The map's ground, not the page's, and quiet until pointed at. `active` is the
 * accent edge a trigger wears while the thing it opens is holding something —
 * what the mark says to anyone who can see it, which is why every caller that
 * sets it also says so in the button's name.
 */
const PANEL =
  "border-[var(--rule)] bg-[var(--panel)] text-[var(--ink-2)] shadow-[var(--shadow)] hover:bg-[var(--panel)] hover:text-[var(--ink)]";
const PANEL_ACTIVE = "border-[var(--accent)]";

/**
 * The shape the primitive's `destructive` already has, in this application's
 * waiting tone. `hover:text` is restated because the ghost it builds on would
 * otherwise take the tone away exactly when the pointer is on it.
 */
const WARNING =
  "bg-[var(--hold)]/10 text-[var(--hold)] hover:bg-[var(--hold)]/20 hover:text-[var(--hold)]";

export interface ButtonStyleProps {
  variant?: ButtonVariant;
  /**
   * A decorative mark, before whatever the control says.
   * `ExternalButtonLink` defaults this to an outbound arrow — that a link
   * leaves is a property of the link, not of the sentence each caller writes
   * around it — and `icon={null}` drops it. Hidden from assistive
   * technology here rather than at every call site, so it is the wrong slot for
   * anything that has something of its own to announce — a `Spinner` carrying a
   * label belongs in the children, beside the text it is standing in for.
   *
   * The glyph is drawn at 16 pixels whatever size it asks for: the primitive
   * sizes any `svg` it contains, and CSS beats the attribute.
   */
  icon?: ReactNode;
  /** `panel` only: the control this opens is holding something. */
  active?: boolean;
}

/**
 * A button with nothing to say is the square; one with a label is not. There is
 * no third shape, so the size follows from the children rather than from a prop
 * a caller has to keep consistent with them.
 */
function classes(
  variant: ButtonVariant = "standard",
  children: ReactNode,
  active = false,
  className?: string,
) {
  return cn(
    buttonVariants({
      variant: PRIMITIVE_VARIANT[variant],
      size: children == null || children === false ? "icon" : "default",
    }),
    variant === "panel" && PANEL,
    variant === "panel" && active && PANEL_ACTIVE,
    variant === "warning" && WARNING,
    className,
  );
}

/** A glyph, hidden from anything that reads rather than looks. */
function mark(icon: ReactNode) {
  if (!icon) {
    return null;
  }

  return (
    // `contents` so the glyph stays a flex child of the button and keeps the
    // gap the primitive puts between a mark and a label.
    <span aria-hidden="true" className="contents">
      {icon}
    </span>
  );
}

/** The mark and the label, in that order, however many of the two there are. */
function content(icon: ReactNode, children: ReactNode) {
  return (
    <>
      {mark(icon)}
      {children}
    </>
  );
}

export interface ButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "type">,
    ButtonStyleProps {
  children?: ReactNode;
}

export function Button({ variant, icon, active, className, children, ...props }: ButtonProps) {
  return (
    <button type="button" className={classes(variant, children, active, className)} {...props}>
      {content(icon, children)}
    </button>
  );
}

export interface ButtonLinkProps extends LinkProps, ButtonStyleProps {}

/**
 * Somewhere else in this application, and unmarked unless the caller says
 * otherwise. There is no glyph for "a different page here" that a directional
 * arrow does not overstate — leading the label, it points away from the words
 * it introduces — and what a link is about is something its caller knows and
 * this does not. `ExternalButtonLink` is the one that marks itself, because
 * leaving is a fact about the link rather than about its destination.
 */
export function ButtonLink({
  variant,
  icon,
  active,
  className,
  children,
  ...props
}: ButtonLinkProps) {
  return (
    <Link className={classes(variant, children, active, className)} {...props}>
      {content(icon, children)}
    </Link>
  );
}

export interface ExternalButtonLinkProps
  extends AnchorHTMLAttributes<HTMLAnchorElement>,
    ButtonStyleProps {}

/**
 * Somewhere that is not this application, which is the part a reader deserves
 * to know before they follow it rather than after — so the outbound mark is the
 * default here, not something each caller remembers to add, and it leads the
 * label rather than trailing it, where it is read before the words are.
 */
export function ExternalButtonLink({
  variant,
  icon = <IconExternalLink stroke={1.6} />,
  active,
  className,
  children,
  ...props
}: ExternalButtonLinkProps) {
  return (
    <a className={classes(variant, children, active, className)} {...props}>
      {content(icon, children)}
    </a>
  );
}
