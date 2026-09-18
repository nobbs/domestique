/** Narrowing the library to some of its sources, one chip per source it holds. */

import type { Route } from "../api/types";
import { providerLabel } from "../lib/provider";

const ORDER = ["veloplanner", "komoot", "local"];

export interface LibrarySource {
  provider: string;
  count: number;
}

/** The sources a library holds, in a fixed order, each with its route count. */
export function librarySources(library: Route[]): LibrarySource[] {
  const counts = new Map<string, number>();
  for (const route of library) {
    counts.set(route.provider, (counts.get(route.provider) ?? 0) + 1);
  }
  const rank = (provider: string) =>
    ORDER.includes(provider) ? ORDER.indexOf(provider) : ORDER.length;

  return [...counts]
    .map(([provider, count]) => ({ provider, count }))
    .sort((left, right) => rank(left.provider) - rank(right.provider));
}

/** The plans this service draws itself are named for where they are drawn. */
function sourceLabel(provider: string): string {
  return provider === "local" ? "Planner" : providerLabel(provider);
}

export interface SourceChipsProps {
  sources: LibrarySource[];
  /** The providers kept; empty keeps every source. */
  chosen: string[];
  onChange: (chosen: string[]) => void;
}

export function SourceChips({ sources, chosen, onChange }: SourceChipsProps) {
  return (
    <div role="group" aria-label="Source" className="flex flex-wrap gap-1.5">
      {sources.map(({ provider, count }) => {
        const on = chosen.includes(provider);
        return (
          <button
            key={provider}
            type="button"
            aria-pressed={on}
            aria-label={`${sourceLabel(provider)}, ${count} ${count === 1 ? "route" : "routes"}`}
            onClick={() =>
              onChange(on ? chosen.filter((each) => each !== provider) : [...chosen, provider])
            }
            className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs focus-visible:outline-2 focus-visible:outline-[var(--accent)] focus-visible:outline-offset-2 ${
              on
                ? "bg-[var(--ink)] text-[var(--panel)]"
                : "bg-[color-mix(in_oklab,var(--ink-2)_7%,transparent)] text-[var(--ink)] hover:bg-[color-mix(in_oklab,var(--ink-2)_16%,transparent)]"
            }`}
          >
            {sourceLabel(provider)}
            <span className="tabular-nums opacity-60">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
