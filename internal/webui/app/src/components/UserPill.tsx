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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
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
    <Popover>
      <PopoverTrigger
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
      <PopoverContent
        align="end"
        aria-label="Session"
        className="w-auto max-w-[min(20rem,calc(100dvw-1.5rem))] gap-2 bg-[var(--panel)] p-2 shadow-[var(--shadow)]"
        side="bottom"
      >
        {/* Breaks anywhere: an address is one word to a browser, and a long one
            would otherwise decide how wide this popover is. */}
        <p className="wrap-anywhere px-1.5 text-sm text-[var(--ink)]">{identity.display}</p>
        {/* The raw flag, not `useEffectiveAdmin`: this is the one control that
            must keep showing even after it is switched on, or it could never
            be switched off again. */}
        {identity.admin ? (
          <div className="flex items-center justify-between gap-3 px-1.5 py-1 text-sm text-[var(--ink)]">
            <span>View as rider</span>
            <Switch
              checked={viewAsRider}
              onCheckedChange={setViewAsRider}
              aria-label="View as rider"
            />
          </div>
        ) : null}
        <Button
          className="w-full justify-start"
          icon={<IconLogout stroke={1.6} />}
          onClick={() => {
            void signOut();
          }}
          variant="ghost"
        >
          Sign out
        </Button>
      </PopoverContent>
    </Popover>
  );
}
