/**
 * The single place that knows the service's URL shapes.
 *
 * Requests are same-origin: the page is served by the Go service itself, and
 * the Tailnet identity header is supplied by the local `tailscale serve` proxy
 * in production and by the Vite dev proxy in development. The browser never
 * holds a credential.
 */

import {
  ContractError,
  parseRoute,
  parseRouteGeometry,
  parseRoutes,
  parseStatus,
  parseSyncRuns,
  parseSyncSchedule,
  parseWebUIConfig,
} from "./parse";
import type {
  Route,
  RouteGeometry,
  Status,
  SyncPhase,
  SyncRunPage,
  SyncSchedule,
  WebUIConfig,
} from "./types";

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

interface RequestOptions {
  method: string;
  body?: unknown;
}

async function request<T>(
  path: string,
  parse: (payload: unknown) => T,
  options: RequestOptions = { method: "GET" },
): Promise<T> {
  const response = await fetch(path, {
    method: options.method,
    headers:
      options.body === undefined
        ? { Accept: "application/json" }
        : { Accept: "application/json", "Content-Type": "application/json" },
    credentials: "same-origin",
    ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
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

  try {
    return parse(payload);
  } catch (error) {
    // The parsers report which field failed; only this layer knows which request
    // it came back from. Attaching it here is what makes a drift between the Go
    // views and this client readable as "unexpected API response from GET
    // /v1/routes: stages[0].title is not a string" wherever the message
    // surfaces, including on screen.
    if (error instanceof ContractError) {
      throw error.at(`${options.method} ${path}`);
    }
    throw error;
  }
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

export function fetchRoutes(): Promise<Route[]> {
  return request("/v1/routes", parseRoutes);
}

export function fetchRoute(provider: string, routeId: number, stageOrder: number): Promise<Route> {
  return request(
    `/v1/providers/${encodeURIComponent(provider)}/routes/${routeId}/stages/${stageOrder}`,
    parseRoute,
  );
}

export function fetchRouteGeometry(
  provider: string,
  routeId: number,
  stageOrder: number,
): Promise<RouteGeometry> {
  return request(
    `/v1/providers/${encodeURIComponent(provider)}/routes/${routeId}/stages/${stageOrder}/geometry`,
    parseRouteGeometry,
  );
}

export function fetchStatus(): Promise<Status> {
  return request("/v1/status", parseStatus);
}

/**
 * Reads one page of the recorded run history, newest first.
 *
 * `after` is the cursor the previous page ended with, absent for the newest
 * page. The service refuses a cursor it did not issue rather than answering with
 * the newest page, so a cursor is never invented here.
 */
export function fetchSyncRuns(after: string | undefined, limit: number): Promise<SyncRunPage> {
  const query = new URLSearchParams({ limit: String(limit) });
  if (after !== undefined) {
    query.set("after", after);
  }

  return request(`/v1/sync/runs?${query.toString()}`, parseSyncRuns);
}

export function fetchWebUIConfig(): Promise<WebUIConfig> {
  return request("/v1/webui/config", parseWebUIConfig);
}

/** The paths that start one half of a synchronisation, or both. */
const SYNC_PATHS: Record<SyncPhase | "all", string> = {
  all: "/v1/sync",
  source: "/v1/sync/source",
  targets: "/v1/sync/targets",
};

/**
 * Asks for one immediate run. The service answers `202` and runs it in the
 * background, so there is nothing to parse and nothing to wait for; a rejected
 * request means a run was already in flight, which surfaces as an ApiError with
 * a 409 status rather than as a silent no-op.
 */
export function triggerSync(phase: SyncPhase | "all"): Promise<null> {
  return request(SYNC_PATHS[phase], () => null, { method: "POST" });
}

/**
 * Asks for one stage to be redone from scratch, and for the run that will do it.
 *
 * The service records the request before starting anything, so a request made
 * while a run is already in flight is honoured by the next pass rather than
 * refused. There is nothing to parse: the work happens in the background.
 */
export function reprocessStage(
  provider: string,
  routeId: number,
  stageOrder: number,
): Promise<null> {
  return request(
    `/v1/providers/${encodeURIComponent(provider)}/routes/${routeId}/stages/${stageOrder}/reprocess`,
    () => null,
    { method: "POST" },
  );
}

/**
 * Sets both schedule switches and returns what the service stored.
 *
 * Both travel together because the service refuses a half-named schedule: the
 * unnamed switch would be left at whatever the caller assumed it was.
 */
export function setSyncSchedule(schedule: SyncSchedule): Promise<SyncSchedule> {
  return request("/v1/sync/schedule", (payload) => parseSyncSchedule(payload), {
    method: "PUT",
    body: schedule,
  });
}
