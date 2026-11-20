/**
 * Adapts every same-origin API call to the authenticated SLD control plane:
 *  - write requests get the X-SLD-Request header the server requires
 *    (CSRF protection for cookie sessions), so existing pages keep working;
 *  - a 401 from any API call asks the app to show the sign-in screen.
 */
export const UNAUTHORIZED_EVENT = "sld:unauthorized";

export function installFetchInterceptor(): void {
  const original = window.fetch.bind(window);
  if ((window.fetch as { __sld?: boolean }).__sld) return;

  const patched = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const target = new URL(url, window.location.href);
    const isApi = target.origin === window.location.origin && target.pathname.startsWith("/api/");

    if (!isApi) return original(input, init);

    const method = (init?.method || (input instanceof Request ? input.method : "GET")).toUpperCase();
    const headers = new Headers(init?.headers || (input instanceof Request ? input.headers : undefined));
    if (method !== "GET" && method !== "HEAD") {
      headers.set("X-SLD-Request", "1");
    }
    const response = await original(input, { ...init, headers, credentials: "same-origin" });
    if (response.status === 401 && !target.pathname.startsWith("/api/v2/auth/")) {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    return response;
  };
  (patched as { __sld?: boolean }).__sld = true;
  window.fetch = patched as typeof window.fetch;
}
