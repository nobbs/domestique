/**
 * Query definitions, kept beside the client so every feature fetches the same
 * resource the same way and cache keys cannot drift apart.
 */

import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import type { ForecastSample } from "../lib/forecastSamples";
import {
  fetchRoute,
  fetchRouteGeometry,
  fetchRoutes,
  fetchStatus,
  fetchSyncRuns,
  fetchWeather,
  fetchWebUIConfig,
  weatherQueryString,
} from "./client";

/** How often a run that has not finished is asked what it is doing now. */
const ACTIVE_POLL_MS = 2000;

/** How many recorded runs one page of the history holds. */
const HISTORY_PAGE_SIZE = 10;

/**
 * How many the notice reads at a time when it is looking for one named run.
 *
 * The largest page the service will serve. Resolving a reference means being
 * able to say it is not there, which is a question about the whole history
 * rather than about its first page — and the history is bounded, so the largest
 * page turns a walk of fifty requests into one of five.
 */
const LOOKUP_PAGE_SIZE = 100;

export const routesQuery = () =>
  queryOptions({
    queryKey: ["stages"] as const,
    queryFn: fetchRoutes,
  });

export const routeQuery = (provider: string, routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage", provider, routeId, stageOrder] as const,
    queryFn: () => fetchRoute(provider, routeId, stageOrder),
  });

export const routeGeometryQuery = (provider: string, routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage-geometry", provider, routeId, stageOrder] as const,
    queryFn: () => fetchRouteGeometry(provider, routeId, stageOrder),
    // Geometry only changes when a sync rewrites it, so it is worth holding.
    staleTime: 5 * 60 * 1000,
  });

export const statusQuery = () =>
  queryOptions({
    queryKey: ["status"] as const,
    queryFn: fetchStatus,
    // Poll only while the service says something has not finished, and quickly
    // while it does. A run reports its own progress, so there is something new
    // to see every few seconds; once it ends there is nothing to watch, and a
    // timer that kept asking would be asking on nobody's behalf. Every control
    // on the page invalidates this query, so the first answer after an operator
    // acts is never waited for.
    refetchInterval: (query) => (query.state.data?.sync.active ? ACTIVE_POLL_MS : false),
  });

/**
 * The recorded run history, a page at a time.
 *
 * Pages follow the cursor the service issues rather than an offset, because the
 * history is pruned from the far end as new runs are recorded and an offset
 * would slide across the rows underneath it.
 */
export const syncRunsQuery = () =>
  infiniteQueryOptions({
    queryKey: ["sync-runs"] as const,
    queryFn: ({ pageParam }) => fetchSyncRuns(pageParam, HISTORY_PAGE_SIZE),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next,
  });

/**
 * The same history, read for one reference rather than for reading.
 *
 * Kept apart from the card's own query because the two want different pages:
 * the card shows ten at a time because that is a readable card, and this reads
 * a hundred at a time because it is searching. It is only ever asked when a
 * notification named a run.
 */
export const syncRunLookupQuery = (reference: string) =>
  infiniteQueryOptions({
    queryKey: ["sync-run-lookup", reference] as const,
    queryFn: ({ pageParam }) => fetchSyncRuns(pageParam, LOOKUP_PAGE_SIZE),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next,
  });

export const webUIConfigQuery = () =>
  queryOptions({
    queryKey: ["webui-config"] as const,
    queryFn: fetchWebUIConfig,
    staleTime: Number.POSITIVE_INFINITY,
  });

/**
 * A forecast for one stage's forecast samples.
 *
 * The key is the exact query string `fetchWeather` sends, built by the same
 * `weatherQueryString` helper — so two calls that would hit the same URL
 * always share the same cache entry, and the two can never drift apart.
 */
export const weatherQuery = (samples: ForecastSample[]) =>
  queryOptions({
    queryKey: ["weather", weatherQueryString(samples)] as const,
    queryFn: () => fetchWeather(samples),
    // The endpoint resolves every point to its nearest hour, so nothing
    // finer than an hour would ever buy a different answer.
    staleTime: 60 * 60 * 1000,
  });
