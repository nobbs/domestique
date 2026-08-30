/**
 * The two mixes, folded away.
 *
 * They are the same shape of fact as the climbs directly below them — a list
 * the route either has or has not, worth a look before riding and not worth
 * the card's height on every reading — so they fold the same way, and the line
 * that folds them carries the fact a reader would otherwise open them for.
 *
 * Lengths rather than shares, which is a different question from the one the
 * dock's ribbon answers: how much of the ride is gravel, and where the gravel
 * is.
 */

import { IconChevronsRight } from "@tabler/icons-react";
import { useState } from "react";
import { Separator } from "../../components/ui/separator";
import { formatDistance } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import type { MixEntry } from "../../lib/mix";
import type { UnitSystem } from "../../lib/units";
import { MixColumn } from "./MixColumn";

/**
 * The biggest slice of a mix, as the rows themselves say it: what it is, then
 * how much of the route it is.
 *
 * Nothing for a mix the route has none of — a column showing its absence has
 * nothing to summarise, and a summary saying so twice would be the absence
 * stated before the reason for it.
 */
function biggest(entries: MixEntry[], unitSystem: UnitSystem): string | null {
  const most = entries.reduce<MixEntry | null>(
    (worst, entry) => (worst === null || entry.metres > worst.metres ? entry : worst),
    null,
  );
  if (most === null) {
    return null;
  }

  return `${most.label.toLowerCase()} ${formatDistance(most.metres, unitSystem)}`;
}

export function MixSection({
  bands,
  surface,
  surfaceAbsence,
  highlight,
  onHighlightChange,
  unitSystem,
}: {
  /** The steepness bands this route has, gentlest first. */
  bands: MixEntry[];
  /** The surface classes this route has, as the classifier measured them. */
  surface: MixEntry[];
  surfaceAbsence: string;
  highlight: Highlight | null;
  onHighlightChange: (highlight: Highlight | null) => void;
  unitSystem: UnitSystem;
}) {
  const [open, setOpen] = useState(false);

  // What the two columns would say first, on the line that stands in for them.
  const summary = [biggest(bands, unitSystem), biggest(surface, unitSystem)]
    .filter((part) => part !== null)
    .join(", ");

  return (
    <section>
      {/* Divides the card, not the route: this is chrome, not structure. */}
      <Separator className="mb-2" />
      <h3>
        <button
          type="button"
          aria-expanded={open}
          // The visible line is the summary, which says what the route is made
          // of rather than what the control does — so the name says the second.
          aria-label={`${open ? "Hide" : "Show"} gradient and surface`}
          onClick={() => {
            const next = !open;
            setOpen(next);
            // Folding takes the class labels away with it, and a pressed one is
            // the only way to give the whole route back. Rather than leave the
            // map lit with no visible cause, putting the mixes away puts the
            // question away too — which is what collapsing the card itself does.
            if (!next) {
              onHighlightChange(null);
            }
          }}
          className="flex w-full items-center gap-1 text-left text-xs text-[var(--ink-2)] hover:text-[var(--ink)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--accent)]"
        >
          <IconChevronsRight
            size={12}
            stroke={2}
            aria-hidden="true"
            className={
              open ? "shrink-0 rotate-90 transition-transform" : "shrink-0 transition-transform"
            }
          />
          <span className="truncate">
            Gradient and surface{summary === "" ? "" : ` · ${summary}`}
          </span>
        </button>
      </h3>
      {!open ? null : (
        <div className="mt-2 flex items-start gap-3">
          <MixColumn
            name="Gradient"
            classesLabel="Gradient bands"
            entries={bands}
            absence="No elevation data."
            highlight={highlight}
            onHighlightChange={onHighlightChange}
            unitSystem={unitSystem}
          />
          <MixColumn
            name="Surface"
            classesLabel="Surface classes"
            entries={surface}
            absence={surfaceAbsence}
            highlight={highlight}
            onHighlightChange={onHighlightChange}
            unitSystem={unitSystem}
          />
        </div>
      )}
    </section>
  );
}
