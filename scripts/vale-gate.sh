#!/usr/bin/env bash
#
# vale-gate turns vale's findings into a gate. splint needs no such wrapper: it
# exits 1 on a finding and its tree is at zero.
#
# vale's own exit status reads the level of a rule and no more: it exits
# non-zero for an error and zero for a warning or a suggestion. Every llm-slop
# rule is level: error, and vale fails the build on those by itself. A rule at
# level: warning prints and passes, which is a job that reports success over
# thousands of findings.
#
# scripts/vale-baseline.json holds the count each rule may report, and the
# numbers only ever go down. A rule over its baseline fails the run. A rule
# absent from the file may not appear at all, which keeps every llm-slop rule
# and every write-good rule at zero blocking. A rule that reaches zero comes
# out of the file.
#
# Raising a number here hides the growth it records. A commit that raises one
# names in its message the finding it accepts and why.
set -u -o pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
baseline="${root}/scripts/vale-baseline.json"
target="${1:-.}"

findings="$(mktemp)"
files="$(mktemp)"
trap 'rm -f "${findings}" "${files}"' EXIT

# git decides which files the gate reads, because .gitignore already names
# every tree that is not ours to rewrite: a composer vendor tree, an agent
# worktree under .claude, a generated document. vale reads no .gitignore - its
# path controls are the section globs in .vale.ini, --glob, and the arguments
# on its command line - and this builds the list and passes it in.
#
# --cached --others --exclude-standard names the tracked files plus the
# untracked files no rule excludes, which is every file `git status` would show.
# A new document counts before `git add` sees it, and `composer install` changes
# no count. The extension filter is the one .vale.ini declares: a run scans what
# the configuration says it scans, and nothing here opens a file to find out.
git -C "${root}" ls-files --cached --others --exclude-standard -- "${target}" \
	| grep -E '\.(md|go|php|tpl|css|sh|sql)$' > "${files}"

if [ ! -s "${files}" ]; then
	echo "vale-gate: no file vale reads under ${target}" >&2
	exit 2
fi

# The same dockerised vale the mdox:lint task runs. The gate and the job that
# prints the findings read the same build of the same rules. xargs splits the
# list into as many invocations as the argument limit needs, and -r keeps an
# empty list from scanning the whole tree.
xargs -r -a "${files}" -d '\n' \
	docker run --rm -i -v "${root}:/src" -w /src jdkato/vale --output=line \
	> "${findings}" 2>/dev/null

if [ ! -f "${baseline}" ]; then
	echo "vale-gate: no baseline at ${baseline}" >&2
	exit 2
fi

# A line is path:line:col:style.Rule:message. The rule is the fourth field.
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

# NF and the +0 both matter on the empty report a clean tree against an empty
# baseline produces: awk compares an empty field as a string and "" sorts below
# "0", which left a bare `$3 < 0` reporting an improvement in a rule with no
# name.
broke="$(printf '%s\n' "${report}" | awk -F'\t' 'NF && $3+0 > 0 { print $4 " grew by " $3 " to " $1 }')"
if [ -n "${broke}" ]; then
	printf '\nvale-gate: %s\n' "${broke}" >&2
	echo "vale-gate: fix the finding, or say why in the commit and raise the baseline" >&2
	exit 1
fi

improved="$(printf '%s\n' "${report}" | awk -F'\t' 'NF && $3+0 < 0 { print $4 }' | tr '\n' ' ')"
if [ -n "${improved}" ]; then
	printf '\nvale-gate: below baseline, lower scripts/vale-baseline.json: %s\n' "${improved}"
fi
echo "vale-gate: no rule grew"
