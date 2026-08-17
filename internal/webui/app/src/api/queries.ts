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
    refetchInterval: 60 * 1000,
  });

export const webUIConfigQuery = () =>
  queryOptions({
    queryKey: ["webui-config"] as const,
    queryFn: fetchWebUIConfig,
    staleTime: Number.POSITIVE_INFINITY,
  });
