/**
 * Who the gate let through, at the end of the bar.
 *
 * There is one authorised address, so this is not here to tell a reader who
 * they are — they know. It is here because the identity is the whole of the
 * authentication: a page that draws itself is a page whose assertion was
 * accepted, and naming the address that was accepted is what turns that into
 * something a reader can check. Two Access sessions in two browsers, or a
 * development proxy holding an assertion minted for somebody else, are
 * indistinguishable until something says which one answered.
 *
 * A mark rather than the address, because the bar's job is navigation and an
 * email is the longest thing that could stand in it. The address is the mark's
 * name, so a pointer and a screen reader both get it without opening anything,
 * and the menu spells it out for everyone else.
 */

import { IconLogout } from "@tabler/icons-react";
import { useQuery } from "@tanstack/react-query";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { webUIConfigQuery } from "../api/queries";
import { Button, ExternalButtonLink } from "./Button";

/**
 * One or two letters standing in for an address.
 *
 * The local part only: every address a deployment ever shows shares its domain
 * with itself, so the half that could tell two of them apart is the half in
 * front of the `@`. A separator inside it usually parts a given name from a
 * family one, and where it does the two initials are worth more than the first
 * two letters of the first name.
 */
export function initialsOf(email: string): string {
  const local = email.trim().split("@")[0] ?? "";
  const parts = local.split(/[.\-_+]+/).filter(Boolean);
  const letters = parts.length > 1 ? [parts[0], parts[1]] : [parts[0]];

  return letters
    .map((part) => part?.[0] ?? "")
    .join("")
    .toUpperCase();
}

export function UserPill() {
  const { data } = useQuery(webUIConfigQuery());
  const identity = data?.identity;

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
        // What it is is the session, and whose it is is the address, so both
        // are said here rather than left to the two letters to imply.
        aria-label={`Signed in as ${identity.email}`}
        render={
          <Button
            className="size-8 shrink-0 rounded-full bg-[var(--base)] p-0 text-xs font-semibold tracking-tight text-[var(--ink-2)] hover:text-[var(--ink)]"
            variant="ghost"
          >
            {initialsOf(identity.email)}
          </Button>
        }
        title={identity.email}
      />
      <PopoverContent
        align="end"
        aria-label="Session"
        className="w-auto max-w-[min(20rem,calc(100dvw-1.5rem))] gap-2 bg-[var(--panel)] p-2 shadow-[var(--shadow)]"
        side="bottom"
      >
        {/* Breaks anywhere: an address is one word to a browser, and a long one
            would otherwise decide how wide this popover is. */}
        <p className="wrap-anywhere px-1.5 text-sm text-[var(--ink)]">{identity.email}</p>
        {/*
         * Absent, not disabled, where the service named no way out. A disabled
         * control says "you cannot do this now"; there is nothing here that
         * could ever serve it, and offering it greyed out would describe a
         * deployment this is not.
         */}
        {identity.signOutUrl ? (
          <ExternalButtonLink
            className="w-full justify-start"
            href={identity.signOutUrl}
            icon={<IconLogout stroke={1.6} />}
            variant="ghost"
          >
            Sign out
          </ExternalButtonLink>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
