/**
 * The single place that knows the service's URL shapes.
 *
 * Requests are same-origin: the page is served by the Go service itself, and
 * the Tailnet identity header is supplied by the local `tailscale serve` proxy
 * in production and by the Vite dev proxy in development. The browser never
 * holds a credential.
 */

import {
  parseStage,
  parseStageGeometry,
  parseStages,
  parseStatus,
  parseWebUIConfig,
} from "./parse";
import type { Stage, StageGeometry, Status, WebUIConfig } from "./types";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }

  get isNotFound(): boolean {
    return this.status === 404;
  }
}

async function request<T>(path: string, parse: (payload: unknown) => T): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
    credentials: "same-origin",
  });

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = undefined;
  }

  if (!response.ok) {
    throw new ApiError(response.status, errorCode(payload), errorMessage(payload, response.status));
  }
  return parse(payload);
}

function errorCode(payload: unknown): string {
  const error = (payload as { error?: { code?: unknown } } | undefined)?.error;
  return typeof error?.code === "string" ? error.code : "unknown";
}

function errorMessage(payload: unknown, status: number): string {
  const error = (payload as { error?: { message?: unknown } } | undefined)?.error;
  return typeof error?.message === "string"
    ? error.message
    : `request failed with status ${status}`;
}

export function fetchStages(): Promise<Stage[]> {
  return request("/v1/routes", parseStages);
}

export function fetchStage(routeId: number, stageOrder: number): Promise<Stage> {
  return request(`/v1/routes/${routeId}/stages/${stageOrder}`, parseStage);
}

export function fetchStageGeometry(routeId: number, stageOrder: number): Promise<StageGeometry> {
  return request(`/v1/routes/${routeId}/stages/${stageOrder}/geometry`, parseStageGeometry);
}

export function fetchStatus(): Promise<Status> {
  return request("/v1/status", parseStatus);
}

export function fetchWebUIConfig(): Promise<WebUIConfig> {
  return request("/v1/webui/config", parseWebUIConfig);
}
