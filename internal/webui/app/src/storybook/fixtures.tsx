import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { MemoryRouter } from "react-router";
import {
  routeGeometryQuery,
  routesQuery,
  settingsQuery,
  statusQuery,
  syncRunsQueryKey,
  tasksQuery,
  weatherQuery,
  webUIConfigQuery,
} from "../api/queries";
import type {
  BoundingBox,
  Route,
  RouteGeometry,
  Settings,
  Status,
  SyncRun,
  TaskList,
  WebUIConfig,
} from "../api/types";
import type { Climb } from "../lib/climbs";
import type { ForecastSample } from "../lib/forecastSamples";
import { buildProfile, gradientShares } from "../lib/profile";
import type { SurfaceSummary } from "../lib/surface";

export const coordinates = Array.from({ length: 40 }, (_, index): [number, number, number] => [
  8 + index * 0.001,
  49 + index * 0.0005,
  100 + index * 5,
]);

export const route: Route = {
  provider: "veloplanner",
  sourceRouteId: 12,
  stageOrder: 2,
  title: "Alpine loop — Descent",
  sourceRouteName: "Alpine loop",
  routeName: "Descent",
  sourceRevision: "2026-08-17",
  contentHash: "storybook",
  distanceMetres: 42_500,
  ascentMetres: 620,
  maxGradientPercent: 11.4,
  pointCount: coordinates.length,
  movingSeconds: 6_420,
  validation: { biasPercent: -1.2, maePercent: 6.8, p90Percent: 14.1, evaluatedRides: 42 },
};

/** The box every coordinate above fits inside, for the entry-page story's geometry. */
const [firstLon, firstLat] = coordinates[0] ?? [8, 49, 100];
export const routeBoundingBox: BoundingBox = coordinates.reduce<BoundingBox>(
  ([minLon, minLat, maxLon, maxLat], [lon, lat]) => [
    Math.min(minLon, lon),
    Math.min(minLat, lat),
    Math.max(maxLon, lon),
    Math.max(maxLat, lat),
  ],
  [firstLon, firstLat, firstLon, firstLat],
);

/** `route`'s own geometry, seeded for stories that render the map that fetches it. */
export const routeGeometryFixture: RouteGeometry = { bbox: routeBoundingBox, coordinates };

export const profile = buildProfile(coordinates);
export const bands = gradientShares(coordinates);
/**
 * Tomorrow morning at seven, by the clock the stories actually run under.
 *
 * The picker and the forecast judge a departure against the real clock — a
 * day in the past is refused, a horizon is sixteen days out — so a fixture
 * pinned to a calendar date drifts out of the window and the stories start
 * showing refusals instead of forecasts. Tomorrow is always inside it.
 */
export const rideStart: Date = (() => {
  const morning = new Date();
  morning.setDate(morning.getDate() + 1);
  morning.setHours(7, 0, 0, 0);

  return morning;
})();

/** Minutes after the ride starts, as a Date. */
function intoRide(minutes: number): Date {
  return new Date(rideStart.getTime() + minutes * 60_000);
}

export const weatherSamples: ForecastSample[] = [
  {
    position: coordinates[0] ?? [8, 49, 100],
    distanceMetres: 0,
    arrivalAt: intoRide(0),
  },
  {
    position: coordinates[20] ?? [8.02, 49.01, 200],
    distanceMetres: 1_500,
    arrivalAt: intoRide(45),
  },
  {
    position: coordinates[39] ?? [8.039, 49.0195, 295],
    distanceMetres: 3_000,
    arrivalAt: intoRide(90),
  },
];
export const surface: SurfaceSummary = {
  bands: [
    { kind: "asphalt", startMetres: 0, endMetres: 1_500 },
    { kind: "gravel", startMetres: 1_500, endMetres: 3_000 },
  ],
  shares: [
    { kind: "asphalt", metres: 1_500, share: 0.5 },
    { kind: "gravel", metres: 1_500, share: 0.5 },
  ],
  totalMetres: 3_000,
};
export const climbs: Climb[] = [
  {
    startMetres: 1_200,
    endMetres: 1_800,
    distanceMetres: 600,
    ascentMetres: 54,
    averageGradePercent: 9,
    maxGradePercent: 11.4,
  },
];

