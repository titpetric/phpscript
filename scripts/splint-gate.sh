#!/usr/bin/env bash
#
# splint-gate runs splint over the tree and turns its findings into a gate.
#
# splint exits 1 when it found anything at all, warnings included, and this tree
# carries hundreds of them: an uncovered symbol, a file whose name does not match
# the symbols in it, a source file with no test beside it. Those are conventions
# the repository is working towards, so a job that fails on them fails always and
# gates nothing.
#
# What gates is the error class. splint reports one severity as ERROR - a test
# file with no source of that name beside it - and this script fails when that
# count grows, or when a rule that was not an error before becomes one. The
# warnings are printed with their movement so a change that adds twenty is
# visible in the job output, and they do not fail the run.
#
# scripts/splint-baseline.json holds the counts a run is compared against. A
# count that went down is reported, not failed: lower the baseline in the same
# commit that lowered the finding, and when an error rule reaches zero take it
# out of the baseline, which is what makes it blocking from then on.
set -u -o pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
baseline="${root}/scripts/splint-baseline.json"
pattern="${1:-./...}"

findings="$(mktemp)"
trap 'rm -f "${findings}"' EXIT

# splint's own exit status is not read: it says "something was found", which is
# always true here. The findings are what this reads.
splint --json "${pattern}" > "${findings}" 2>/dev/null
if ! jq -e '.Issues' "${findings}" > /dev/null 2>&1; then
	echo "splint-gate: splint wrote no findings for ${pattern}" >&2
	exit 2
fi

if [ ! -f "${baseline}" ]; then
	echo "splint-gate: no baseline at ${baseline}" >&2
	exit 2
fi

report="$(jq -r --slurpfile base "${baseline}" '
	($base[0].rules // {}) as $baseline
	| [.Issues[] | {rule: (.Linter + "/" + .Rule), sev: .Severity}]
	| group_by(.rule)
	| map({rule: .[0].rule, sev: .[0].sev, count: length})
	| (map(.rule) | map({key: ., value: true}) | from_entries) as $seen
	| . + ($baseline | to_entries | map(select($seen[.key] | not) | {rule: .key, sev: "GONE", count: 0}))
	| sort_by(.rule)
	| map(. + {base: ($baseline[.rule] // 0)})
	| map(. + {delta: (.count - .base)})
	| .[]
	| [.sev, (.count | tostring), (.base | tostring), (.delta | tostring), .rule]
	| @tsv
' "${findings}")"

printf '%-6s %7s %8s %6s  %s\n' SEVERITY COUNT BASELINE DELTA RULE
printf '%s\n' "${report}" | while IFS=$'\t' read -r sev count base delta rule; do
	[ -n "${rule}" ] || continue
	printf '%-6s %7s %8s %+6s  %s\n' "${sev}" "${count}" "${base}" "${delta}" "${rule}"
done

# A rule splint calls an error may not grow, and a rule that is an error and is
# not in the baseline at all may not appear.
broke="$(printf '%s\n' "${report}" | awk -F'\t' '$1 == "ERROR" && $4 > 0 { print $5 " grew by " $4 " to " $2 }')"
if [ -n "${broke}" ]; then
	printf '\nsplint-gate: %s\n' "${broke}" >&2
	echo "splint-gate: fix the finding, or say why in the commit and raise the baseline" >&2
	exit 1
fi

improved="$(printf '%s\n' "${report}" | awk -F'\t' '$4 < 0 { print $5 }' | tr '\n' ' ')"
if [ -n "${improved}" ]; then
	printf '\nsplint-gate: below baseline, lower scripts/splint-baseline.json: %s\n' "${improved}"
fi
echo "splint-gate: no error class grew"
