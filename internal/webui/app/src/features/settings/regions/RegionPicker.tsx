/**
 * The regions the surface index is built from: the chosen ones as chips, and a
 * search box that adds another.
 *
 * The nearest thing to the textarea it would replace: a list of regions, added
 * and removed one at a time, over the whole of Geofabrik. What it adds is that
 * the name is chosen from the catalogue rather than typed, and that every row
 * carries its size.
 *
 * The matches are a plain listbox under the input rather than a popover. A
 * popover would take focus off the input on every keystroke, and the list is
 * part of the field here, not a menu over the page.
 */

import { IconSearch, IconX } from "@tabler/icons-react";
import { useId, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { covers, deselect, formatBytes, region, search, select } from "./model";
import { Size, Summary } from "./Summary";

export function RegionPicker({
  value,
  onChange,
}: {
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const id = useId();
  const [query, setQuery] = useState("");
  // Neither what is already chosen, nor what a chosen region already contains:
  // picking a state out of a selected country would drop the country, which is
  // not what reaching for a name in a list looks like it will do.
  const matches = search(query).filter(
    (match) => !value.includes(match.slug) && !value.some((held) => covers(held, match.slug)),
  );

  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <FieldLabel htmlFor={`${id}-search`}>Regions to index</FieldLabel>
        {value.length > 0 ? (
          <ul className="flex flex-wrap gap-2" aria-label="Selected regions">
            {value.map((slug) => (
              <li key={slug}>
                <Badge variant="secondary" className="h-7 gap-1.5 pr-1 pl-2.5">
                  <span className="font-mono text-xs">{slug}</span>
                  <span className="text-[var(--ink-2)]">
                    {formatBytes(region(slug)?.bytes ?? null)}
                  </span>
                  <button
                    type="button"
                    aria-label={`Remove ${slug}`}
                    className="grid size-5 place-items-center rounded-full hover:bg-[var(--rule)]"
                    onClick={() => onChange(deselect(value, slug))}
                  >
                    <IconX className="size-3" />
                  </button>
                </Badge>
              </li>
            ))}
          </ul>
        ) : null}
        <div className="relative">
          <IconSearch className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-[var(--ink-2)]" />
          <Input
            id={`${id}-search`}
            className="pl-9"
            placeholder="Search Geofabrik — Bayern, Rheinland-Pfalz, France…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        <FieldDescription>
          Germany and its states are offered first; anywhere Geofabrik publishes is a search away.
        </FieldDescription>
      </div>

      <ul
        aria-label="Matching regions"
        className="max-h-64 overflow-y-auto rounded-lg border border-[var(--rule)]"
      >
        {matches.length === 0 ? (
          <li className="px-3 py-2 text-sm text-[var(--ink-2)]">
            Nothing published under that name.
          </li>
        ) : (
          matches.map((match) => (
            <li key={match.slug} className="border-[var(--rule)] border-b last:border-b-0">
              <button
                type="button"
                className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-[var(--base)]"
                onClick={() => {
                  onChange(select(value, match.slug));
                  setQuery("");
                }}
              >
                <span className="grid">
                  <span className="text-sm">{match.name}</span>
                  <span className="font-mono text-[var(--ink-2)] text-xs">{match.slug}</span>
                </span>
                <Size bytes={match.bytes} />
              </button>
            </li>
          ))
        )}
      </ul>

      <Summary value={value} />
    </div>
  );
}