export const status: Status = {
  ready: true,
  converged: false,
  build: { revision: "0123456789abcdef0123456789abcdef01234567" },
  targets: [
    {
      id: "rider-a",
      authorisation: "authorized",
      convergence: "current",
      routes: { current: 4, pending: 0 },
    },
    {
      id: "rider-b",
      authorisation: "authorized",
      convergence: "lagging",
      routes: { current: 2, pending: 2 },
    },
  ],
  sync: {
    state: "idle",
    lastCompletedAt: "2026-08-18T06:30:00Z",
    sourceRoutes: 4,
    created: 0,
    updated: 2,
    deleted: 0,
    phases: {
      source: {
        lastCompletedAt: "2026-08-18T06:15:00Z",
        lastResult: "succeeded",
        sourceRoutes: 4,
        created: 0,
        updated: 0,
        deleted: 0,
      },
      targets: {
        lastCompletedAt: "2026-08-18T06:30:00Z",
        lastResult: "succeeded",
        sourceRoutes: 0,
        created: 0,
        updated: 2,
        deleted: 0,
      },
    },
    surface: { classified: 4, total: 4, incomplete: 0, enrichmentFailures: 0 },
  },
};

export const runs: SyncRun[] = [
  {
    reference: "1a2b3c4d5e6f",
    phase: "targets",
    completedAt: "2026-08-18T06:30:00Z",
    result: "succeeded",
    sourceRoutes: 0,
    created: 0,
    updated: 2,
    deleted: 0,
  },
  {
    reference: "f6e5d4c3b2a1",
    phase: "source",
    completedAt: "2026-08-18T06:15:00Z",
    result: "succeeded",
    sourceRoutes: 4,
    created: 0,
    updated: 0,
    deleted: 0,
  },
];

/** What the service is set to, as the settings form reads it back. */
export const settings: Settings = {
  timezone: "Europe/Berlin",
  alerts: [
    { task: "sync", alert: "source", enabled: true, decided: false },
    { task: "sync", alert: "destination", enabled: false, decided: true },
    { task: "surface:index", alert: "build", enabled: true, decided: false },
  ],
  sync: {
    allowEmptySourceDeletion: false,
    staleAfterSeconds: 26 * 3600,
    initialDelaySeconds: 60,
  },
  notifications: {
    enabled: true,
    pushoverBaseUrl: "https://api.pushover.net",
  },
  basemaps: [{ name: "Streets", styleUrl: "https://tiles.example.test/streets.json" }],
  surface: { regions: ["europe/germany"], rebuildIntervalSeconds: 7 * 24 * 3600 },
  wahoo: {
    apiBaseUrl: "https://api.wahooligan.com",
    oauthBaseUrl: "https://api.wahooligan.com",
    clientId: "wahoo-client-id",
    targets: ["rider-a", "rider-b"],
  },
  sources: [{ provider: "veloplanner", baseUrl: "https://veloplanner.com" }],
  rideModel: { coefficientsFile: "/var/lib/domestique/coefficients.json" },
  // A configured deployment: every credential stored, and nothing outstanding.
  secretsSet: {
    "wahoo.client_secret": true,
    "veloplanner.email": true,
    "veloplanner.password": true,
    "komoot.email": false,
    "komoot.password": false,
    "notifications.pushover.application_token": true,
    "notifications.pushover.user_key": true,
  },
  missing: [],
};

const config: WebUIConfig = {
  basemaps: [
    {
      name: "Streets",
      styleUrl: "https://tiles.openfreemap.org/styles/bright",
      darkCartography: false,
    },
  ],
  sourceBaseUrls: { veloplanner: "https://veloplanner.com" },
  // A session with a way out of it, which is what a deployment behind
  // Cloudflare Access has. Stories that need the other deployment — the one
  // with nothing in front of it to sign out to — override this.
  identity: { email: "rider@example.test", signOutUrl: "/cdn-cgi/access/logout" },
};

