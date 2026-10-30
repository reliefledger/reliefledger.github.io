"""Static checks for the public pages and machine-readable files."""

import json
import re
import unittest
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = "https://reliefledger.pro"
PAGES = {"index.html": f"{BASE}/", "about.html": f"{BASE}/about.html", "privacy.html": f"{BASE}/privacy.html"}
VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"}


class Page(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.stack, self.errors, self.meta, self.links, self.tags = [], [], {}, {}, set()
        self.lang, self.ld, self._in_ld, self.text = None, [], False, []
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        self.tags.add(tag)
        if tag == "html":
            self.lang = a.get("lang")
        if tag == "meta":
            self.meta[a.get("name") or a.get("property")] = a.get("content")
        if tag == "link":
            self.links.setdefault(a.get("rel"), []).append(a.get("href"))
        if tag == "script" and a.get("type") == "application/ld+json":
            self._in_ld = True
        if tag not in VOID:
            self.stack.append(tag)

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if self.stack and self.stack[-1] == tag:
            self.stack.pop()
        else:
            self.errors.append(tag)
        self._in_ld = False

    def handle_data(self, data):
        if self._in_ld:
            self.ld.append(json.loads(data))
        else:
            self.text.append(data)


def load(name):
    return Page((ROOT / name).read_text())


def graph(page):
    return [node for doc in page.ld for node in doc.get("@graph", [doc])]


class PagesTest(unittest.TestCase):
    def test_html_is_balanced(self):
        for name in [*PAGES, "404.html"]:
            page = load(name)
            self.assertEqual(page.errors, [], name)
            self.assertEqual(page.stack, [], name)

    def test_indexable_pages_have_complete_metadata(self):
        for name, url in PAGES.items():
            page = load(name)
            self.assertEqual(page.lang, "en", name)
            self.assertEqual(page.links.get("canonical"), [url], name)
            self.assertTrue(page.meta.get("description"), name)
            self.assertNotIn("robots", page.meta, f"{name} should be indexable")
            self.assertIn("main", page.tags, name)
            self.assertIn("nav", page.tags, name)
        home = load("index.html")
        self.assertEqual(home.meta["og:type"], "website")
        self.assertTrue(home.meta["og:image"].startswith(f"{BASE}/assets/"))
        self.assertTrue((ROOT / home.meta["og:image"].removeprefix(f"{BASE}/")).is_file())

    def test_organization_schema_is_complete(self):
        for name in PAGES:
            orgs = [n for n in graph(load(name)) if n["@type"] == "Organization"]
            self.assertEqual(len(orgs), 1, name)
            org = orgs[0]
            self.assertEqual(org["name"], "Sire Run, Inc.")
            self.assertEqual(org["address"]["@type"], "PostalAddress")
            contact = org["contactPoint"][0]
            self.assertEqual(contact["@type"], "ContactPoint")
            self.assertTrue(contact["contactType"] and contact["email"] and contact["telephone"])

    def test_homepage_describes_the_product(self):
        apps = [n for n in graph(load("index.html")) if n["@type"] == "SoftwareApplication"]
        self.assertEqual(len(apps), 1)
        for field in ("name", "description", "url", "applicationCategory"):
            self.assertTrue(apps[0][field], field)

    def test_about_page_has_substance(self):
        text = " ".join(" ".join(load("about.html").text).split())
        self.assertGreaterEqual(len(text), 500)
        self.assertIn("Sire Run, Inc.", text)

    def test_404_page_links_to_discovery_files(self):
        page = (ROOT / "404.html").read_text()
        for target in ("/sitemap.xml", "/llms.txt", "/about.html"):
            self.assertIn(f'href="{target}"', page)


class MachineFilesTest(unittest.TestCase):
    def test_sitemap_lists_every_indexable_page(self):
        ns = {"s": "http://www.sitemaps.org/schemas/sitemap/0.9"}
        root = ET.parse(ROOT / "sitemap.xml").getroot()
        urls = root.findall("s:url", ns)
        self.assertEqual(sorted(u.find("s:loc", ns).text for u in urls), sorted(PAGES.values()))
        for u in urls:
            self.assertRegex(u.find("s:lastmod", ns).text, r"^\d{4}-\d{2}-\d{2}$")

    def test_robots_points_to_sitemap(self):
        robots = (ROOT / "robots.txt").read_text()
        self.assertIn(f"Sitemap: {BASE}/sitemap.xml", robots)
        self.assertNotRegex(robots, r"(?m)^Disallow:\s*/\s*$")

    def test_llms_txt_follows_format_and_says_when_to_use(self):
        text = (ROOT / "llms.txt").read_text()
        lines = text.splitlines()
        self.assertTrue(lines[0].startswith("# "))
        self.assertTrue(any(l.startswith("> ") for l in lines[:4]))
        self.assertIn("## When to use ReliefLedger", text)
        self.assertRegex(text, r"\]\(https://reliefledger\.pro/")

    def test_markdown_twins_exist(self):
        for name in ("index.md", "about.md", "privacy.md", "404.md"):
            text = (ROOT / name).read_text()
            self.assertTrue(text.startswith("# "), name)
            self.assertGreater(len(text), 20, name)

    def test_markdown_and_html_agree_on_contact_details(self):
        for name in ("index.md", "about.md"):
            text = (ROOT / name).read_text()
            for fact in ("calendly.com/david-sire/30min", "david@reliefledger.pro", "Sire Run, Inc."):
                self.assertIn(fact, text, name)


if __name__ == "__main__":
    unittest.main()
