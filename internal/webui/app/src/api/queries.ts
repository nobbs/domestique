/** UI cache policy over Orval's generated operations and hooks. */
import type { InfiniteData } from "@tanstack/react-query";
import type { ForecastSample } from "../lib/forecastSamples";
import {
  type WebUIConfig as GeneratedWebUIConfig,
  getGetRouteGeometryQueryOptions,
  getGetRouteQueryOptions,
  getGetRoutesQueryOptions,
  getGetStatusQueryOptions,
  getGetSyncRunsInfiniteQueryKey,
  getGetWeatherQueryOptions,
  getGetWebUIConfigQueryOptions,
  useGetSyncRunsInfinite,
} from "./generated";
import {
  type GeoJSONFeature,
  type Route,
  type RouteGeometry,
  routeGeometry,
  type Status,
  type SyncRunPage,
  type WeatherForecast,
  webUIConfig,
} from "./types";

const ACTIVE_POLL_MS = 2000;
const HISTORY_PAGE_SIZE = 10;
const LOOKUP_PAGE_SIZE = 100;

/** Keeps generated response envelopes at the transport boundary. */
function payload<T>(value: { data: T } | T): T {
  if (value && typeof value === "object" && "data" in value) {
    return value.data;
  }

  return value;
}

export const routesQuery = () =>
  getGetRoutesQueryOptions({
    query: {
      select: (response) => {
        const routes = payload<{ stages: Route[] } | Route[]>(response);

        return Array.isArray(routes) ? routes : routes.stages;
      },
    },
  });

export const routeQuery = (provider: string, routeId: number, stageOrder: number) =>
  getGetRouteQueryOptions(provider, routeId, stageOrder, {
    query: {
      select: (response) => payload<Route>(response),
    },
  });

export const routeGeometryQuery = (provider: string, routeId: number, stageOrder: number) =>
  getGetRouteGeometryQueryOptions<RouteGeometry>(provider, routeId, stageOrder, {
    query: {
      select: (response) => routeGeometry(payload<GeoJSONFeature>(response) as GeoJSONFeature),
      staleTime: 5 * 60 * 1000,
    },
  });

export const statusQuery = () =>
  getGetStatusQueryOptions({
    query: {
      select: (response) => payload<Status>(response),
      refetchInterval: (query) =>
        query.state.data && payload<Status>(query.state.data)?.sync?.active
          ? ACTIVE_POLL_MS
          : false,
    },
  });

export const webUIConfigQuery = () =>
  getGetWebUIConfigQueryOptions({
    query: {
      select: (response) => webUIConfig(payload<GeneratedWebUIConfig>(response)),
      staleTime: Number.POSITIVE_INFINITY,
    },
  });

function weatherParameters(samples: ForecastSample[]) {
  return {
    point: samples.map(({ position: [longitude, latitude], arrivalAt }) =>
      [latitude, longitude, arrivalAt.toISOString()].join(","),
    ),
  };
}

export const weatherQuery = (samples: ForecastSample[]) => {
  const parameters = weatherParameters(samples);

  return getGetWeatherQueryOptions(parameters, {
    query: {
      select: (response) => {
        const forecast = payload<WeatherForecast>(response) as WeatherForecast;
        if (forecast.points.length !== samples.length) {
          throw new Error(
            `weather returned ${forecast.points.length} points for ${samples.length} samples`,
          );
        }

        return forecast;
      },
    },
  });
};

/** Uses Orval's generated cursor hook while keeping the UI's page sizes local. */
export function useSyncRuns(limit = HISTORY_PAGE_SIZE, enabled = true) {
  return useGetSyncRunsInfinite(
    { limit },
    {
      query: {
        enabled,
        initialPageParam: undefined,
        getNextPageParam: (page) => payload<SyncRunPage>(page).next,
        select: (data) =>
          ({
            pages: data.pages.map((page) => payload<SyncRunPage>(page) as SyncRunPage),
            pageParams: data.pageParams,
          }) as InfiniteData<SyncRunPage, string | undefined>,
      },
    },
  );
}

export const syncRunsQueryKey = (limit = HISTORY_PAGE_SIZE) =>
  getGetSyncRunsInfiniteQueryKey({ limit });

export function useSyncRunLookup(reference: string | null) {
  return useSyncRuns(LOOKUP_PAGE_SIZE, reference !== null);
}

export { HISTORY_PAGE_SIZE, LOOKUP_PAGE_SIZE };
