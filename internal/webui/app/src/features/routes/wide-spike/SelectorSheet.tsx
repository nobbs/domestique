/**
 * **1 — Selector.** Komoot's arrangement: figures, one instrument, a chooser.
 *
 * The reference, and the control for this spike. The sheet holds a single
 * chart area and a control that decides what is in it, so the panel stays
 * about two hundred pixels tall whatever the route carries and nothing has to
 * be given up as more instruments arrive — a fourth is a fourth button.
 *
 * What it cannot do is put two things on the same axis. "The rain arrives on
 * the second col" is a fact about the elevation profile *and* the forecast,
 * and this is the one alternative here in which those two are never on screen
 * together. Whether that matters is the question the spike exists to answer.
 */

import { IconInfoCircle } from "@tabler/icons-react";
import { useState } from "react";
import { ElevationProfile } from "../../../components/route/ElevationProfile";
import { FilmstripBand } from "../../../components/route/forecast-spike/FilmstripBand";
import { PADDING } from "../../../lib/plotAxis";
import { useElementWidth } from "../../../lib/useElementWidth";
import {
  bandEntries,
  gradientSegments,
  groundSegments,
  Ribbon,
  surfaceEntries,
} from "../panel-spike/shared";
import { ClassList } from "./ClassList";
import type { SheetProps } from "./shared";
import { RideWindow, Sheet } from "./shared";

const VIEWS = [
  { key: "elevation", label: "Elevation" },
  { key: "weather", label: "Weather" },
  { key: "ground", label: "Ground" },
] as const;

type View = (typeof VIEWS)[number]["key"];

export function SelectorSheet({
  route,
  profile,
  surface,
  runs,
  bands,
  cells,
  samples,
  startAt,
  activeMetres,
  onActiveChange,
  highlight,
  onHighlightChange,
  unitSystem,
}: SheetProps) {
  const [view, setView] = useState<View>("elevation");
  const { ref, width } = useElementWidth<HTMLDivElement>();

  return (
    <Sheet>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <RideWindow startAt={startAt} samples={samples} />
        <div className="flex items-center gap-2">
          <div className="flex rounded-lg bg-[var(--base)] p-0.5">
            {VIEWS.map((candidate) => (
              <button
                key={candidate.key}
                type="button"
                aria-pressed={view === candidate.key}
                onClick={() => setView(candidate.key)}
                className={`rounded-md px-2.5 py-1 text-xs focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--accent)] ${
                  view === candidate.key
                    ? "bg-[var(--panel)] font-semibold shadow-[var(--shadow)]"
                    : "text-[var(--ink-2)]"
                }`}
              >
                {candidate.label}
              </button>
            ))}
          </div>
          <IconInfoCircle size={16} stroke={2} className="text-[var(--ink-2)]" aria-hidden="true" />
        </div>
      </div>
      <div ref={ref}>
        {view === "elevation" ? (
          <ElevationProfile
            profile={profile}
            title={route.title}
            surface={surface}
            activeMetres={activeMetres}
            onActiveChange={onActiveChange}
            highlight={highlight}
            unitSystem={unitSystem}
          />
        ) : null}
        {view === "weather" ? (
          <FilmstripBand
            cells={cells}
            width={width}
            startMetres={0}
            endMetres={route.distanceMetres}
            unitSystem={unitSystem}
          />
        ) : null}
        {view === "ground" ? (
          <div
            className="grid gap-2"
            style={{ paddingLeft: PADDING.left, paddingRight: PADDING.right }}
          >
            <Ribbon segments={gradientSegments(runs)} className="h-5" highlight={highlight} />
            <Ribbon segments={groundSegments(surface)} className="h-3" highlight={highlight} />
            <ClassList
              name="Gradient"
              entries={bandEntries(bands, route.distanceMetres)}
              absence="No elevation data."
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
            <ClassList
              name="Surface"
              entries={surfaceEntries(surface)}
              absence="Surface not classified yet."
              highlight={highlight}
              onHighlightChange={onHighlightChange}
            />
          </div>
        ) : null}
      </div>
    </Sheet>
  );
}
