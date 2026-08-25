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
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = undefined;
  }
  if (!response.ok) {
    const error = (payload as { error?: { code?: unknown; message?: unknown } } | undefined)?.error;
    throw new ApiError(
      response.status,
      typeof error?.code === "string" ? error.code : "unknown",
      typeof error?.message === "string"
        ? error.message
        : `request failed with status ${response.status}`,
    );
  }

  return { data: payload, headers: response.headers, status: response.status } as T;
}
