# reliefledger.pro edge

Pulumi program for the Cloudflare side of reliefledger.pro:

- every DNS record in the zone, imported unchanged from `dns-records.json`
  and protected from deletion;
- the `reliefledger-markdown` Worker (`../edge/markdown-negotiation.mjs`) on
  `reliefledger.pro/*`, which answers `Accept: text/markdown` with the page's
  Markdown twin and Markdown 404s, and falls back to GitHub Pages on any error.

State lives in Pulumi Cloud. **This repository is public and GitHub Pages
serves every file, so no secret is ever committed here.** The Cloudflare token
exists only in your shell while you run these commands.

## One-time setup

1. In Cloudflare, create an API token scoped to the reliefledger.pro zone and
   its account with: Zone: Read, DNS: Edit, Workers Routes: Edit, and
   Account: Workers Scripts: Edit. Give it an expiry.
2. In your own terminal (not in a chat or a committed file):

   ```sh
   cd pulumi
   read -rs CLOUDFLARE_API_TOKEN && export CLOUDFLARE_API_TOKEN
   scripts/export-dns.sh            # read-only; writes dns-records.json
   git diff dns-records.json        # review, then commit it
   pulumi stack select production --create
   pulumi preview --diff
   ```

The first preview must show every DNS record as **import** with no changes,
plus two **create** steps: the Worker script and its route. Apply only after
the owner approves that preview: `pulumi up`.

## Verify after apply

```sh
curl -sS -i -H 'Accept: text/markdown' https://reliefledger.pro/            # 200, text/markdown, Vary: Accept
curl -sS -i -H 'Accept: text/html' https://reliefledger.pro/                # 200, HTML, Vary includes Accept
curl -sS -i -H 'Accept: text/markdown' https://reliefledger.pro/missing     # 404, text/markdown
```

## Roll back

`pulumi destroy --target 'urn:pulumi:production::reliefledger-site::cloudflare:index/workersRoute:WorkersRoute::reliefledger-markdown-route'`
removes the route and returns all traffic straight to GitHub Pages. DNS
records are protected and are not touched by this.
