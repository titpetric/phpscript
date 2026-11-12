#!/usr/bin/env bash
#
# release builds the binaries of a tag and publishes the GitHub release that
# holds them, under the note scripts/update-changelog.sh wrote.
#
# The build runs `go build` alone: no docker, no goreleaser, no cgo.
# CGO_ENABLED=0 with GOOS and GOARCH named outright leaves a static binary of
# the architecture the asset name claims, and verify() refuses one that is
# either dynamic or built for another machine.
#
# Each tag builds from its own source. The script keeps one detached worktree
# under build/release-src and checks each tag out there, which leaves the
# caller's working tree untouched.
#
# This absorbs the atkins release jobs. The asset name
# bin/<name>-<goos>-<goarch> comes from go:build, and `gh release create
# --verify-tag --title <tag> --latest=false --notes-file -` is the command
# `worktree changelog` prints for a tag that names no release.
set -u -o pipefail

usage() {
	cat <<'EOF'
usage: scripts/release.sh [--build-only] [--publish-only] [--dry-run] [--keep] [tag ...]

With no tag, every tag of the repository that names no GitHub release. Builds
build/release-bin/<tag>/<name>-<goos>-<goarch>, then creates or updates the
release and uploads the binaries.

  --build-only     build and verify, publish nothing
  --publish-only   publish the binaries already built, build nothing
  --dry-run        print the gh commands and run none of them
  --keep           keep the detached worktree the build read

environment:
  GOOS_LIST    the operating systems to build (default "linux")
  GOARCH_LIST  the architectures to build (default "amd64")
  REPO         the GitHub repository (default the origin remote)
EOF
}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
name="$(basename "${root}")"
goos_list="${GOOS_LIST:-linux}"
goarch_list="${GOARCH_LIST:-amd64}"
src_dir="${root}/build/release-src"
bin_root="${root}/build/release-bin"
notes_dir="${NOTES_DIR:-${root}/build/release-notes}"

build_only=0
publish_only=0
dry_run=0
keep=0
tags=()

for arg in "$@"; do
	case "${arg}" in
	--build-only) build_only=1 ;;
	--publish-only) publish_only=1 ;;
	--dry-run) dry_run=1 ;;
	--keep) keep=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		echo "release: unknown flag ${arg}" >&2
		usage >&2
		exit 2
		;;
	*) tags+=("${arg}") ;;
	esac
done

repository="${REPO:-$(git -C "${root}" remote get-url origin 2>/dev/null | sed -e 's#^git@github.com:##' -e 's#^https://github.com/##' -e 's#\.git$##')}"
if [ -z "${repository}" ]; then
	echo "release: no origin remote to name the repository; set REPO" >&2
	exit 1
fi

newest="$(git -C "${root}" tag --sort=v:refname | tail -n 1)"

# released reports whether the tag already has a GitHub release.
released() {
	gh release view "$1" --repo "${repository}" >/dev/null 2>&1
}

if [ "${#tags[@]}" -eq 0 ]; then
	mapfile -t all < <(git -C "${root}" tag --sort=v:refname)
	for tag in "${all[@]}"; do
		released "${tag}" || tags+=("${tag}")
	done
	if [ "${#tags[@]}" -eq 0 ]; then
		echo "release: every tag has a release"
		exit 0
	fi
	printf 'release: %s tags with no release: %s\n' "${#tags[@]}" "${tags[*]}"
fi

# file_arch is the architecture string file(1) prints for a GOARCH. A GOARCH
# with no entry fails the run, because a binary no check reads is a binary
# nobody should download.
file_arch() {
	case "$1" in
	amd64) printf 'x86-64\n' ;;
	arm64) printf 'ARM aarch64\n' ;;
	386) printf 'Intel 80386\n' ;;
	arm) printf 'ARM,\n' ;;
	*) return 1 ;;
	esac
}

# verify refuses a binary that is not static or not the architecture the asset
# name claims.
verify() {
	local path="$1" goarch="$2" want described
	if ! want="$(file_arch "${goarch}")"; then
		echo "release: no file(1) signature for GOARCH=${goarch}" >&2
		return 1
	fi

	described="$(file -b "${path}")"
	case "${described}" in
	*"statically linked"* | *"static-pie linked"*) ;;
	*)
		echo "release: ${path} is not static: ${described}" >&2
		return 1
		;;
	esac
	case "${described}" in
	*"${want}"*) ;;
	*)
		echo "release: ${path} is not ${goarch} (${want}): ${described}" >&2
		return 1
		;;
	esac
	# ldd reads the loader the binary names, where file(1) reads the header.
	# Two readings of one claim: an upload nobody can run fails here.
	if ldd "${path}" 2>&1 | grep -qv 'not a dynamic executable'; then
		echo "release: ${path} names a dynamic loader: $(ldd "${path}" 2>&1 | head -n 1)" >&2
		return 1
	fi
	printf '  %s: %s\n' "$(basename "${path}")" "${described}"
}

