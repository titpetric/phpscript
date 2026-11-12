#!/usr/bin/env bash
#
# update-changelog writes the GitHub release note of a tag to
# build/release-notes/<tag>.md: the summary sentence, a mermaid pie of the
# commit categories and the changelog itself.
#
# scripts/release.sh sends the note to gh. A rerun fills the release text and
# builds no binary. `worktree changelog -from <prev> -to <tag> .` reports the
# tags that name no release and the gh command each one needs; this writes
# what that command reads from stdin.
#
# The summary sentence comes from claude, one call per tag, cached under
# build/release-notes/.focus. A tag with a cache file keeps the sentence it
# holds: pass --force to replace it, or NO_CLAUDE=1 for the sentence the
# category counts spell.
#
# Every count here drops the merge commits. The number the sentence states is
# the number of lines the changelog lists.
set -u -o pipefail

usage() {
	cat <<'EOF'
usage: scripts/update-changelog.sh [--force] [tag ...]

With no tag, every tag of the repository. Writes build/release-notes/<tag>.md.

  --force   ask claude again for a tag that already has a cached sentence

environment:
  NOTES_DIR    where the notes go (default build/release-notes)
  CLAUDE_BIN   the claude executable (default claude)
  NO_CLAUDE    set to 1 to write the sentence the category counts spell
EOF
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
notes_dir="${NOTES_DIR:-${root}/build/release-notes}"
claude_bin="${CLAUDE_BIN:-claude}"
force=0
name="$(basename "${root}")"

tags=()
for arg in "$@"; do
	case "${arg}" in
	--force) force=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		echo "update-changelog: unknown flag ${arg}" >&2
		usage >&2
		exit 2
		;;
	*) tags+=("${arg}") ;;
	esac
done

if [ "${#tags[@]}" -eq 0 ]; then
	mapfile -t tags < <(git -C "${root}" tag --sort=v:refname)
fi
if [ "${#tags[@]}" -eq 0 ]; then
	echo "update-changelog: the repository has no tag" >&2
	exit 1
fi

mkdir -p "${notes_dir}/.focus"

# previous_rev is the tag before this one, and the root commit for the first,
# which leaves the first tag a range to measure.
previous_rev() {
	local tag="$1" prev
	prev="$(git -C "${root}" tag --sort=v:refname | grep -x -B1 -- "${tag}" | head -n 1)"
	if [ -z "${prev}" ] || [ "${prev}" = "${tag}" ]; then
		git -C "${root}" rev-list --max-parents=0 "${tag}" | tail -n 1
		return
	fi
	printf '%s\n' "${prev}"
}

# category reads the conventional commit prefix of a subject, and falls back
# to the verb the subject opens on. A subject with neither reads as other.
category() {
	local subject="$1" type scope
	local conventional='^([a-zA-Z]+)(\(([^)]*)\))?!?:'
	if [[ "${subject}" =~ ${conventional} ]]; then
		type="$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')"
		scope="$(printf '%s' "${BASH_REMATCH[3]}" | tr '[:upper:]' '[:lower:]')"
		if [[ "${scope}" == *deps* ]]; then
			printf 'update\n'
			return
		fi
		case "${type}" in
		feat) printf 'feature\n' ;;
		fix) printf 'fix\n' ;;
		docs) printf 'docs\n' ;;
		test) printf 'test\n' ;;
		refactor) printf 'refactor\n' ;;
		perf) printf 'perf\n' ;;
		build) printf 'build\n' ;;
		chore) printf 'chore\n' ;;
		ci) printf 'ci\n' ;;
		style) printf 'style\n' ;;
		revert) printf 'revert\n' ;;
		deps) printf 'update\n' ;;
		*) printf 'other\n' ;;
		esac
		return
	fi
	shopt -s nocasematch
	case "${subject}" in
	add* | impl* | introduce* | support*) printf 'feature\n' ;;
	fix* | correct* | repair*) printf 'fix\n' ;;
	update* | upgrade* | bump*) printf 'update\n' ;;
	refactor* | rename* | extract* | split*) printf 'refactor\n' ;;
	optimi[sz]e* | speed*) printf 'perf\n' ;;
	test* | cover*) printf 'test\n' ;;
	doc* | readme*) printf 'docs\n' ;;
	*) printf 'other\n' ;;
	esac
	shopt -u nocasematch
}

# pie writes the mermaid block GitHub renders, the categories by descending
# count with "other" last whatever it counts.
pie() {
	local counts="$1" tag="$2"
	printf '```mermaid\n'
	printf 'pie title %s commits by category\n' "${tag}"
	grep -v '^other ' "${counts}" | sort -k2,2nr -k1,1 | while read -r label n; do
		printf '    "%s" : %s\n' "${label}" "${n}"
	done
	grep '^other ' "${counts}" | while read -r label n; do
		printf '    "%s" : %s\n' "${label}" "${n}"
	done
	printf '```\n'
}

