import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { MemoryRouter } from "react-router";
import {
  settingsQuery,
  statusQuery,
  syncRunsQueryKey,
  weatherQuery,
  webUIConfigQuery,
} from "../api/queries";
import type { Route, Settings, Status, SyncRun, WebUIConfig } from "../api/types";
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
  routeId: 12,
  stageOrder: 2,
  title: "Alpine loop — Descent",
  routeName: "Alpine loop",
  stageName: "Descent",
  sourceRevision: "2026-08-17",
  contentHash: "storybook",
  distanceMetres: 42_500,
  ascentMetres: 620,
  maxGradientPercent: 11.4,
  pointCount: coordinates.length,
  movingSeconds: 6_420,
  validation: { biasPercent: -1.2, maePercent: 6.8, p90Percent: 14.1, evaluatedRides: 42 },
};

export const profile = buildProfile(coordinates);
export const bands = gradientShares(coordinates);
export const weatherSamples: ForecastSample[] = [
  {
    position: coordinates[0] ?? [8, 49, 100],
    distanceMetres: 0,
    arrivalAt: new Date("2026-08-18T07:00:00Z"),
  },
  {
    position: coordinates[20] ?? [8.02, 49.01, 200],
    distanceMetres: 1_500,
    arrivalAt: new Date("2026-08-18T07:45:00Z"),
  },
  {
    position: coordinates[39] ?? [8.039, 49.0195, 295],
    distanceMetres: 3_000,
    arrivalAt: new Date("2026-08-18T08:30:00Z"),
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
      stages: { current: 4, pending: 0 },
    },
    {
      id: "rider-b",
      authorisation: "authorized",
      convergence: "lagging",
      stages: { current: 2, pending: 2 },
    },
  ],
  sync: {
    state: "idle",
    lastCompletedAt: "2026-08-18T06:30:00Z",
    sourceStages: 4,
    created: 0,
    updated: 2,
    deleted: 0,
    schedule: { source: true, targets: true },
    phases: {
      source: {
        lastCompletedAt: "2026-08-18T06:15:00Z",
        lastResult: "succeeded",
        sourceStages: 4,
        created: 0,
        updated: 0,
        deleted: 0,
      },
      targets: {
        lastCompletedAt: "2026-08-18T06:30:00Z",
        lastResult: "succeeded",
        sourceStages: 0,
        created: 0,
        updated: 2,
        deleted: 0,
      },
    },
    surface: { classified: 4, total: 4, incomplete: 0 },
  },
};

export const runs: SyncRun[] = [
  {
    reference: "1a2b3c4d5e6f",
    phase: "targets",
    completedAt: "2026-08-18T06:30:00Z",
    result: "succeeded",
    sourceStages: 0,
    created: 0,
    updated: 2,
    deleted: 0,
  },
  {
    reference: "f6e5d4c3b2a1",
    phase: "source",
    completedAt: "2026-08-18T06:15:00Z",
    result: "succeeded",
    sourceStages: 4,
    created: 0,
    updated: 0,
    deleted: 0,
  },
];

/** What the service is set to, as the settings form reads it back. */
export const settings: Settings = {
  sync: { allowEmptySourceDeletion: false, staleAfterSeconds: 26 * 3600 },
  notifications: {
    enabled: true,
    successPolicy: "digest",
    digestIntervalSeconds: 24 * 3600,
    pushoverBaseUrl: "https://api.pushover.net",
  },
  basemaps: [{ name: "Streets", styleUrl: "https://tiles.example.test/streets.json" }],
  surface: { regions: ["europe/germany"], rebuildIntervalSeconds: 7 * 24 * 3600 },
};

const config: WebUIConfig = {
  basemaps: [],
  sourceBaseUrls: { veloplanner: "https://veloplanner.com" },
  // A session with a way out of it, which is what a deployment behind
  // Cloudflare Access has. Stories that need the other deployment — the one
  // with nothing in front of it to sign out to — override this.
  identity: { email: "rider@example.test", signOutUrl: "/cdn-cgi/access/logout" },
};

export function StoryProviders({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const next = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    next.setQueryData(statusQuery().queryKey, status);
    next.setQueryData(webUIConfigQuery().queryKey, config);
    next.setQueryData(settingsQuery().queryKey, settings);
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
