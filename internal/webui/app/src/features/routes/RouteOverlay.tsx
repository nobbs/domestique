/**
 * The selected route, drawn over the library map.
 *
 * Not a map of its own. The entry page has one MapLibre instance for the whole
 * library, and this is the stack of sources and layers that appears over it when
 * a route is picked: the casing, the edging — steepness, or what the wind is
 * doing to the rider once that is asked for — the surface classes, the direction
 * cues, the two ends, and the position the elevation chart shares with it.
 * Mounting a second map for the selected route would download the style again
 * and throw away the ground the reader was already looking at.
 *
 * Everything here is drawn from geometry the service already holds locally and
 * is never sent anywhere; only the basemap underneath comes from outside.
 */

import { useMemo, useState } from "react";
import { Layer, Source } from "react-map-gl/maplibre";
import type { Position, SurfaceRange } from "../../api/types";
import { useCartography } from "../../components/map/CartographyContext";
import { ROUTE_ACCENT, PANEL as ROUTE_CASING } from "../../lib/cartography";
import type { ForecastSample } from "../../lib/forecastSamples";
import type { Highlight } from "../../lib/highlight";
import { highlightRanges, litRanges } from "../../lib/highlight";
import type { MeasureKey } from "../../lib/measures";
import type { DistanceWindow, Profile } from "../../lib/profile";
import { bandColour, coordinateRange, sampleAt } from "../../lib/profile";
import { cuesDescription, routeCues } from "../../lib/routeCues";
import { gradientSlices, routeLinesWithin } from "../../lib/routeLines";
import type { SurfaceSummary } from "../../lib/surface";
import { SURFACE_LINE_WIDTH, surfaceColour, surfaceLinesWithin } from "../../lib/surface";
import { useEscapeKey } from "../../lib/useEscapeKey";
import { ConditionsWash } from "./ConditionsWash";
import { DirectionCues } from "./DirectionCues";
import { HoverLink } from "./HoverLink";
import { PositionTooltip } from "./PositionTooltip";
import { RouteTerminal } from "./RouteTerminal";
import { dimmedOutside, EDGING_WIDTH, taggedCollection } from "./routeFeatures";
import { SelectionLink } from "./SelectionLink";
import { WindDriftField } from "./WindDriftField";
import { useWindRuns, WindRelationTint } from "./WindRelationTint";

const SOURCE_ID = "route-geometry";

/** The lowest layer the route itself draws, and so what the wash goes under. */
const HALO_LAYER_ID = "route-window-halo";

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

export interface RouteOverlayProps {
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
  /**
   * The forecast measure the route is washed in, and null — the default — for
   * none. Read from the same samples above; nothing is drawn without both.
   */
  measure?: MeasureKey | null;
}

export function RouteOverlay({
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
  measure = null,
}: RouteOverlayProps) {
  // Picks the steepness ramp: the edging has to match the ground it is drawn
  // on, which is the loaded basemap rather than the page's scheme.
  const { dark: darkBasemap } = useCartography();
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

  // Read here rather than inside the layer that draws it: the steepness edging
  // below has to know whether the wind has taken the slot.
  const windRuns = useWindRuns(samples, coordinates, measure === "wind");
  const windTinted = windRuns.length > 0;

  // One feature collection per band, for the same reason the classes get one
  // each: the width and colour of an edging belong to its layer. Always one per
  // drawn band, empty where the route has no such ground, so the layers stay
  // mounted and the stack keeps the order it was built in — and empty
  // throughout while the wind has the slot, since two ramps along one line
  // leave a reader guessing which of them a colour belongs to.
  const gradientFeatures = useMemo(() => {
    const slices = windTinted ? [] : gradientSlices(coordinates, lit);

    return GRADIENT_BANDS_DRAWN.map((band) => ({
      band,
      data: taggedCollection(
        slices.find((entry) => entry.band === band) ?? { inside: [], outside: [] },
      ),
    }));
  }, [coordinates, lit, windTinted]);

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

  // Whichever profile the chart is actually showing, when the position falls
  // inside it: a windowed profile resamples at a different density, so this is
  // what makes the tooltip agree with the readout exactly. Outside a zoom window
  // it falls back to the whole-route sample.
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
              "line-width": EDGING_WIDTH,
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
          id={HALO_LAYER_ID}
          type="line"
          beforeId={`route-gradient-${LOWEST_BAND_DRAWN}-line`}
          layout={{ "line-cap": "round", "line-join": "round" }}
          paint={{
            "line-color": accent,
            // Wider than the edging it sits under, so the glow shows either
            // side of it rather than only through it.
            "line-width": EDGING_WIDTH + 6,
            "line-opacity": 0.22,
            "line-blur": 3,
          }}
        />
      </Source>
      {/*
       * The forecast wash, under the halo and so under everything: it is a
       * broad field the route is read against, and a route drawn on top of its
       * own weather is still a route. Mounted after the layer it names, for the
       * same reason the halo is mounted after the band it names.
       */}
      <ConditionsWash
        coordinates={coordinates}
        samples={samples}
        measure={measure}
        beforeId={HALO_LAYER_ID}
      />
      {/*
       * Which way the air is going, drifting through the corridor it was washed
       * in: mounted after the wash and named against the same layer, so it goes
       * immediately above the corridor and still under everything the route
       * itself draws. It is the only mark on this map drawn frame by frame.
       */}
      <WindDriftField
        coordinates={coordinates}
        samples={samples}
        measure={measure}
        beforeId={HALO_LAYER_ID}
      />
      {/*
       * The wind on the rider, in the edging slot the steepness bands give up
       * for it: under the casing, so it never touches the class colour above.
       * Named against the casing rather than a band, because the bands are
       * drawing nothing at all for as long as this is here.
       */}
      <WindRelationTint
        runs={windRuns}
        coordinates={coordinates}
        lit={lit}
        beforeId="route-casing"
      />
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
      <DirectionCues coordinates={coordinates} />
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
        />
      ) : null}
      {/*
       * The cues in words. Markers and chevrons are drawn into a WebGL surface
       * that carries no text at all, so this is not a caption repeating what is
       * visible — for a reader who is not looking at the canvas it is the whole
       * of what the cues say.
       */}
      {cues ? <p className="visually-hidden">{cuesDescription(cues)}</p> : null}
    </>
  );
}
