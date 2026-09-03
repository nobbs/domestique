/**
 * The registry of forecast measures the conditions overlay can wash the route
 * in: wind, temperature, rain, cloud. One entry owns everything a picker, a
 * legend, and a map layer need for that measure — reading it out of a
 * `WeatherPoint`, banding it, colouring the band, and putting it in words —
 * so those callers iterate `MEASURES` instead of each rebuilding the mapping.
 *
 * Every measure is banded rather than drawn continuously. It matches
 * `GRADIENT_BANDS` and `TEMPERATURE_FLOORS`, gives the words a vocabulary
 * instead of a decimal, and bands onto a MapLibre `step` expression cleanly
 * once a map layer reads this table. Every cut below is chosen for what
 * changes for a cyclist, not for round numbers.
 *
 * Colour lives here rather than in `cartography.ts`, following `bandColour`
 * in `profile.ts` and `surfaceColour` in `surface.ts`: `cartography.ts` holds
 * only map chrome, not domain palettes. MapLibre paint properties cannot
 * resolve a CSS custom property, so — exactly as `cartography.ts` explains for
 * the route's own colours — the rain and cloud ramps are hex here and mirrored
 * as custom properties in index.css, held equal by `cartography.test.ts`.
 * `--temp-0..4` already existed on the CSS side with no TS mirror; the hex
 * table here closes that gap too, reusing `temperatureBand` from `weather.ts`
 * so the boundary lives in one place.
 */

import type { WeatherPoint } from "../api/types";
import type { UnitSystem } from "./units";
import {
  precipitationUnitLabel,
  precipitationValue,
  speedUnitLabel,
  speedValue,
  temperatureUnitLabel,
  temperatureValue,
} from "./units";
import { temperatureBand } from "./weather";

export type MeasureKey = "wind" | "temperature" | "rain" | "cloud";

/** One band's name and what it means, for a legend or a screen reader. */
export interface MeasureBand {
  label: string;
  description: string;
}

/** An internal band cut: `limit` is the ascending exclusive ceiling; the last band has none. */
interface BandCut extends MeasureBand {
  limit: number;
}

/** The band a value falls in, 0 gentlest, from a table of ascending cuts. */
function bandFor(cuts: readonly BandCut[], value: number): number {
  const index = cuts.findIndex((cut) => value < cut.limit);

  return index === -1 ? cuts.length - 1 : index;
}

/**
 * A hex ramp for one measure, per basemap.
 *
 * The same shape `SurfaceColours` uses in `surface.ts`: MapLibre paints on a
 * canvas and cannot resolve a custom property, so both sides are carried as
 * plain hex and held equal to index.css by `cartography.test.ts` instead.
 */
interface MeasureRamp {
  light: readonly string[];
  dark: readonly string[];
}

function rampColour(ramp: MeasureRamp, band: number, dark: boolean): string {
  const scale = dark ? ramp.dark : ramp.light;

  return scale[band] ?? scale[0] ?? "#000000";
}

/**
 * Mirrors `--temp-0..4` from index.css exactly — take the values from there,
 * never re-pick them, or the two sides drift and `cartography.test.ts` fails.
 */
const TEMPERATURE_COLOURS: MeasureRamp = {
  light: ["#2e5f9e", "#5aa2cf", "#cbb579", "#e08a3c", "#c9453a"],
  dark: ["#6f9fd8", "#8ec8e8", "#e3d199", "#f2a961", "#ef7266"],
};

/**
 * Indigo, kept clearly toward violet from the route's own azure accent
 * (`ROUTE_ACCENT`) so a rain wash under the route line is never read as the
 * route itself.
 */
const RAIN_COLOURS: MeasureRamp = {
  light: ["#c7c2f2", "#8477d9", "#5b4bc4", "#3a2f96"],
  dark: ["#4c4570", "#6d5fc4", "#9385e2", "#bcb0fa"],
};

/**
 * A cool ramp of its own rather than the gradient scale: steepness is edged on
 * the route in `bandColour` at the same time a wash is drawn, and one palette
 * carrying two meanings at once is what the casing in `RouteOverlay` exists to
 * prevent.
 */
