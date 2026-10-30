// Cloudflare Worker in front of the GitHub Pages origin.
// Requests that prefer text/markdown get the page's .md twin (or 404.md);
// everything else passes through unchanged, with Vary: Accept on pages.

const MARKDOWN_PAGES = new Map([
  ["/", "/index.md"],
  ["/index.html", "/index.md"],
  ["/about", "/about.md"],
  ["/about.html", "/about.md"],
  ["/privacy", "/privacy.md"],
  ["/privacy.html", "/privacy.md"],
]);

const MARKDOWN_TYPE = "text/markdown; charset=utf-8";

// Returns the q-value for a media type in an Accept header, or 0 if absent.
function quality(accept, type) {
  let best = 0;
  for (const part of accept.split(",")) {
    const [range, ...params] = part.trim().toLowerCase().split(";");
    if (range.trim() !== type) continue;
    let q = 1;
    for (const p of params) {
      const [k, v] = p.trim().split("=");
      if (k === "q") q = Number.parseFloat(v);
    }
    if (!Number.isNaN(q)) best = Math.max(best, q);
  }
  return best;
}

export function prefersMarkdown(accept) {
  if (!accept) return false;
  const md = quality(accept, "text/markdown");
  return md > 0 && md >= quality(accept, "text/html");
}

function withVary(response) {
  const out = new Response(response.body, response);
  const vary = out.headers.get("Vary");
  if (!vary) out.headers.set("Vary", "Accept");
  else if (!/\baccept\b/i.test(vary.replace(/accept-encoding/gi, ""))) {
    out.headers.set("Vary", `${vary}, Accept`);
  }
  return out;
}

async function markdown(origin, path, status, fetchImpl) {
  const res = await fetchImpl(new Request(new URL(path, origin), { method: "GET" }));
  if (!res.ok) return null;
  return new Response(await res.text(), {
    status,
    headers: { "Content-Type": MARKDOWN_TYPE, Vary: "Accept", "Cache-Control": "public, max-age=600" },
  });
}

export async function handle(request, fetchImpl = fetch) {
  if (request.method !== "GET" && request.method !== "HEAD") return fetchImpl(request);
  const url = new URL(request.url);
  const page = MARKDOWN_PAGES.get(url.pathname);

  if (prefersMarkdown(request.headers.get("Accept"))) {
    if (page) {
      const md = await markdown(url.origin, page, 200, fetchImpl);
      if (md) return md;
    }
    const res = await fetchImpl(request);
    if (res.status === 404) {
      const md = await markdown(url.origin, "/404.md", 404, fetchImpl);
      if (md) return md;
    }
    return page ? withVary(res) : res;
  }

  const res = await fetchImpl(request);
  return page || res.status === 404 ? withVary(res) : res;
}

// Fail open: any error in negotiation falls back to the origin unchanged.
export async function safeHandle(request, fetchImpl = fetch) {
  try {
    return await handle(request.clone(), fetchImpl);
  } catch {
    return fetchImpl(request);
  }
}

export default {
  fetch(request) {
    return safeHandle(request);
  },
};
