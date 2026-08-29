/**
 * The selected route, drawn over the library map.
 *
 * Not a map of its own. The entry page has one MapLibre instance for the whole
 * library, and this is the stack of sources and layers that appears over it when
 * a route is picked: the casing, the steepness edging, the surface classes, the
 * direction cues, the two ends, and the position the elevation chart shares with
 * it. Mounting a second map for the selected route would download the style
 * again and throw away the ground the reader was already looking at.
 *
 * Everything here is drawn from geometry the service already holds locally and
 * is never sent anywhere; only the basemap underneath comes from outside.
 */

import type { DataDrivenPropertyValueSpecification } from "maplibre-gl";
import { useMemo, useState } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import type { Position, SurfaceRange } from "../../api/types";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { Highlight } from "../../lib/highlight";
import { highlightRanges, litRanges } from "../../lib/highlight";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { bandColour, coordinateRange, sampleAt } from "../../lib/profile";
import { cuesDescription, routeCues } from "../../lib/routeCues";
import { gradientSlices, routeLinesWithin } from "../../lib/routeLines";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_LINE_WIDTH, surfaceColour, surfaceLinesWithin } from "../../lib/surface";
import type { UnitSystem } from "../../lib/units";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { DirectionCues } from "./DirectionCues";
import { HoverLink } from "./HoverLink";
import { PositionTooltip } from "./PositionTooltip";
import { RouteTerminal } from "./RouteTerminal";
import { SelectionLink } from "./SelectionLink";

/**
 * The accent the route itself is drawn in, per basemap.
 *
 * Keyed on which basemap is loaded rather than on the system scheme, because
 * this sits on the cartography rather than on the page — see `LoadedBasemap.dark`.
 * The same pair is `--accent` in index.css; both copies must stay in step.
 */
const ROUTE_ACCENT = { light: "#236fc7", dark: "#70adfb" } as const;

/**
 * The casing under the route: the panel colour, so the line reads as lifted off
 * the ground rather than merely recoloured. The same pair is `--panel` in
 * index.css; both copies must stay in step.
 */
const ROUTE_CASING = { light: "#fcfdff", dark: "#24282c" } as const;

const SOURCE_ID = "route-geometry";

/**
 * Credit for the surface classification, which is derived from OpenStreetMap.
 *
 * The basemap's own credit arrives from the style document and covers the tiles
 * only. The classification is a separate derived database under the ODbL, whose
 * share-alike terms oblige this attribution wherever it is shown. Exported
 * because the credit is drawn by the map this stack sits on rather than by the
 * stack itself: it is one line in one corner, and the classification is the
 * reason for half of it.
 */
export const SURFACE_ATTRIBUTION = "Surface data © OpenStreetMap contributors (ODbL)";

/** The casing's normal opacity, kept in one place so the dimmed one follows it. */
const CASING_OPACITY = 0.85;

/**
 * How far the two terminal markers are nudged apart, in screen pixels.
 *
 * A loop and an out-and-back both finish where they started, so both markers
 * would be drawn on the same coordinate and one would simply be missing. The
 * nudge is applied only when the two ends are that close: elsewhere each marker
 * sits exactly on its own point, because a marker is where a ride begins, not a
 * decoration near it. Even nudged, it stays a label for the terminal rather than
 * a survey mark, which is why the words beside the map say plainly that the two
 * ends are the same place.
 */
const TERMINAL_NUDGE_PIXELS = 16;

/**
 * The bands worth ink on a map.
 *
 * Gentle ground is most of most routes, and painting it would edge the whole
 * line in gold to say the unremarkable thing. Left unpainted, the white casing
 * stands for it, and colour appears only where the road starts to bite. The
 * chart still shows every band, which is where a reader goes to compare one
 * stretch against another; the map is answering "where does it get steep?".
 */
const GRADIENT_BANDS_DRAWN = [1, 2, 3, 4] as const;

/** The band whose layer sits lowest, and so the one the halo goes under. */
const LOWEST_BAND_DRAWN = GRADIENT_BANDS_DRAWN[0];

/**
 * How wide the steepness line runs beneath the casing.
 *
 * Wider than the casing, so what shows is an edging either side of it rather
 * than a colour behind it. The casing keeps the two encodings apart: steepness
 * never touches the class colour it would otherwise be read as part of.
 */
const BAND_EDGE_WIDTH = 11;

