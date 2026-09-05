/**
 * What one basemap would look like, drawn in the basemap it is offering.
 *
 * A name is not a picture: "Positron" and "Fiord" tell a reader nothing about
 * the ground they would get. So each choice carries a small map of its own,
 * loaded in that style.
 *
 * Somewhere fixed, and deliberately not where the reader is looking. What a
 * thumbnail has to do is separate five styles from one another, and that needs
 * something they disagree about: over open country — which is most of a route
 * library — every style is one flat patch of colour, and the reader's own view
 * would show five of them. The view below was picked by comparing candidates
 * at this size: a large water body against built ground with roads through it
 * is where cartography differs most, so it is what the tiles show.
 *
 * Each of these is a WebGL context, which browsers ration at around sixteen.
 * They exist only while the list is open, and the list stops drawing them past
 * the handful an operator would plausibly configure — see `BasemapPicker` and
 * the settings page, which draws the strips the same way.
 */

import { Map as MapLibre, useMap } from "react-map-gl/maplibre";
import "maplibre-gl/dist/maplibre-gl.css";
// Configures the shared worker pool; without it this map renders no tiles.
import "../../lib/maplibre";

/**
 * The lake edge at Konstanz: water, built ground and roads in one 64-pixel
 * square. Roads alone leave the light styles looking alike, and green alone
 * leaves all five looking alike; the water is what separates them. Dense enough,
 * too, that the style's own lettering is part of what the tile is showing rather
 * than one cropped name over a flat colour.
 */
const PREVIEW_VIEW = { longitude: 9.1785, latitude: 47.6603, zoom: 12 };

export interface BasemapPreviewProps {
  styleUrl: string;
  /** The one on screen wears the accent edge, as the checked radio does. */
  selected: boolean;
}

export function BasemapPreview({ styleUrl, selected }: BasemapPreviewProps) {
  const { current: map } = useMap();
  const edge = selected ? "ring-[var(--accent)]" : "ring-[var(--rule)]";

  /*
   * Only where a map is already running. This is furniture on a map and never
   * renders anywhere else in the application, so the map beside it is a fair
   * proxy for "this page can draw one at all" — and it keeps a picture of
   * nothing out of the places that render this without one.
   */
  if (!map) {
    return (
      <span aria-hidden="true" className={`size-16 rounded-lg bg-[var(--base)] ring-2 ${edge}`} />
    );
  }

  return (
    <span aria-hidden="true" className={`size-16 overflow-hidden rounded-lg ring-2 ${edge}`}>
      <PreviewMap styleUrl={styleUrl} />
    </span>
  );
}

/**
 * The same view as a wide strip, for the settings page: there is no map beside
 * it there, and the row it sits in already says which basemap it shows.
 */
export function BasemapStrip({
  styleUrl,
  className = "",
}: {
  styleUrl: string;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={`block h-40 overflow-hidden rounded-lg ring-1 ring-[var(--rule)] ${className}`}
    >
      <PreviewMap styleUrl={styleUrl} />
    </span>
  );
}

function PreviewMap({ styleUrl }: { styleUrl: string }) {
  return (
    <MapLibre
      attributionControl={false}
      initialViewState={PREVIEW_VIEW}
      interactive={false}
      mapStyle={styleUrl}
      style={{ width: "100%", height: "100%" }}
    />
  );
}
