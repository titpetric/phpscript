#!/usr/bin/env bash
#
# vale-gate turns vale's findings into a gate, the way scripts/splint-gate.sh
# turns splint's into one.
#
# vale's own exit status reads the level of a rule and nothing else: it exits
# non-zero for an error and zero for a warning or a suggestion. Every llm-slop
# rule is level: error, so vale fails the build on those by itself. Three of
# write-good's are level: warning, so vale prints them and passes, which is a
# job that reports success over thousands of findings.
#
# scripts/vale-baseline.json holds the count each rule is allowed, and it only
# ever goes down. A rule over its baseline fails the run. A rule absent from the
# file may not appear at all, which is how every llm-slop rule and every
# write-good rule at zero stays blocking. A rule that reaches zero comes out of
# the file.
#
# Raising a number here hides the growth it records, so a commit that raises one
# says in its message which finding it is accepting and why.
set -u -o pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
baseline="${root}/scripts/vale-baseline.json"
target="${1:-.}"

findings="$(mktemp)"
trap 'rm -f "${findings}"' EXIT

# The same dockerised vale the mdox:lint task runs, so the gate and the job
# that prints the findings read the same build of the same rules.
docker run --rm -v "${root}:/src" -w /src jdkato/vale --output=line "${target}" \
	> "${findings}" 2>/dev/null

if [ ! -f "${baseline}" ]; then
	echo "vale-gate: no baseline at ${baseline}" >&2
	exit 2
fi

# A line is path:line:col:style.Rule:message, and the rule is the fourth field.
report="$(awk -F: '{ print $4 }' "${findings}" | sort | uniq -c \
	| awk '{ print $2 "\t" $1 }' \
	| jq -R -r -s --slurpfile base "${baseline}" '
	($base[0].rules // {}) as $baseline
	| [splits("\n") | select(length > 0) | split("\t") | {rule: .[0], count: (.[1] | tonumber)}]
	| (map(.rule) | map({key: ., value: true}) | from_entries) as $seen
	| . + ($baseline | to_entries | map(select($seen[.key] | not) | {rule: .key, count: 0}))
	| sort_by(.rule)
	| map(. + {base: ($baseline[.rule] // 0)})
	| map(. + {delta: (.count - .base)})
	| .[]
	| [(.count | tostring), (.base | tostring), (.delta | tostring), .rule]
	| @tsv
')"

printf '%7s %8s %6s  %s\n' COUNT BASELINE DELTA RULE
printf '%s\n' "${report}" | while IFS=$'\t' read -r count base delta rule; do
	[ -n "${rule}" ] || continue
	printf '%7s %8s %+6s  %s\n' "${count}" "${base}" "${delta}" "${rule}"
done

broke="$(printf '%s\n' "${report}" | awk -F'\t' '$3 > 0 { print $4 " grew by " $3 " to " $1 }')"
if [ -n "${broke}" ]; then
	printf '\nvale-gate: %s\n' "${broke}" >&2
	echo "vale-gate: fix the finding, or say why in the commit and raise the baseline" >&2
	exit 1
fi

improved="$(printf '%s\n' "${report}" | awk -F'\t' '$3 < 0 { print $4 }' | tr '\n' ' ')"
if [ -n "${improved}" ]; then
	printf '\nvale-gate: below baseline, lower scripts/vale-baseline.json: %s\n' "${improved}"
fi
echo "vale-gate: no rule grew"