/** What this build registers, as the sync page reads it. */
export const tasks: TaskList = {
  tasks: [
    { name: "sync:source", scheduled: true, enabled: true, running: 0 },
    { name: "sync:target", scheduled: true, enabled: false, running: 0 },
    { name: "surface:annotate", scheduled: false, enabled: true, running: 0 },
    { name: "surface:index", scheduled: true, enabled: true, running: 0 },
  ],
};

export function StoryProviders({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(statusQuery().queryKey, status);
    next.setQueryData(tasksQuery().queryKey, tasks);
    next.setQueryData(webUIConfigQuery().queryKey, config);
    next.setQueryData(settingsQuery().queryKey, settings);
    next.setQueryData(routesQuery().queryKey, [route]);
    next.setQueryData(
      routeGeometryQuery(route.provider, route.sourceRouteId, route.stageOrder).queryKey,
      routeGeometryFixture,
    );
    next.setQueryData(syncRunsQueryKey(), {
      pages: [{ runs }],
      pageParams: [undefined],
    });
    next.setQueryData(weatherQuery(weatherSamples).queryKey, {
      points: weatherSamples.map((sample, index) => ({
        time: sample.arrivalAt.toISOString(),
        temperatureCelsius: 14 + index,
        apparentTemperatureCelsius: 12 + index,
        precipitationMillimetres: 0,
        precipitationProbabilityPercent: 5,
        windSpeedKmh: 10,
        windDirectionDegrees: 180,
        weatherCode: 1,
      })),
    });

    return next;
  });

  return (
    <QueryClientProvider client={client}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

/**
 * Replaces one global for the length of a story, and hands back the undo.
 *
 * `vi.stubGlobal` does the same thing, but it comes from Vitest's runtime,
 * which exists only under the test runner. These stories are also read in the
 * Storybook dev server, where reaching for it renders the story as an error
 * instead — so the stub is done here, against nothing but the platform.
 *
 * The undo puts back the original descriptor rather than the original value.
 * `localStorage` is a getter-only accessor on the window, so assigning to it
 * does nothing at all, and restoring it as a plain value would leave a frozen
 * copy of whatever the getter returned once — writable, where the real one is
 * not. A name with no own property to begin with is deleted rather than
 * restored, so nothing on the prototype stays shadowed.
 */
export function stubGlobal(name: string, value: unknown): () => void {
  const target = globalThis as unknown as Record<string, unknown>;
  const original = Object.getOwnPropertyDescriptor(target, name);
  Object.defineProperty(target, name, { value, configurable: true, writable: true });

  return () => {
    if (original) {
      Object.defineProperty(target, name, original);

      return;
    }
    delete target[name];
  };
}

/**
 * Answers the story's `fetch` for as long as the story is on screen.
 *
 * Stubbed while rendering rather than from an effect: the components below ask
 * for their data as they mount, which is before any effect of this one runs,
 * and well before a play function does. Undone on unmount rather than at the
 * end of `play`, which is not a promise that the global is ever put back — a
 * play function that never runs, or is abandoned when the reader moves to
 * another story, would leave every story after it fetching through this.
 *
 * The stub is also taken once per mount, not once per render: `stubGlobal`
 * captures whatever it replaces, and calling it twice would capture the first
 * stub as the thing to restore.
 */
export function StubbedFetch({
  respond,
  children,
}: {
  respond: typeof fetch;
  children: ReactNode;
}) {
  const restore = useRef<(() => void) | null>(null);
  if (!restore.current) {
    restore.current = stubGlobal("fetch", respond);
  }

  useEffect(
    () => () => {
      restore.current?.();
      restore.current = null;
    },
    [],
  );

  return <>{children}</>;
}

/**
 * Story parameters for anything that mounts a live MapLibre canvas. Its pixels
 * are third-party tiles drawn by WebGL over several frames, so two captures of
 * one story differ and Chromatic reports it as an unstable test.
 */
export const liveMap = { chromatic: { disableSnapshot: true } };
