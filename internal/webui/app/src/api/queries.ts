/**
 * Query definitions, kept beside the client so every feature fetches the same
 * resource the same way and cache keys cannot drift apart.
 */

import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import {
  fetchRoute,
  fetchRouteGeometry,
  fetchRoutes,
  fetchStatus,
  fetchSyncRuns,
  fetchWebUIConfig,
} from "./client";

/** How often a run that has not finished is asked what it is doing now. */
const ACTIVE_POLL_MS = 2000;

/** How many recorded runs one page of the history holds. */
const HISTORY_PAGE_SIZE = 10;

export const routesQuery = () =>
  queryOptions({
    queryKey: ["stages"] as const,
    queryFn: fetchRoutes,
  });

export const routeQuery = (routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage", routeId, stageOrder] as const,
    queryFn: () => fetchRoute(routeId, stageOrder),
  });

export const routeGeometryQuery = (routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage-geometry", routeId, stageOrder] as const,
    queryFn: () => fetchRouteGeometry(routeId, stageOrder),
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

export const webUIConfigQuery = () =>
  queryOptions({
    queryKey: ["webui-config"] as const,
    queryFn: fetchWebUIConfig,
    staleTime: Number.POSITIVE_INFINITY,
  });
