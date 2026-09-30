import { describe, expect, it, vi } from "vitest";

import { createApiClient, isApiErrorResponse } from "@/lib/api/client";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("createApiClient", () => {
  it("calls documented endpoints with typed responses", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(200, { status: "ok" }),
    );
    const client = createApiClient({
      baseUrl: "http://api.test",
      fetch: fetchMock,
    });

    const { data, error } = await client.GET("/health/live");

    expect(error).toBeUndefined();
    expect(data?.status).toBe("ok");
    const request = fetchMock.mock.calls[0]?.[0];
    expect(request).toBeInstanceOf(Request);
    expect((request as Request).url).toBe("http://api.test/health/live");
  });

  it("surfaces the error contract on failure", async () => {
    const body = {
      error: {
        code: "SERVICE_UNAVAILABLE",
        message: "Unavailable.",
        request_id: "req_1",
      },
    };
    const client = createApiClient({
      baseUrl: "http://api.test",
      fetch: async () => jsonResponse(503, body),
    });

    const { data, error } = await client.GET("/health/live");

    expect(data).toBeUndefined();
    expect(isApiErrorResponse(error)).toBe(true);
  });
});

describe("isApiErrorResponse", () => {
  it.each([
    [{ error: { code: "X", message: "m", request_id: "r" } }, true],
    [{ error: { code: "X", message: "m" } }, false],
    [{ error: "boom" }, false],
    [{ status: "ok" }, false],
    [null, false],
    ["error", false],
  ])("%j -> %s", (body, want) => {
    expect(isApiErrorResponse(body)).toBe(want);
  });
});
