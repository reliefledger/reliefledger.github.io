import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { handle, prefersMarkdown } from "./markdown-negotiation.mjs";

const root = new URL("../", import.meta.url);

// Fake GitHub Pages origin backed by the repository files.
async function origin(request) {
  const { pathname } = new URL(request.url);
  const file = { "/": "index.html", "/about": "about.html", "/privacy": "privacy.html" }[pathname] ?? pathname.slice(1);
  try {
    const body = await readFile(new URL(file, root));
    const type = file.endsWith(".md") ? "text/markdown" : "text/html; charset=utf-8";
    return new Response(body, { status: 200, headers: { "Content-Type": type, Vary: "Accept-Encoding" } });
  } catch {
    const body = await readFile(new URL("404.html", root));
    return new Response(body, { status: 404, headers: { "Content-Type": "text/html; charset=utf-8" } });
  }
}

const get = (path, accept) =>
  handle(new Request(`https://reliefledger.pro${path}`, { headers: accept ? { Accept: accept } : {} }), origin);

test("prefersMarkdown honours q-values", () => {
  assert.equal(prefersMarkdown("text/markdown"), true);
  assert.equal(prefersMarkdown("text/markdown, text/html"), true);
  assert.equal(prefersMarkdown("text/html, text/markdown;q=0.5"), false);
  assert.equal(prefersMarkdown("text/html,application/xhtml+xml,*/*;q=0.8"), false);
  assert.equal(prefersMarkdown("text/markdown;q=0"), false);
  assert.equal(prefersMarkdown(null), false);
});

test("homepage returns Markdown with Vary: Accept", async () => {
  const res = await get("/", "text/markdown");
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("Content-Type"), "text/markdown; charset=utf-8");
  assert.match(res.headers.get("Vary"), /Accept/);
  assert.match(await res.text(), /^# ReliefLedger/);
});

test("homepage still returns HTML to browsers, with Vary: Accept", async () => {
  const res = await get("/", "text/html");
  assert.equal(res.status, 200);
  assert.match(res.headers.get("Content-Type"), /text\/html/);
  assert.match(res.headers.get("Vary"), /Accept-Encoding, Accept/);
  assert.match(await res.text(), /<!doctype html>/i);
});

test("about and privacy have Markdown twins", async () => {
  for (const path of ["/about", "/about.html", "/privacy.html"]) {
    const res = await get(path, "text/markdown");
    assert.equal(res.status, 200, path);
    assert.match(await res.text(), /^# /, path);
  }
});

test("missing page returns a Markdown 404 with links", async () => {
  const res = await get("/__missing", "text/markdown");
  assert.equal(res.status, 404);
  assert.equal(res.headers.get("Content-Type"), "text/markdown; charset=utf-8");
  const body = await res.text();
  assert.ok(body.length >= 20);
  assert.match(body, /llms\.txt/);
  assert.match(body, /sitemap\.xml/);
});

test("missing page keeps the HTML 404 for browsers", async () => {
  const res = await get("/__missing", "text/html");
  assert.equal(res.status, 404);
  assert.match(res.headers.get("Content-Type"), /text\/html/);
});

test("non-page files pass through untouched", async () => {
  const res = await get("/llms.txt", "text/markdown");
  assert.equal(res.status, 200);
  assert.equal(res.headers.get("Vary"), "Accept-Encoding");
});
