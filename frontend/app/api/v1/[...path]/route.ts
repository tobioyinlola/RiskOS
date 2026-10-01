/**
 * Same-origin proxy to the RiskOS API. The console calls /api/v1/* on its own origin, so the
 * session cookie can be SameSite=Strict and HttpOnly, and the API's address stays server-side
 * (RISKOS_API_URL, read at request time, not baked into the build).
 */

const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
]);

// Only these request headers reach the API: cookies, the console's own headers, and the
// client's address chain. The API believes X-Forwarded-For only from proxies it is configured to
// trust (RISKOS_TRUSTED_PROXIES), and then only the entries those proxies added.
const FORWARD = [
  "accept",
  "content-type",
  "cookie",
  "origin",
  "user-agent",
  "x-csrf-token",
  "riskos-environment",
  "x-request-id",
  "x-forwarded-for",
];

function apiBase(): string {
  return (process.env.RISKOS_API_URL ?? "http://127.0.0.1:8080").replace(
    /\/$/,
    "",
  );
}

async function proxy(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const target = `${apiBase()}${url.pathname}${url.search}`;
  const headers = new Headers();
  for (const name of FORWARD) {
    const value = request.headers.get(name);
    if (value !== null) headers.set(name, value);
  }
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      redirect: "manual",
      cache: "no-store",
    });
  } catch {
    return Response.json(
      {
        error: {
          code: "SERVICE_UNAVAILABLE",
          message: "The RiskOS API is unreachable.",
          request_id: "",
        },
      },
      { status: 503 },
    );
  }
  const out = new Headers();
  upstream.headers.forEach((value, name) => {
    if (
      !HOP_BY_HOP.has(name.toLowerCase()) &&
      name.toLowerCase() !== "set-cookie"
    ) {
      out.set(name, value);
    }
  });
  for (const cookie of upstream.headers.getSetCookie()) {
    out.append("set-cookie", cookie);
  }
  return new Response(upstream.body, { status: upstream.status, headers: out });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const dynamic = "force-dynamic";
