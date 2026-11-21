#!/usr/bin/env bash
# Build the ToastStunt binary a manifest in profiles/toast pins, at
# toaststunt/build/moo under the current directory: the manifest's
# implementation_ref of MongooseMoo/toaststunt, options.h checked against its
# config_checksum, a Release build. The runner user cannot read the
# workstation's /root Toast build, so a job builds its own.
#
# Usage: build-toast-oracle.sh [MANIFEST]
#   MANIFEST  pinning manifest (default profiles/toast/stock-wsl-testdb.json,
#             the conformance oracle)
#
# A script rather than a local composite action: zizmor's self-repository
# audit (ci.yml "Audit discovery workflows") requires a same-repository
# action to be named as $/..., which actionlint 1.7.12, the newest release,
# rejects (rhysd/actionlint issues 711 and 780).
set -euo pipefail

manifest="${1:-profiles/toast/stock-wsl-testdb.json}"
read -r ref options < <(python3 -c 'import json, sys; m = json.load(open(sys.argv[1])); print(m["implementation_ref"], m["config_checksum"])' "$manifest")

rm -rf toaststunt
git init -q toaststunt
git -C toaststunt fetch -q --depth 1 https://github.com/MongooseMoo/toaststunt.git "$ref"
git -C toaststunt checkout -q --detach FETCH_HEAD
[ "$(git -C toaststunt rev-parse HEAD)" = "$ref" ] || { echo "fetched $(git -C toaststunt rev-parse HEAD), manifest pins $ref" >&2; exit 1; }

echo "$options  toaststunt/src/include/options.h" | sha256sum -c -
# CMakeLists.txt generates version_options.h in cmake's working directory,
# which must be the build directory to be included.
mkdir toaststunt/build
(cd toaststunt/build && cmake -DCMAKE_BUILD_TYPE=Release .. && cmake --build . -j2)