/**
 * How much of the route survives outside the ground on show.
 *
 * Dimmed rather than hidden: the ride does not stop at the edges of a window,
 * and a route drawn only in the middle would read as a shorter route rather than
 * as a closer look at a longer one. The same holds for a class picked out of the
 * key — the gravel is somewhere on this ride, and hiding the tarmac either side
 * of it would lose where. A quarter is faint enough that the eye lands on the
 * lit ground first and still dark enough to follow the road between.
 */
const OUTSIDE_OPACITY = 0.25;

/**
 * An opacity that drops away outside the ground on show.
 *
 * One expression over one tagged source rather than a second stack of layers:
 * `line-opacity` is a layer property, and one layer per class is exactly what
 * keeps a class's dash pattern identical on both sides of a lit stretch's edge.
 * Written as a function because a bare array in a `const` widens to `unknown[]`
 * and stops matching the tuple union the paint property is typed as.
 */
function dimmedOutside(
  full: number,
  dimmed: boolean,
): DataDrivenPropertyValueSpecification<number> {
  return dimmed ? ["case", ["get", "shown"], full, full * OUTSIDE_OPACITY] : full;
}

/**
 * The route's geometry as at most two features, tagged by whether the chart is
 * showing them, so one paint expression can tell the two apart.
 */
function taggedCollection(slices: { inside: Position[][]; outside: Position[][] }) {
  const features = ([lines, shown]: [Position[][], boolean]) =>
    lines.length === 0
      ? []
      : [
          {
            type: "Feature" as const,
            geometry: { type: "MultiLineString" as const, coordinates: lines },
            properties: { shown },
          },
        ];

  return {
    type: "FeatureCollection" as const,
    features: [...features([slices.inside, true]), ...features([slices.outside, false])],
  };
}

export interface RouteOverlayProps {
  /**
   * Whether `styleUrl` is the dark cartography, which picks the steepness ramp.
   *
   * Passed in rather than read from the system scheme here, because a deployment
   * with no dark style configured keeps the light basemap under a dark scheme,
   * and the edging has to match the ground it is drawn on.
   */
  darkBasemap?: boolean;
  /**
   * The forecast requests for this ride, which the position tooltip reads the
   * wind from. Empty until a start time is picked, which is also what a caller
   * with no forecast to offer passes.
   */
  samples?: ForecastSample[];
  coordinates: Position[];
  /**
   * The ground under the route, addressing `coordinates` by index. Absent leaves
   * the route drawn in the accent, which is what an unclassified stage gets.
   */
  surface?: SurfaceRange[] | undefined;
  /**
   * The same classification, already summarised for naming a position.
   *
   * `surface` above stays index-addressed geometry, which is all painting the
   * line needs; naming the class under the hovered position needs the bands and
   * lookup `summariseSurface` builds. `StageDetail` (the page) has already built
   * exactly that for the profile readout, so it is handed here rather than
   * walked a second time.
   */
  surfaceSummary?: SurfaceSummary | null;
  /**
   * The whole route's profile, never a zoomed one: it is what turns a point on
   * this map into a distance, and it has to answer for the whole route however
   * little of it the chart is showing. Hover detection and the tooltip's
   * distance-to-end both read it for that reason.
   */
  profile?: Profile | null;
  /**
   * The profile the elevation chart is actually drawing right now — the same
   * object `RouteProfile` receives: windowed while the chart is zoomed into a
   * stretch, the whole route otherwise.
   *
   * The tooltip's content (elevation, gradient, band) is read from this in
   * preference to `profile` above, because a windowed profile resamples the
   * stretch on show at the same count `profile` samples the whole route at —
   * finer near a short climb, coarser near a long one — so the two can report
   * a different gradient at the same distance. Reading whichever one the
   * chart is reading is what keeps the tooltip from disagreeing with the
   * readout it stands in for.
   *
   * The position dot never reads this: it tracks `profile` regardless, so
   * hovering the dimmed route outside the zoomed stretch still moves it — a
   * windowed profile has no sample to give there at all. The tooltip falls
   * back to that same whole-route sample outside the window, which the chart
   * has nothing to disagree with in the first place.
   */
  activeProfile?: Profile | null;
  /** Shared with the elevation chart, in metres from the start of the route. */
  activeMetres?: number | null;
  onActiveChange?: (metres: number | null) => void;
  /**
   * Whether the profile card is folded to its row, which unmounts the chart's
   * own `aria-live` readout along with it.
   *
   * The tooltip stays presentational while that readout exists, so a hover is
   * not announced twice. Folded, the readout is gone and the tooltip is the
   * only thing left that could say the position out loud, so it carries the
   * announcement itself instead.
   */
  profileCollapsed?: boolean;
  /**
   * The stretch the chart is showing, lit while the rest of the route dims.
   *
   * Named for what it is rather than `window`, which would shadow the global of
   * that name inside this module. The camera follows it, so the map reads at
   * the scale the chart is being read at; the rest of the route stays drawn,
   * dimmed, so the stretch is still seen as part of a longer ride rather than
   * as a route of its own. A drag along the painted route can set it, through
   * `onZoomChange`; moving the camera never does, which is what keeps a look
   * around at the surrounding roads free.
   */
  zoomWindow?: DistanceWindow | null;
  /**
   * Takes the stretch a drag along the route settled on, and null for the whole
   * route again.
   *
   * Absent leaves the map reading the window and never writing it, which is what
   * every other view of a route wants: a thumbnail or a mini-map is not somewhere
   * a stretch is chosen. Present, the link runs both ways for the window alone —
   * panning and zooming the camera still change nothing but the camera.
   */
  onZoomChange?: ((window: DistanceWindow | null) => void) | undefined;
  /**
   * The class picked out of the key, lit while the rest of the route dims.
   *
   * Scattered along the whole ride rather than one stretch of it, which is the
   * question it answers: where on this route is the gravel, where does it get
   * steep. It narrows a window rather than replacing one — a reader who zooms
   * in on a climb and then asks for the gravel means the gravel on that climb.
   */
  highlight?: Highlight | null;
  /** The units the tooltip and the route's own screen-reader summary report in. */
  unitSystem?: UnitSystem;
}

