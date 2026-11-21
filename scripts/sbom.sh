#!/usr/bin/env bash
#
# sbom writes a CycloneDX software bill of materials for one Go binary.
#
# The build metadata inside the binary names every module the linker kept. The
# document describes the file a reader downloads. The module graph in go.mod
# also names modules the binary never calls.
#
# usage: scripts/sbom.sh <binary> <output.json> [version]
#
# version names the main module's tag. License detection reads the module
# cache, and the main module downloads at that version. Without a version the
# document lists the modules and no license.
set -u -o pipefail

tool="github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.12.0"

if [ "$#" -lt 2 ]; then
	echo "usage: scripts/sbom.sh <binary> <output.json> [version]" >&2
	exit 2
fi

binary="$1"
output="$2"
version="${3:-}"

args=(bin -json -std -output "${output}")
if [ -n "${version}" ]; then
	args+=(-licenses -assert-licenses -version "${version}")
fi

if ! GOFLAGS= go run "${tool}" "${args[@]}" "${binary}"; then
	rm -f "${output}"
	echo "sbom: no document for ${binary}" >&2
	exit 1
fi
