// ─── Cloudflare Worker: API Proxy ─────────────────────────
// Routes /api/* requests to the backend origin
// Provides CORS, caching, and DDoS protection

export interface Env {
  API_ORIGIN: string;       // e.g. https://api.observeid.io (backend origin)
  FRONTEND_ORIGIN?: string; // e.g. https://app.observeid.io (for CORS)
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const origin = env.API_ORIGIN || "https://api.observeid.io";

    // ─── CORS Headers ─────────────────────────────────────
    const frontendOrigin = env.FRONTEND_ORIGIN || "https://app.observeid.io";
    const corsHeaders = {
      "Access-Control-Allow-Origin": frontendOrigin,
      "Access-Control-Allow-Methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, Authorization, X-Correlation-ID",
      "Access-Control-Max-Age": "86400",
    };

    // Handle preflight
    if (request.method === "OPTIONS") {
      return new Response(null, {
        headers: corsHeaders,
        status: 204,
      });
    }

    // ─── Proxy Request ────────────────────────────────────
    const targetUrl = `${origin}${url.pathname}${url.search}`;

    const headers = new Headers(request.headers);
    headers.set("X-Forwarded-For", request.headers.get("CF-Connecting-IP") || "");
    headers.set("X-Cloudflare-Worker", "true");

    const proxyRequest = new Request(targetUrl, {
      method: request.method,
      headers: headers,
      body: request.method !== "GET" && request.method !== "HEAD" ? request.body : null,
    });

    try {
      const response = await fetch(proxyRequest);

      // ─── Build Response ─────────────────────────────────
      const responseHeaders = new Headers(response.headers);
      Object.entries(corsHeaders).forEach(([key, value]) => {
        responseHeaders.set(key, value);
      });

      // Cache static responses at the edge
      if (request.method === "GET" && response.status === 200) {
        responseHeaders.set("Cache-Control", "public, max-age=60, s-maxage=120");
      }

      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: responseHeaders,
      });
    } catch (err) {
      return new Response(
        JSON.stringify({
          error: "upstream_unavailable",
          message: "Identity service temporarily unavailable",
        }),
        {
          status: 503,
          headers: {
            ...corsHeaders,
            "Content-Type": "application/json",
          },
        }
      );
    }
  },
};
