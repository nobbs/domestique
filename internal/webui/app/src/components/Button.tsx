import type { AnchorHTMLAttributes, ButtonHTMLAttributes } from "react";
import { Link, type LinkProps } from "react-router";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type ButtonVariant = "primary" | "standard";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "type"> {
  variant?: ButtonVariant;
}

function classes(variant: ButtonVariant, className?: string) {
  return cn(
    buttonVariants({ variant: variant === "primary" ? "default" : "outline", size: "sm" }),
    className,
  );
}

export function Button({ variant = "standard", className, ...props }: ButtonProps) {
  return <button type="button" className={classes(variant, className)} {...props} />;
}

export interface ButtonLinkProps extends LinkProps {
  variant?: ButtonVariant;
}

export function ButtonLink({ variant = "standard", className, ...props }: ButtonLinkProps) {
  return <Link className={classes(variant, className)} {...props} />;
}

export interface ExternalButtonLinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  variant?: ButtonVariant;
}

export function ExternalButtonLink({
  variant = "standard",
  className,
  ...props
}: ExternalButtonLinkProps) {
  return <a className={classes(variant, className)} {...props} />;
}
