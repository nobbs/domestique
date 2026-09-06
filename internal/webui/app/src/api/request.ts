/** The generated operations' one same-origin transport boundary. */
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

export type ErrorType<_Error> = ApiError;

/**
 * Every error response, binary operations included, is this service's own
 * JSON error envelope — only a success body's shape varies by operation.
 */
async function throwApiError(response: Response): Promise<never> {
  // The gate redirected a document request that lacked a session; this is the
  // same rejection reaching a fetch, which gets no redirect to follow.
  if (response.status === 401) {
    window.location.assign("/auth/login");
  }
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = undefined;
  }
  const error = (payload as { error?: { code?: unknown; message?: unknown } } | undefined)?.error;
  throw new ApiError(
    response.status,
    typeof error?.code === "string" ? error.code : "unknown",
    typeof error?.message === "string"
      ? error.message
      : `request failed with status ${response.status}`,
  );
}

/**
 * Returns Orval's full response envelope. The generated request serializer owns
 * query strings, including repeated weather `point` parameters.
 */
export async function domestiqueRequest<T>(url: string, options: RequestInit): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json, application/geo+json");
  if (options.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(url, { ...options, credentials: "same-origin", headers });
  if (!response.ok) {
    await throwApiError(response);
  }
  let payload: unknown;
  let readable = true;
  try {
    payload = await response.json();
  } catch {
    payload = undefined;
    readable = false;
  }
  // A success whose body did not parse is a transport failure, not an empty
  // answer. Returning `data: undefined` would push it downstream to surface as
  // a property access on nothing, naming neither the request nor the problem.
  if (!readable && response.status !== 204) {
    throw new ApiError(
      response.status,
      "unreadable_response",
      `${options.method ?? "GET"} ${url} returned a body that is not JSON`,
    );
  }

  return { data: payload, headers: response.headers, status: response.status } as T;
}

/**
 * Same transport as `domestiqueRequest`, for the one operation whose success
 * body is bytes rather than JSON: no `Accept` override, no `response.json()`
 * on the happy path — but a failure still carries this service's own JSON
 * error envelope, same as every other operation.
 */
export async function domestiqueBinaryRequest<T>(url: string, options: RequestInit): Promise<T> {
  const response = await fetch(url, { ...options, credentials: "same-origin" });
  if (!response.ok) {
    await throwApiError(response);
  }

  return { data: await response.blob(), headers: response.headers, status: response.status } as T;
}

/** Keeps a generated operation's response envelope at the transport boundary. */
export function unwrap<T>(value: { data: T; status: number; headers: Headers } | T): T {
  // The full envelope shape and its runtime types, not just matching key
  // names, so a legitimate payload that happens to carry its own `data`,
  // `status` and `headers` fields is never unwrapped by mistake.
  const envelope = value as { data: T; status: unknown; headers: unknown };
  if (
    value &&
    typeof value === "object" &&
    "data" in value &&
    "status" in value &&
    "headers" in value &&
    typeof envelope.status === "number" &&
    envelope.headers instanceof Headers
  ) {
    return envelope.data;
  }

  return value as T;
}
