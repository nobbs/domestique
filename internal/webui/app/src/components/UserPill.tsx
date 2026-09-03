/**
 * Who the gate let through, at the end of the bar.
 *
 * It is here because the identity is the whole of the authentication: a page
 * that draws itself is a page whose session was accepted, and naming the
 * account that was accepted is what turns that into something a reader can
 * check. Two sessions in two browsers are indistinguishable until something
 * says which one answered.
 *
 * A mark rather than the name, because the bar's job is navigation and an
 * address is the longest thing that could stand in it. The name is the mark's
 * accessible name, so a pointer and a screen reader both get it without
 * opening anything, and the menu spells it out for everyone else.
 */

import { IconLogout } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { webUIConfigQuery } from "../api/queries";
import { useViewAsRider } from "../lib/identity";
import { Button } from "./Button";

/**
 * Ends the session and leaves for the sign-in page regardless of the answer:
 * the cookie is cleared either way, so staying would show a dead session.
 */
async function signOut(): Promise<void> {
  await fetch("/auth/logout", { method: "POST", credentials: "same-origin" }).catch(
    () => undefined,
  );
  window.location.assign("/auth/login");
}

/**
 * One or two letters standing in for a display name.
 *
 * The local part only, where the name is an address: every address a
 * deployment ever shows shares its domain with itself, so the half that could
 * tell two of them apart is the half in front of the `@`. A separator inside
 * it usually parts a given name from a family one, and where it does the two
 * initials are worth more than the first two letters of the first name. The
 * provider may hand over a plain name instead, so a space parts it too.
 */
export function initialsOf(display: string): string {
  const local = display.trim().split("@")[0] ?? "";
  const parts = local.split(/[\s.\-_+]+/).filter(Boolean);
  const letters = parts.length > 1 ? [parts[0], parts[1]] : [parts[0]];

  return letters
    .map((part) => part?.[0] ?? "")
    .join("")
    .toUpperCase();
}

export function UserPill() {
  const { data } = useQuery(webUIConfigQuery());
  const identity = data?.identity;
  const [viewAsRider, setViewAsRider] = useViewAsRider();

  /*
   * Nothing at all until the configuration has arrived, and nothing ever if it
   * does not. An empty circle in the corner would be a claim about the session
   * that no answer has been given for yet.
   */
  if (!identity) {
    return null;
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        // The circle holds initials, which are an abbreviation and not a name.
        // What it is is the session, and whose it is is the account, so both
        // are said here rather than left to the two letters to imply.
        aria-label={`Signed in as ${identity.display}`}
        render={
          <Button
            className="size-8 shrink-0 rounded-full bg-[var(--base)] p-0 text-xs font-semibold tracking-tight text-[var(--ink-2)] hover:text-[var(--ink)]"
            variant="ghost"
          >
            {initialsOf(identity.display)}
          </Button>
        }
        title={identity.display}
      />
      <DropdownMenuContent align="end" className="w-auto max-w-[min(20rem,calc(100dvw-1.5rem))]">
        {/* `GroupLabel` requires a `Group` ancestor, and `wrap-anywhere` keeps a
            long address from deciding how wide this menu is. */}
        <DropdownMenuGroup>
          <DropdownMenuLabel className="wrap-anywhere whitespace-normal">
            {identity.display}
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        {/* The raw flag, not `useEffectiveAdmin`: this is the one control that
            must keep showing even after it is switched on, or it could never
            be switched off again. */}
        {identity.admin ? (
          <>
            <DropdownMenuCheckboxItem checked={viewAsRider} onCheckedChange={setViewAsRider}>
              View as rider
            </DropdownMenuCheckboxItem>
            <DropdownMenuSeparator />
          </>
        ) : null}
        <DropdownMenuItem
          onClick={() => {
            void signOut();
          }}
          variant="destructive"
        >
          <IconLogout stroke={1.6} />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
