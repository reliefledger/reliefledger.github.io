#!/usr/bin/env bash
# Export the zone's current DNS records to pulumi/dns-records.json so the
# Pulumi program can import them unchanged. Read-only: it never writes to
# Cloudflare. Review the diff before committing; the file holds no secrets.
#
# Usage (from pulumi/):  CLOUDFLARE_API_TOKEN=... scripts/export-dns.sh
set -euo pipefail

zone_name="${1:-reliefledger.pro}"
out="${2:-dns-records.json}"
: "${CLOUDFLARE_API_TOKEN:?export CLOUDFLARE_API_TOKEN first}"

api() {
  curl -fsS -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" "https://api.cloudflare.com/client/v4$1"
}

zone_id="$(api "/zones?name=${zone_name}" | jq -er '.result[0].id')"

api "/zones/${zone_id}/dns_records?per_page=500" | jq -e '
  if .result_info.total_count > (.result | length) then error("more records than one page") else . end
  | [.result[]
     | {id, type, name, content, ttl, proxied}
       + (if .priority != null then {priority} else {} end)
       + (if (.comment // "") != "" then {comment} else {} end)]
  | sort_by(.type, .name, .content)' > "${out}"

echo "Wrote $(jq length "${out}") records for ${zone_name} to ${out}"
