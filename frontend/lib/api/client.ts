import createClient, {
  type ClientOptions,
  type Middleware,
} from "openapi-fetch";

import type { components, paths } from "./schema.gen";

/**
 * Typed RiskOS API client. Paths, parameters and response bodies are generated from
 * backend/api/openapi.yaml (`make generate`), so a contract change that breaks the frontend fails
 * type-checking instead of failing at runtime. Never hand-write API types.
 */
export function createApiClient(options: ClientOptions) {
  return createClient<paths>(options);
}

export type ApiClient = ReturnType<typeof createApiClient>;

/** Schema types, re-exported for the console. */
export type Schemas = components["schemas"];

/** The standard error contract returned by every RiskOS endpoint (spec §7). */
export type ApiErrorResponse = components["schemas"]["ErrorResponse"];

/** Narrows an unknown response body to the error contract. */
export function isApiErrorResponse(body: unknown): body is ApiErrorResponse {
  if (typeof body !== "object" || body === null || !("error" in body)) {
    return false;
  }
  const { error } = body;
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as Record<string, unknown>).code === "string" &&
    typeof (error as Record<string, unknown>).message === "string" &&
    typeof (error as Record<string, unknown>).request_id === "string"
  );
}

/** An API error in the console: status, stable code and a message safe to show. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Converts an openapi-fetch result into data or a thrown ApiError. */
export function unwrap<T>(result: {
  data?: T;
  error?: unknown;
  response: Response;
}): T {
  if (result.error !== undefined || !result.response.ok) {
    const body = result.error;
    if (isApiErrorResponse(body)) {
      throw new ApiError(
        result.response.status,
        body.error.code,
        body.error.message,
        body.error.request_id,
      );
    }
    throw new ApiError(
      result.response.status,
      "UNKNOWN",
      "The request failed.",
    );
  }
  return result.data as T;
}

export type Environment = "test" | "production";

/**
 * Per-page request context: the session's CSRF token and the chosen environment. Held in module
 * state (not in storage) so a token never outlives the page.
 */
const context: { csrf: string; environment: Environment } = {
  csrf: "",
  environment: "test",
};

export function setCsrfToken(token: string) {
  context.csrf = token;
}

export function setEnvironment(env: Environment) {
  context.environment = env;
}

export function currentEnvironment(): Environment {
  return context.environment;
}

const consoleHeaders: Middleware = {
  onRequest({ request }) {
    request.headers.set("RiskOS-Environment", context.environment);
    if (context.csrf && request.method !== "GET" && request.method !== "HEAD") {
      request.headers.set("X-CSRF-Token", context.csrf);
    }
    return request;
  },
};

let browserClient: ApiClient | undefined;

/** The console's client: same-origin calls through the /api/v1 proxy, with the cookie. */
export function api(): ApiClient {
  if (!browserClient) {
    const origin =
      typeof window === "undefined"
        ? "http://localhost"
        : window.location.origin;
    browserClient = createApiClient({
      baseUrl: origin,
      credentials: "same-origin",
    });
    browserClient.use(consoleHeaders);
  }
  return browserClient;
}
