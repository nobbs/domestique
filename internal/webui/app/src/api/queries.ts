/** UI cache policy over Orval's generated operations and hooks. */
import type { InfiniteData } from "@tanstack/react-query";
import type { ForecastSample } from "../lib/forecastSamples";
import {
  type WebUIConfig as GeneratedWebUIConfig,
  type GetTaskRunsParams,
  getGetActivitiesQueryOptions,
  getGetRouteGeometryQueryOptions,
  getGetRouteQueryOptions,
  getGetRoutesQueryOptions,
  getGetSettingsQueryOptions,
  getGetStatusQueryOptions,
  getGetSyncRunsInfiniteQueryKey,
  getGetTaskRunsInfiniteQueryKey,
  getGetWeatherQueryOptions,
  getGetWebUIConfigQueryOptions,
  getListTasksQueryOptions,
  useGetSyncRunsInfinite,
  useGetTaskRunsInfinite,
} from "./generated";
import {
  type Activity,
  type ActivityList,
  type GeoJSONFeature,
  type Route,
  type RouteGeometry,
  routeGeometry,
  type Settings,
  type Status,
  type SyncRunPage,
  type TaskList,
  type TaskRunPage,
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
        const routes = payload<{ routes: Route[] } | Route[]>(response);

        return Array.isArray(routes) ? routes : routes.routes;
      },
    },
  });

/**
 * The rider's own recorded activities from `from` until now.
 *
 * The window's end is left to the service so that today's ride is in it; the
 * start is a whole day, which is what keeps the key — and the cache — steady
 * across a session.
 */
export const activitiesQuery = (from: string) =>
  getGetActivitiesQueryOptions(
    { from },
    {
      query: {
        select: (response) => {
          const list = payload<ActivityList | Activity[]>(response);

          return Array.isArray(list) ? list : list.activities;
        },
      },
    },
  );

export const routeQuery = (provider: string, sourceRouteId: number, stageOrder: number) =>
  getGetRouteQueryOptions(provider, sourceRouteId, stageOrder, {
    query: {
      select: (response) => payload<Route>(response),
    },
  });

export const routeGeometryQuery = (provider: string, sourceRouteId: number, stageOrder: number) =>
  getGetRouteGeometryQueryOptions<RouteGeometry>(provider, sourceRouteId, stageOrder, {
    query: {
      select: (response) => routeGeometry(payload<GeoJSONFeature>(response) as GeoJSONFeature),
      staleTime: 5 * 60 * 1000,
    },
  });

/** Each registered task's schedule state, polled while any attempt is in flight. */
export const tasksQuery = () =>
  getListTasksQueryOptions({
    query: {
      select: (response) => payload<TaskList>(response),
      refetchInterval: (query) => {
        const tasks = query.state.data && payload<TaskList>(query.state.data).tasks;

        return tasks?.some((task) => task.running > 0) ? ACTIVE_POLL_MS : false;
      },
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

/**
 * The settings the service holds, which an operator edits on this page.
 *
 * Not cached for the session the way the page's own configuration is: this one
 * is written from the same screen it is read on, and the answer to a write is
 * what the form shows next.
 */
export const settingsQuery = () =>
  getGetSettingsQueryOptions({
    query: {
      select: (response) => payload<Settings>(response),
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

/** `task` omitted rather than `undefined`: `exactOptionalPropertyTypes` treats the two differently. */
function taskRunsParams(task: string | undefined, limit: number): GetTaskRunsParams {
  return task === undefined ? { limit } : { task, limit };
}

/** Uses Orval's generated cursor hook while keeping the UI's page sizes local. */
export function useTaskRuns(task?: string, limit = HISTORY_PAGE_SIZE, enabled = true) {
  return useGetTaskRunsInfinite(taskRunsParams(task, limit), {
    query: {
      enabled,
      initialPageParam: undefined,
      getNextPageParam: (page) => payload<TaskRunPage>(page).next,
      select: (data) =>
        ({
          pages: data.pages.map((page) => payload<TaskRunPage>(page) as TaskRunPage),
          pageParams: data.pageParams,
        }) as InfiniteData<TaskRunPage, string | undefined>,
    },
  });
}

export const taskRunsQueryKey = (task?: string, limit = HISTORY_PAGE_SIZE) =>
  getGetTaskRunsInfiniteQueryKey(taskRunsParams(task, limit));

/** Every task-run page there is, whatever it is filtered or sized to. */
export const taskRunsQueryPrefix = () => getGetTaskRunsInfiniteQueryKey();

export { HISTORY_PAGE_SIZE, LOOKUP_PAGE_SIZE };
