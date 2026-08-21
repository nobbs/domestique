/**
 * The profile, floating across the foot of the map.
 *
 * It starts where the left-hand panel ends, so the two never overlap and the
 * ground between them stays map. The chart inside is the same instrument it has
 * always been — scrub, drag to zoom, arrow keys, readout — and this only puts a
 * frame and a header around it.
 *
 * The header collapses the panel to a pill in the same corner. Collapsed, the
 * pill still carries the two figures the chart existed to give at a glance: how
 * much climbing there is, and between which heights. That choice lives for as
 * long as the tab does and is deliberately not stored — a reader who put the
 * chart away one evening should not have to remember they did so.
 */

import { ElevationProfile } from "../../components/ElevationProfile";
import { formatAscent, formatElevation } from "../../lib/format";
import type { Highlight } from "../../lib/highlight";
import type { DistanceWindow, Profile } from "../../lib/profile";
import type { SurfaceSummary } from "../../lib/surface";

export interface ElevationPanelProps {
  /** The stretch on show: the whole route, or the window the reader chose. */
  profile: Profile | null;
  title: string;
  /** How much climbing the whole route has, for the collapsed pill. */
  ascentMetres: number;
  surface: SurfaceSummary | null;
  activeMetres: number | null;
  onActiveChange: (metres: number | null) => void;
  zoomWindow: DistanceWindow | null;
  onZoomChange: (window: DistanceWindow | null) => void;
  highlight: Highlight | null;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
}

export function ElevationPanel({
  profile,
  title,
  ascentMetres,
  surface,
  activeMetres,
  onActiveChange,
  zoomWindow,
  onZoomChange,
  highlight,
  collapsed,
  onCollapsedChange,
}: ElevationPanelProps) {
  const range = profile
    ? `${formatElevation(profile.minElevationMetres)}–${formatElevation(profile.maxElevationMetres)}`
    : "";
  // Collapsed, the pill is the summary; open, the same line is the hint that
  // says what the chart will do if it is dragged across.
  const summary = collapsed
    ? [formatAscent(ascentMetres), range].filter(Boolean).join(" · ")
    : zoomWindow
      ? `${(zoomWindow.startMetres / 1000).toFixed(1)}–${(zoomWindow.endMetres / 1000).toFixed(1)} km shown · Escape returns`
      : range === ""
        ? ""
        : `${range} · drag across to look closer`;

  return (
    <section
      className="panel elevation-panel"
      data-collapsed={collapsed}
      aria-labelledby="elevation-heading"
    >
      <div className="elevation-panel__header">
        <h2 id="elevation-heading">Elevation</h2>
        {summary === "" ? null : <span className="elevation-panel__summary">{summary}</span>}
        <button
          className="elevation-panel__collapse"
          type="button"
          aria-expanded={!collapsed}
          // Only while there is a plot to point at: the chart is unmounted when
          // the panel is a pill, and a control naming an element that is not in
          // the document is a dangling reference a screen reader cannot follow.
          {...(collapsed ? {} : { "aria-controls": "elevation-plot" })}
          onClick={() => onCollapsedChange(!collapsed)}
        >
          {collapsed ? "Show the profile" : "Hide the profile"}
        </button>
      </div>
      {/*
       * Unmounted rather than hidden when the panel is a pill: the chart holds
       * a pointer listener over its whole plot area, and a plot nobody can see
       * must not be a plot a stray drag can still select a stretch of.
       */}
      {collapsed ? null : (
        <div id="elevation-plot">
          <ElevationProfile
            profile={profile}
            title={title}
            surface={surface}
            activeMetres={activeMetres}
            onActiveChange={onActiveChange}
            zoomWindow={zoomWindow}
            onZoomChange={onZoomChange}
            highlight={highlight}
          />
        </div>
      )}
    </section>
  );
}
