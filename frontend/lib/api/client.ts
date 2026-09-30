import createClient, { type ClientOptions } from "openapi-fetch";

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