# prepare checks the tag out in the one worktree the build reads.
#
# The checkout forces and the clean follows it: go build writes go.mod and
# go.sum when a module needs resolving, and an edit left by the tag before
# would block the checkout or build the wrong source.
prepare() {
	local tag="$1"
	if [ ! -d "${src_dir}/.git" ] && [ ! -f "${src_dir}/.git" ]; then
		rm -rf "${src_dir}"
		git -C "${root}" worktree add -q --detach "${src_dir}" "${tag}" || return 1
		return
	fi
	git -C "${src_dir}" checkout -q -f --detach "${tag}" || return 1
	git -C "${src_dir}" clean -qfdx || return 1
}

# build writes one binary per GOOS and GOARCH and verifies each.
build() {
	local tag="$1" out="${bin_root}/${tag}" goos goarch target commit stamp branch
	mkdir -p "${out}"

	commit="$(git -C "${root}" rev-list -n 1 "${tag}")"
	stamp="$(git -C "${root}" log -1 --format=%cI "${tag}")"
	branch="$(git -C "${root}" branch --contains "${tag}" --format='%(refname:short)' 2>/dev/null | grep -x main || echo detached)"

	for goos in ${goos_list}; do
		for goarch in ${goarch_list}; do
			target="${out}/${name}-${goos}-${goarch}"
			(
				cd "${src_dir}" || exit 1
				CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
					-trimpath \
					-ldflags="-X 'main.Version=${tag}' -X 'main.Commit=${commit}' -X 'main.CommitTime=${stamp}' -X 'main.Branch=${branch}' -X 'main.Modified=0'" \
					-o "${target}" .
			) || return 1
			verify "${target}" "${goarch}" || return 1
		done
	done
}

# publish creates the release a tag names none of, replaces the note of the
# release a tag already has, and uploads every binary built for it.
publish() {
	local tag="$1" note="${notes_dir}/${tag}.md" latest=false
	if [ ! -s "${note}" ]; then
		"${root}/scripts/update-changelog.sh" "${tag}" >/dev/null || return 1
	fi
	[ "${tag}" = "${newest}" ] && latest=true

	local -a create=(gh release create "${tag}" --repo "${repository}" --verify-tag
		--title "${tag}" --latest="${latest}" --notes-file "${note}")
	local -a edit=(gh release edit "${tag}" --repo "${repository}" --notes-file "${note}")
	# A tag whose source fails to build still earns its note. The release
	# goes without the binary, and the run above said why.
	local -a upload=(gh release upload "${tag}" --repo "${repository}" --clobber)
	local goos goarch asset built=0
	for goos in ${goos_list}; do
		for goarch in ${goarch_list}; do
			asset="${bin_root}/${tag}/${name}-${goos}-${goarch}"
			if [ -s "${asset}" ]; then
				upload+=("${asset}")
				built=$((built + 1))
			fi
		done
	done
	if [ "${built}" -eq 0 ]; then
		echo "release: ${tag} has no binary to upload, publishing the note alone" >&2
	fi

	if [ "${dry_run}" -eq 1 ]; then
		released "${tag}" && printf '  %s\n' "${edit[*]}" || printf '  %s\n' "${create[*]}"
		printf '  %s\n' "${upload[*]}"
		return
	fi

	if released "${tag}"; then
		"${edit[@]}" >/dev/null || return 1
	else
		"${create[@]}" >/dev/null || return 1
	fi
	if [ "${built}" -gt 0 ]; then
		"${upload[@]}" >/dev/null || return 1
	fi
	printf '  https://github.com/%s/releases/tag/%s (%s binaries)\n' "${repository}" "${tag}" "${built}"
}

status=0
for tag in "${tags[@]}"; do
	if ! git -C "${root}" rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
		echo "release: no tag ${tag}" >&2
		status=1
		continue
	fi

	printf '%s\n' "${tag}"
	if [ "${publish_only}" -eq 0 ]; then
		if ! prepare "${tag}"; then
			echo "release: ${tag} would not check out" >&2
			status=1
			continue
		fi
		if ! build "${tag}"; then
			echo "release: ${tag} would not build" >&2
			status=1
			continue
		fi
	fi
	if [ "${build_only}" -eq 0 ]; then
		if ! publish "${tag}"; then
			echo "release: ${tag} would not publish" >&2
			status=1
			continue
		fi
	fi
done

if [ "${keep}" -eq 0 ] && [ -e "${src_dir}" ]; then
	git -C "${root}" worktree remove --force "${src_dir}" 2>/dev/null || rm -rf "${src_dir}"
fi

exit "${status}"