export function RouteOverlay({
  darkBasemap = false,
  samples = [],
  coordinates,
  surface,
  surfaceSummary = null,
  profile = null,
  activeProfile = null,
  activeMetres = null,
  onActiveChange,
  profileCollapsed = false,
  zoomWindow = null,
  onZoomChange,
  highlight = null,
  unitSystem = "metric",
}: RouteOverlayProps) {
  /**
   * The stretch being drawn on the route right now, which is not a window yet.
   *
   * Kept here rather than handed up to the page: while a hand is still moving,
   * there is nothing to show the chart and nowhere to fly the camera — the
   * answer to a question half asked is to light the ground it covers so far.
   */
  const [pending, setPending] = useState<DistanceWindow | null>(null);

  // Rounded outwards, so the lit stretch covers every metre the chart draws.
  const windowRange = useMemo(
    () =>
      zoomWindow
        ? coordinateRange(coordinates, zoomWindow.startMetres, zoomWindow.endMetres)
        : null,
    [coordinates, zoomWindow],
  );
  // The ground a drag along the route has covered so far, lit exactly as a
  // settled window is. The camera is deliberately left out of it: flying after a
  // hand that is still moving would drag the ground out from under the gesture
  // being drawn on it.
  const pendingRange = useMemo(
    () => (pending ? coordinateRange(coordinates, pending.startMetres, pending.endMetres) : null),
    [coordinates, pending],
  );

  // Both questions come out as the same answer — the stretches of route left
  // lit — so one mask serves every layer, and asking both at once narrows
  // rather than fights.
  const lit = useMemo(
    () =>
      litRanges(
        pendingRange ?? windowRange,
        highlight ? highlightRanges(coordinates, surface ?? [], highlight) : null,
      ),
    [coordinates, highlight, pendingRange, surface, windowRange],
  );
  const dimmed = lit !== null;

  const routeSlices = useMemo(() => routeLinesWithin(coordinates, lit), [coordinates, lit]);
  const route = useMemo(() => taggedCollection(routeSlices), [routeSlices]);

  // One feature per class rather than one with a data-driven colour, so the
  // classes stack in a fixed order and a stretch that carries two of them draws
  // the same way every render.
  const surfaceFeatures = useMemo(
    () =>
      surfaceLinesWithin(coordinates, surface ?? [], lit).map(({ kind, ...slices }) => ({
        kind,
        data: taggedCollection(slices),
      })),
    [coordinates, surface, lit],
  );

  // One feature collection per band, for the same reason the classes get one
  // each: the width and colour of an edging belong to its layer. Always one per
  // drawn band, empty where the route has no such ground, so the layers stay
  // mounted and the stack keeps the order it was built in.
  const gradientFeatures = useMemo(() => {
    const slices = gradientSlices(coordinates, lit);

    return GRADIENT_BANDS_DRAWN.map((band) => ({
      band,
      data: taggedCollection(
        slices.find((entry) => entry.band === band) ?? { inside: [], outside: [] },
      ),
    }));
  }, [coordinates, lit]);

  // The halo marks the ground on show, so it has nothing to draw until
  // something is asked: unqualified, every metre of the route counts as inside.
  const haloRoute = useMemo(
    () => taggedCollection({ inside: dimmed ? routeSlices.inside : [], outside: [] }),
    [routeSlices, dimmed],
  );

  // The dot's own sample: the whole route, in or out of any zoom window. The
  // map keeps drawing the rest of the route, dimmed, outside the window, and
  // a hover there still has ground to answer for — the same reason `profile`
  // (never the windowed one) is what `HoverLink` reads from.
  const activeSample = useMemo(
    () => (profile && activeMetres !== null ? sampleAt(profile, activeMetres) : null),
    [profile, activeMetres],
  );

  // Whatever the chart's own readout would compute for this position: null
  // outside a zoom window, since a windowed profile has no sample to give
  // there at all, in which case the readout's live region announces nothing.
  const windowedSample = useMemo(
    () => (activeProfile && activeMetres !== null ? sampleAt(activeProfile, activeMetres) : null),
    [activeProfile, activeMetres],
  );

  // The tooltip's own content: whichever profile the chart is actually
  // showing, when the position falls inside it — this is what lets the
  // tooltip agree with the readout exactly rather than merely approximately,
  // since a windowed profile resamples its stretch at a different density
  // than the whole route does. Outside a zoom window the chart has nothing to
  // say either, so this falls back to the whole-route sample above, which is
  // still an honest answer for ground the chart is not currently showing.
  const contentSample = windowedSample ?? activeSample;

  // The position shared with the elevation chart. An empty collection keeps the
  // source mounted, so the marker appears without rebuilding the layer.
  const marker = useMemo(
    () => ({
      type: "FeatureCollection" as const,
      features: activeSample
        ? [
            {
              type: "Feature" as const,
              geometry: {
                type: "Point" as const,
                coordinates: [activeSample.longitude, activeSample.latitude],
              },
              properties: {},
            },
          ]
        : [],
    }),
    [activeSample],
  );

  // The terminals and the direction they are ridden in, from the same stored
  // coordinates already drawn. Null for anything that is not a ride — a single
  // point has an end but no direction, and there is nothing honest to draw.
  const cues = useMemo(() => routeCues(coordinates), [coordinates]);
  // Opposite nudges, so a loop shows two markers side by side at the one point
  // it both leaves and returns to.
  const nudge = cues?.sharedTerminal ? TERMINAL_NUDGE_PIXELS : 0;

  /**
   * Escape leaves the stretch on show.
   *
   * The same way back the chart's own control offers, carried here as well
   * because the chart can be collapsed to its pill: a stretch chosen and then
   * put away would otherwise be a view with no way out of it. The page answers
   * Escape too, by dropping the selection — but only when there is no stretch
   * to leave first, so one press never undoes two things.
   *
   * A stretch still being drawn is neither: `selectionGesture` takes Escape in the
   * capture phase and marks it handled, so abandoning a half-drawn window never
   * also throws away the view it was being drawn on.
   */
  useEscapeKey(zoomWindow !== null && onZoomChange !== undefined, () => onZoomChange?.(null));

  // The route sits on the cartography, so it follows the basemap rather than the
  // page: a dark route on a light basemap under a dark system scheme would be a
  // line chosen for a background it is not on.
  const accent = ROUTE_ACCENT[darkBasemap ? "dark" : "light"];

  return (
    <>
      <HoverLink profile={profile} onActiveChange={onActiveChange} />
      <SelectionLink profile={profile} onPending={setPending} onZoomChange={onZoomChange} />
      <Source id={SOURCE_ID} type="geojson" data={route}>
        {/*
         * The casing is drawn from the whole route and stays solid under the
         * classes, so the line the rider follows is continuous even where the
         * class above it is a row of dots.
         */}
        <Layer
          id="route-casing"
          type="line"
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{
            "line-color": ROUTE_CASING[darkBasemap ? "dark" : "light"],
            "line-opacity": dimmedOutside(CASING_OPACITY, dimmed),
            "line-width": 7,
          }}
        />
        {surfaceFeatures.length === 0 ? (
          <Layer
            id="route-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": accent,
              "line-width": SURFACE_LINE_WIDTH,
              "line-opacity": dimmedOutside(1, dimmed),
            }}
          />
        ) : null}
      </Source>
      {/*
       * Steepness, as an edging under the casing: over it, it would be a
       * second line to read along the route, and the point of putting it
       * underneath is that the eye picks up the colour at the edges without
       * ever losing the route itself.
       *
       * Mounted after the casing it names and kept mounted whether or not the
       * band has any ground — an empty collection draws nothing. MapLibre
       * refuses to add a layer before one that does not exist yet, and a band
       * that came and went with the window would be re-added at whatever
       * height the stack happened to have then. Mounting every layer once
       * fixes the order for the life of the map.
       */}
      {gradientFeatures.map(({ band, data }) => (
        <Source key={band} id={`route-gradient-${band}`} type="geojson" data={data}>
          <Layer
            id={`route-gradient-${band}-line`}
            type="line"
            beforeId="route-casing"
            // Butt caps, so a band ends on the point it hands over at rather
            // than half a line width into the band that follows it.
            layout={{ "line-cap": "butt", "line-join": "round" }}
            paint={{
              "line-color": bandColour(band, darkBasemap),
              "line-width": BAND_EDGE_WIDTH,
              "line-opacity": dimmedOutside(1, dimmed),
            }}
          />
        </Source>
      ))}
      {/*
       * A soft halo under the stretch on show, at the bottom of the stack:
       * it is a pointer, and a pointer that tints the two things it points at
       * has changed what they say. Named against the lowest band rather than
       * against the casing for that reason, and mounted last so that layer is
       * there to name.
       */}
      <Source id="route-window" type="geojson" data={haloRoute}>
        <Layer
          id="route-window-halo"
          type="line"
          beforeId={`route-gradient-${LOWEST_BAND_DRAWN}-line`}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{
            "line-color": accent,
            // Wider than the edging it sits under, so the glow shows either
            // side of it rather than only through it.
            "line-width": BAND_EDGE_WIDTH + 6,
            "line-opacity": 0.22,
            "line-blur": 3,
          }}
        />
      </Source>
      {surfaceFeatures.map(({ kind, data }) => (
        <Source key={kind} id={`route-surface-${kind}`} type="geojson" data={data}>
          <Layer
            id={`route-surface-${kind}-line`}
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": surfaceColour(kind, darkBasemap),
              "line-width": SURFACE_LINE_WIDTH,
              "line-opacity": dimmedOutside(1, dimmed),
            }}
          />
        </Source>
      ))}
      <DirectionCues
        coordinates={coordinates}
        darkBasemap={darkBasemap}
        color={ROUTE_CASING[darkBasemap ? "dark" : "light"]}
      />
      {/*
       * The two ends are DOM markers, so their pictograms stay legible at every
       * zoom instead of becoming two nearly identical dots on the canvas.
       */}
      {cues ? (
        <>
          <RouteTerminal kind="start" position={cues.start} offset={-nudge} accent={accent} />
          <RouteTerminal kind="finish" position={cues.finish} offset={nudge} accent={accent} />
        </>
      ) : null}
      <Source id="route-position" type="geojson" data={marker}>
        <Layer
          id="route-position-halo"
          type="circle"
          paint={{ "circle-radius": 8, "circle-color": "#ffffff", "circle-opacity": 0.9 }}
        />
        <Layer
          id="route-position-dot"
          type="circle"
          paint={{ "circle-radius": 5, "circle-color": accent }}
        />
      </Source>
      {activeSample && contentSample && profile ? (
        <PositionTooltip
          position={activeSample}
          content={contentSample}
          endMetres={profile.endMetres}
          surfaceSummary={surfaceSummary}
          coordinates={coordinates}
          samples={samples}
          // The readout is silent whenever this tooltip is standing in for
          // it: the profile card is folded away, or — even open — a windowed
          // chart has no sample to announce for a hover outside its window.
          announce={profileCollapsed || windowedSample === null}
          darkBasemap={darkBasemap}
          unitSystem={unitSystem}
        />
      ) : null}
      {/*
       * The cues in words. Markers and chevrons are drawn into a WebGL surface
       * that carries no text at all, so this is not a caption repeating what is
       * visible — for a reader who is not looking at the canvas it is the whole
       * of what the cues say.
       */}
      {cues ? <p className="visually-hidden">{cuesDescription(cues, unitSystem)}</p> : null}
    </>
  );
}
