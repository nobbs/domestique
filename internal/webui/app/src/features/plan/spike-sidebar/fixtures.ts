/** Scenario data shared by the sidebar spikes — no API calls, no planner state. */

import { formatDuration } from "../../../lib/format";

export interface SpikeWaypoint {
  id: number;
  stop: string;
  name: string;
  meta?: string;
  straight?: boolean;
  terminal?: boolean;
}

export interface SpikeAvoidArea {
  id: number;
  place: string;
  radius: string;
}

export type DeliveryState = "none" | "current" | "pending" | "failed";

export interface Scenario {
  key: string;
  label: string;
  planName: string;
  published: boolean;
  profile: "trekking" | "fastbike" | "gravel";
  cues: boolean;
  turnCount?: number;
  waypoints: SpikeWaypoint[];
  avoid: SpikeAvoidArea[];
  saving: boolean;
  saveError: string | null;
  delivery: { state: DeliveryState; sent: number; total: number };
}

const SIX_STOPS: SpikeWaypoint[] = [
  { id: 0, stop: "Start", name: "Mainzer Hauptbahnhof", terminal: true },
  {
    id: 1,
    stop: "Via 1",
    name: "Georg-Büchner-Straße 42, Mainz",
    meta: `4.8 km · ${formatDuration(13 * 60)}`,
  },
  {
    id: 2,
    stop: "Via 2",
    name: "Rüdesheimer Straße, Mainz",
    meta: `9.1 km · ${formatDuration(24 * 60)}`,
    straight: true,
  },
  { id: 3, stop: "Via 3", name: "Burg Rheinstein", meta: `31.4 km · ${formatDuration(71 * 60)}` },
  {
    id: 4,
    stop: "Via 4",
    name: "Loreley-Felsen, St. Goarshausen",
    meta: `52.7 km · ${formatDuration(118 * 60)}`,
    straight: true,
  },
  {
    id: 5,
    stop: "Finish",
    name: "Bahnhof Bingen (Rhein)",
    meta: `68.2 km · ${formatDuration(152 * 60)}`,
    terminal: true,
  },
];

const THREE_STOPS: SpikeWaypoint[] = [
  { id: 0, stop: "Start", name: "Georg-Büchner-Straße 42, Mainz", terminal: true },
  {
    id: 1,
    stop: "Via 1",
    name: "Kloster Eberbach, Eltville",
    meta: `18.3 km · ${formatDuration(42 * 60)}`,
  },
  {
    id: 2,
    stop: "Finish",
    name: "Rüdesheim am Rhein",
    meta: `29.6 km · ${formatDuration(68 * 60)}`,
    terminal: true,
  },
];

const TWO_AREAS: SpikeAvoidArea[] = [
  { id: 0, place: "Baustelle B42, Rüdesheim", radius: "500 m" },
  { id: 1, place: "Gesperrter Weg, Assmannshausen", radius: "250 m" },
];

const PUBLISHED_BASE = {
  planName: "Rheinsteig Etappe 3",
  published: true,
  profile: "gravel",
  cues: true,
  turnCount: 32,
  waypoints: SIX_STOPS,
  avoid: TWO_AREAS,
} as const;

const LONG_NAMES = [
  "Mainzer Hauptbahnhof",
  "Georg-Büchner-Straße 42, Mainz",
  "Rheinufer Mombach",
  "Budenheim Fähre",
  "Heidesheim am Rhein",
  "Ingelheim Rotweinstraße",
  "Gau-Algesheim Marktplatz",
  "Laurenziberg",
  "Bingen Kulturufer",
  "Burg Rheinstein",
  "Trechtingshausen",
  "Bacharach Altstadt",
  "Oberwesel Stadtmauer",
  "Loreley-Felsen, St. Goarshausen",
  "Bahnhof St. Goar",
];

const FIFTEEN_STOPS: SpikeWaypoint[] = LONG_NAMES.map((name, index) => {
  const km = (index * 5.3).toFixed(1);
  return {
    id: index,
    stop: index === 0 ? "Start" : index === LONG_NAMES.length - 1 ? "Finish" : `Via ${index}`,
    name,
    ...(index === 0 ? {} : { meta: `${km} km · ${formatDuration(index * 12 * 60)}` }),
    straight: index === 6 || index === 11,
    terminal: index === 0 || index === LONG_NAMES.length - 1,
  };
});

export const SCENARIOS: Scenario[] = [
  {
    key: "empty",
    label: "Empty — new plan",
    planName: "",
    published: false,
    profile: "trekking",
    cues: false,
    waypoints: [],
    avoid: [],
    saving: false,
    saveError: null,
    delivery: { state: "none", sent: 0, total: 0 },
  },
  {
    key: "draft3",
    label: "Draft — 3 waypoints",
    planName: "Feierabendrunde Rheingau",
    published: false,
    profile: "trekking",
    cues: false,
    waypoints: THREE_STOPS,
    avoid: [],
    saving: false,
    saveError: null,
    delivery: { state: "none", sent: 0, total: 0 },
  },
  {
    key: "published6",
    label: "Published — 6 waypoints, cues on, delivered",
    ...PUBLISHED_BASE,
    saving: false,
    saveError: null,
    delivery: { state: "current", sent: 3, total: 3 },
  },
  {
    key: "long15",
    label: "Published — 15 waypoints",
    ...PUBLISHED_BASE,
    planName: "Rheinsteig lang",
    waypoints: FIFTEEN_STOPS,
    saving: false,
    saveError: null,
    delivery: { state: "current", sent: 3, total: 3 },
  },
  {
    key: "saving",
    label: "Saving in progress",
    ...PUBLISHED_BASE,
    saving: true,
    saveError: null,
    delivery: { state: "current", sent: 3, total: 3 },
  },
  {
    key: "saveError",
    label: "Save error",
    ...PUBLISHED_BASE,
    saving: false,
    saveError: "Could not reach the routing service.",
    delivery: { state: "current", sent: 3, total: 3 },
  },
  {
    key: "deliveryFailed",
    label: "Delivery failed",
    ...PUBLISHED_BASE,
    saving: false,
    saveError: null,
    delivery: { state: "failed", sent: 1, total: 3 },
  },
];

export const SCENARIO_BY_KEY = new Map(SCENARIOS.map((scenario) => [scenario.key, scenario]));

export const PROFILE_LABEL: Record<Scenario["profile"], string> = {
  trekking: "Trekking",
  fastbike: "Road",
  gravel: "Gravel",
};