const WIND_COLOURS: MeasureRamp = {
  light: ["#cfe3ea", "#86bdd0", "#3f8fac", "#1c5f78"],
  dark: ["#2b4a57", "#4b8298", "#7cb6cd", "#b3dced"],
};

/**
 * A neutral ramp, so it darkens against a light basemap and lightens against
 * a dark one rather than favouring either — the same reason `SURFACE_STYLES`
 * carries a light/dark pair for every class instead of one grey.
 */
const CLOUD_COLOURS: MeasureRamp = {
  light: ["#dfe1e4", "#aeb3b8", "#7d838a", "#4d5257"],
  dark: ["#3d4044", "#6e7378", "#9aa0a6", "#c7ccd1"],
};

/**
 * Cut where the jersey changes, the same reasoning `TEMPERATURE_FLOORS`
 * states — the floors themselves stay owned by `weather.ts` so there is one
 * boundary rather than two.
 */
const TEMPERATURE_BANDS: readonly MeasureBand[] = [
  { label: "cold", description: "cold — gilet weather" },
  { label: "cool", description: "cool — long sleeves" },
  { label: "mild", description: "mild, no extra layer needed" },
  { label: "warm", description: "warm" },
  { label: "hot", description: "hot — the ride itself is the challenge" },
];

/**
 * Cut by what a headwind costs a rider rather than by round numbers: barely
 * felt, noticeable, hard work, and the point handling itself gets harder.
 */
const WIND_BANDS: readonly BandCut[] = [
  { limit: 15, label: "light wind", description: "light wind, barely felt" },
  { limit: 30, label: "breezy", description: "breezy — noticeable on the flat" },
  { limit: 45, label: "strong wind", description: "strong wind — hard work into it" },
  {
    limit: Number.POSITIVE_INFINITY,
    label: "gale",
    description: "gale-force — exposed ground needs care",
  },
];

/**
 * The lowest band is dry rather than "no rain" at nought, so a trace the
 * forecast reports but a rider would never feel does not light the corridor.
 */
const RAIN_BANDS: readonly BandCut[] = [
  { limit: 0.2, label: "dry", description: "essentially dry" },
  { limit: 2, label: "light rain", description: "light rain — road spray, still rideable" },
  { limit: 6, label: "moderate rain", description: "moderate rain — kit soaks through" },
  {
    limit: Number.POSITIVE_INFINITY,
    label: "heavy rain",
    description: "heavy rain — standing water, poor grip",
  },
];

/** Cut close to the okta scale a forecaster already reads cloud cover on. */
const CLOUD_BANDS: readonly BandCut[] = [
  { limit: 20, label: "clear", description: "clear skies" },
  { limit: 50, label: "few clouds", description: "a few clouds" },
  { limit: 85, label: "cloudy", description: "mostly cloudy" },
  { limit: Number.POSITIVE_INFINITY, label: "overcast", description: "overcast" },
];

function temperatureWords(celsius: number, system: UnitSystem): string {
  const band = TEMPERATURE_BANDS[temperatureBand(celsius)];
  const value = Math.round(temperatureValue(celsius, system));

  return `${band?.label}, ${value}${temperatureUnitLabel(system)}`;
}

function windWords(kmh: number, system: UnitSystem): string {
  const band = WIND_BANDS[bandFor(WIND_BANDS, kmh)];
  const value = Math.round(speedValue(kmh, system));

  return `${band?.label}, ${value} ${speedUnitLabel(system)}`;
}

function rainWords(millimetres: number, system: UnitSystem): string {
  const band = RAIN_BANDS[bandFor(RAIN_BANDS, millimetres)];
  const value = precipitationValue(millimetres, system);
  const decimals = system === "imperial" ? 2 : 1;

  return `${band?.label}, ${value.toFixed(decimals)} ${precipitationUnitLabel(system)}`;
}

/** Cloud cover is a share, so it reads the same regardless of `system`; the parameter stays for the shape every measure's `words` shares. */
function cloudWords(percent: number, _system: UnitSystem): string {
  const band = CLOUD_BANDS[bandFor(CLOUD_BANDS, percent)];

  return `${band?.label}, ${Math.round(percent)}%`;
}

