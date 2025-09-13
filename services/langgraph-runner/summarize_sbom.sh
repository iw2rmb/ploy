#!/usr/bin/env bash
set -euo pipefail

SBOM_JSON="$1"
OUT_JSON="$2"

if [ ! -s "$SBOM_JSON" ]; then
  echo '{"error":"missing_sbom"}' >"$OUT_JSON"
  exit 0
fi

bytes=$(wc -c < "$SBOM_JSON" 2>/dev/null || echo 0)

# Extract up to 10 CVE IDs (simple heuristic)
cves=$(grep -o 'CVE-[0-9]\{4\}-[0-9]\{4,\}' "$SBOM_JSON" 2>/dev/null | sort -u | head -n 10 | tr '\n' ',' | sed 's/,$//')

# Extract up to 30 dependency names (heuristic on "name": "...")
deps=$(grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]\+"' "$SBOM_JSON" 2>/dev/null | sed 's/.*:"//;s/"$//' | sort -u | head -n 30 | tr '\n' ',' | sed 's/,$//')

# Extract up to 10 license strings
licenses=$(grep -o '"license"[[:space:]]*:[[:space:]]*"[^"]\+"' "$SBOM_JSON" 2>/dev/null | sed 's/.*:"//;s/"$//' | sort -u | head -n 10 | tr '\n' ',' | sed 's/,$//')

# Count approximate components/packages
components=$(grep -o '"packages"[[:space:]]*:' "$SBOM_JSON" 2>/dev/null | wc -l | tr -d ' ')

# Approximate severity tallies (if present)
crit=$(grep -oi '"severity"[[:space:]]*:[[:space:]]*"\(critical\|CRITICAL\)"' "$SBOM_JSON" 2>/dev/null | wc -l | tr -d ' ')
high=$(grep -oi '"severity"[[:space:]]*:[[:space:]]*"\(high\|HIGH\)"' "$SBOM_JSON" 2>/dev/null | wc -l | tr -d ' ')
med=$(grep -oi '"severity"[[:space:]]*:[[:space:]]*"\(medium\|MEDIUM\)"' "$SBOM_JSON" 2>/dev/null | wc -l | tr -d ' ')
low=$(grep -oi '"severity"[[:space:]]*:[[:space:]]*"\(low\|LOW\)"' "$SBOM_JSON" 2>/dev/null | wc -l | tr -d ' ')

cat >"$OUT_JSON" <<EOF
{
  "bytes": ${bytes},
  "components_hint": ${components},
  "vulns_hint": {"critical": ${crit}, "high": ${high}, "medium": ${med}, "low": ${low}},
  "top_cves": [${cves:+"${cves//,/","}"}],
  "top_names": [${deps:+"${deps//,/","}"}],
  "licenses": [${licenses:+"${licenses//,/","}"}]
}
EOF

exit 0
