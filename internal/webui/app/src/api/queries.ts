/**
 * Query definitions, kept beside the client so every feature fetches the same
 * resource the same way and cache keys cannot drift apart.
 */

import { queryOptions } from "@tanstack/react-query";
import {
  fetchStage,
  fetchStageGeometry,
  fetchStages,
  fetchStatus,
  fetchWebUIConfig,
} from "./client";

/** How often a run that has not finished is asked what it is doing now. */
const ACTIVE_POLL_MS = 2000;

export const stagesQuery = () =>
  queryOptions({
    queryKey: ["stages"] as const,
    queryFn: fetchStages,
  });

export const stageQuery = (routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage", routeId, stageOrder] as const,
    queryFn: () => fetchStage(routeId, stageOrder),
  });

export const stageGeometryQuery = (routeId: number, stageOrder: number) =>
  queryOptions({
    queryKey: ["stage-geometry", routeId, stageOrder] as const,
    queryFn: () => fetchStageGeometry(routeId, stageOrder),
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

export const webUIConfigQuery = () =>
  queryOptions({
    queryKey: ["webui-config"] as const,
    queryFn: fetchWebUIConfig,
    staleTime: Number.POSITIVE_INFINITY,
  });