/**
 * One forecast measure the overlay can wash the route in: how to read it off
 * a `WeatherPoint`, band it, colour the band, and put it in words.
 *
 * `kind` is `"vector"` only for wind — it also carries a direction
 * (`WeatherPoint.windDirectionDegrees`), which a map layer reads directly
 * off the point rather than through this registry; everything here still
 * banded and worded from wind *speed* alone, the same as any scalar measure.
 */
export interface Measure {
  key: MeasureKey;
  label: string;
  kind: "scalar" | "vector";
  /** The bands this measure has, gentlest first, for a legend to list. */
  bands: readonly MeasureBand[];
  /** Pulls this measure's reading out of one forecast point. */
  reading: (point: WeatherPoint) => number;
  /** Which band a reading falls in, indexing `bands`. */
  band: (value: number) => number;
  /** The band's colour on the cartography actually loaded. */
  colour: (band: number, dark: boolean) => string;
  /**
   * The band's opacity on the cartography, 0 to 1. Rain and cloud's lowest
   * band is 0: most of most rides are dry under a plain sky, and washing the
   * corridor to say so is noise — the same refusal `RouteOverlay.tsx` makes
   * for gentle gradient, left to the plain casing rather than edged in gold.
   */
  opacity: (band: number) => number;
  /** The reading in words, in the reader's chosen units. */
  words: (value: number, system: UnitSystem) => string;
}

const ALWAYS_OPAQUE = () => 1;

/** Opaque except a transparent lowest band — rain and cloud's "nothing to say" band. */
const TRANSPARENT_LOWEST = (band: number) => (band === 0 ? 0 : 1);

/**
 * The four measures, in the order a picker or a legend lists them.
 *
 * One source of truth for that order and for everything each measure needs,
 * so the picker, the legend, and the map layer cannot disagree about what
 * "wind" means.
 */
export const MEASURES: readonly Measure[] = [
  {
    key: "wind",
    label: "Wind",
    kind: "vector",
    bands: WIND_BANDS,
    reading: (point) => point.windSpeedKmh,
    band: (value) => bandFor(WIND_BANDS, value),
    colour: (band, dark) => rampColour(WIND_COLOURS, band, dark),
    opacity: ALWAYS_OPAQUE,
    words: windWords,
  },
  {
    key: "temperature",
    label: "Temperature",
    kind: "scalar",
    bands: TEMPERATURE_BANDS,
    // Apparent rather than actual: a rider dresses for what the wind and the
    // air together feel like, not the thermometer's own reading.
    reading: (point) => point.apparentTemperatureCelsius,
    band: temperatureBand,
    colour: (band, dark) => rampColour(TEMPERATURE_COLOURS, band, dark),
    opacity: ALWAYS_OPAQUE,
    words: temperatureWords,
  },
  {
    key: "rain",
    label: "Rain",
    kind: "scalar",
    bands: RAIN_BANDS,
    reading: (point) => point.precipitationMillimetres,
    band: (value) => bandFor(RAIN_BANDS, value),
    colour: (band, dark) => rampColour(RAIN_COLOURS, band, dark),
    opacity: TRANSPARENT_LOWEST,
    words: rainWords,
  },
  {
    key: "cloud",
    label: "Cloud",
    kind: "scalar",
    bands: CLOUD_BANDS,
    reading: (point) => point.cloudCoverPercent,
    band: (value) => bandFor(CLOUD_BANDS, value),
    colour: (band, dark) => rampColour(CLOUD_COLOURS, band, dark),
    opacity: TRANSPARENT_LOWEST,
    words: cloudWords,
  },
];

/** `temperatureColour`'s hex twin, for a MapLibre paint property. */
export function temperatureHexColour(band: number, dark: boolean): string {
  return rampColour(TEMPERATURE_COLOURS, band, dark);
}

/** `WIND_COLOURS`, for `cartography.test.ts` to hold equal to index.css. */
export function windColour(band: number, dark: boolean): string {
  return rampColour(WIND_COLOURS, band, dark);
}

/** `RAIN_COLOURS`, for `cartography.test.ts` to hold equal to index.css. */
export function rainColour(band: number, dark: boolean): string {
  return rampColour(RAIN_COLOURS, band, dark);
}

/** `CLOUD_COLOURS`, for `cartography.test.ts` to hold equal to index.css. */
export function cloudColour(band: number, dark: boolean): string {
  return rampColour(CLOUD_COLOURS, band, dark);
}
