import createClient, { type Middleware } from "openapi-fetch";

import type { components, paths } from "./admin.gen";
import { createApiClient, type ApiClient, type Environment } from "./client";

/**
 * Typed client for the internal admin API (backend/api/admin.yaml), used only by the staff
 * console under /admin. Same-origin through the /api/admin/v1 proxy; the staff session cookie is
 * HttpOnly, and its CSRF token is held in module state for the page's life.
 */
export type AdminSchemas = components["schemas"];

const state = { csrf: "" };

export function setStaffCsrfToken(token: string) {
  state.csrf = token;
}

const staffHeaders: Middleware = {
  onRequest({ request }) {
    if (state.csrf && request.method !== "GET" && request.method !== "HEAD") {
      request.headers.set("X-CSRF-Token", state.csrf);
    }
    return request;
  },
};

function origin(): string {
  return typeof window === "undefined"
    ? "http://localhost"
    : window.location.origin;
}

let adminClient: ReturnType<typeof createClient<paths>> | undefined;

export function adminApi() {
  if (!adminClient) {
    adminClient = createClient<paths>({
      baseUrl: origin(),
      credentials: "same-origin",
    });
    adminClient.use(staffHeaders);
  }
  return adminClient;
}

/**
 * A customer API client that reads one tenant with approved support access: every request
 * carries the grant and the environment; the API refuses anything but GET.
 */
export function supportApi(grant: string, environment: Environment): ApiClient {
  const client = createApiClient({
    baseUrl: origin(),
    credentials: "same-origin",
  });
  client.use({
    onRequest({ request }) {
      request.headers.set("RiskOS-Support-Grant", grant);
      request.headers.set("RiskOS-Environment", environment);
      return request;
    },
  });
  return client;
}