# changelog lists the commits under a heading per category, the subject as the
# commit wrote it. The categories run in the pie's order.
changelog() {
	local prev="$1" tag="$2" counts="$3" label
	{
		grep -v '^other ' "${counts}" | sort -k2,2nr -k1,1
		grep '^other ' "${counts}"
	} | while read -r label _; do
		printf '### %s\n\n' "${label}"
		git -C "${root}" log --no-merges --format='%h %s' "${prev}..${tag}" | while IFS= read -r line; do
			if [ "$(category "${line#* }")" = "${label}" ]; then
				printf -- '- %s\n' "${line}"
			fi
		done
		printf '\n'
	done
}

# focus_sentence is the clause that completes "The changes focus on", from
# claude for a tag with no cached answer and from the counts otherwise.
focus_sentence() {
	local tag="$1" subjects="$2" counts="$3" cache="${notes_dir}/.focus/${tag}.txt"

	if [ "${force}" -eq 0 ] && [ -s "${cache}" ]; then
		cat "${cache}"
		return
	fi

	if [ "${NO_CLAUDE:-0}" != "1" ] && command -v "${claude_bin}" >/dev/null 2>&1; then
		local answer
		answer="$(
			{
				printf 'Read these git commit subjects from one release of %s, a PHP interpreter written in Go.\n\n' "${name}"
				cat "${subjects}"
				printf '\nWrite the clause that completes this sentence: "The changes focus on ". '
				printf 'Name the three or four largest themes in the commits, as noun phrases joined by commas and a final "and". '
				printf 'Name the subsystem and the feature the commits work on. '
				printf 'Write plain ASCII, no markdown, no trailing period, no preamble, under 40 words. '
				printf 'Answer with the clause alone.\n'
			} | "${claude_bin}" -p 2>/dev/null | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/\.$//' | grep -v '^$' | head -n 1
		)"
		if [ -n "${answer}" ]; then
			printf '%s\n' "${answer}" | tee "${cache}"
			return
		fi
		echo "update-changelog: claude wrote nothing for ${tag}, taking the counts" >&2
	fi

	local top
	top="$(grep -v '^other ' "${counts}" | sort -k2,2nr -k1,1 | head -n 3 | awk '{print $1}' | paste -sd, - | sed -e 's/,/, /g' -e 's/\(.*\), /\1 and /')"
	if [ -z "${top}" ]; then
		top="other work"
	fi
	printf '%s commits\n' "${top}" | tee "${cache}"
}

subjects="$(mktemp)"
counts="$(mktemp)"
trap 'rm -f "${subjects}" "${counts}"' EXIT

status=0
for tag in "${tags[@]}"; do
	if ! git -C "${root}" rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
		echo "update-changelog: no tag ${tag}" >&2
		status=1
		continue
	fi

	prev="$(previous_rev "${tag}")"
	git -C "${root}" log --no-merges --format='%s' "${prev}..${tag}" >"${subjects}"
	commits="$(wc -l <"${subjects}" | tr -d ' ')"

	: >"${counts}"
	while IFS= read -r subject; do
		category "${subject}"
	done <"${subjects}" | sort | uniq -c | awk '{print $2, $1}' >"${counts}"

	focus="$(focus_sentence "${tag}" "${subjects}" "${counts}")"

	# A tag reads as itself and the root commit as a short sha, the revision
	# the first release measures from.
	if git -C "${root}" rev-parse -q --verify "refs/tags/${prev}" >/dev/null; then
		shown="${prev}"
	else
		shown="$(git -C "${root}" rev-parse --short "${prev}")"
	fi

	noun="commits"
	[ "${commits}" = "1" ] && noun="commit"

	note="${notes_dir}/${tag}.md"
	{
		printf 'The %s release of %s contains %s %s since %s. The changes focus on %s.\n\n' \
			"${tag}" "${name}" "${commits}" "${noun}" "${shown}" "${focus}"
		pie "${counts}" "${tag}"
		printf '\n## Changelog\n\n'
		changelog "${prev}" "${tag}" "${counts}"
		printf '\n[%s...%s](https://github.com/titpetric/%s/compare/%s...%s)\n' \
			"${shown}" "${tag}" "${name}" "${prev}" "${tag}"
	} >"${note}"

	printf '%s: %s commits, %s categories -> %s\n' \
		"${tag}" "${commits}" "$(wc -l <"${counts}" | tr -d ' ')" "${note#"${root}/"}"
done

exit "${status}"
