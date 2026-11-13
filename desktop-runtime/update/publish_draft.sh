#!/usr/bin/env bash
# Run from the tagged checkout with a GitHub Actions release token.
set -euo pipefail
[[ $TAG =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "Invalid stable release tag" >&2; exit 1; }
jq -e --arg repo "$GITHUB_REPOSITORY" --arg version "${TAG#v}" \
  '.repository == $repo and .version == $version' release/desktop-channel.json >/dev/null
api="repos/$GITHUB_REPOSITORY"
releases=$(gh api --paginate "$api/releases?per_page=100" --slurp)
draft=$(jq -ce --arg tag "$TAG" '[.[][] | select(.tag_name == $tag and .draft and (.prerelease | not))] |
  if length == 1 then .[0] else error("Expected one matching stable release draft") end' <<< "$releases")
stable_tags=$(jq -r '.[][] | select((.draft or .prerelease) | not) | .tag_name | select(test("^v[0-9]+\\.[0-9]+\\.[0-9]+$"))' <<< "$releases")
newest=$(printf '%s\n' "$TAG" "$stable_tags" | sort -V | tail -1)
[[ $TAG == "$newest" ]] || { echo "A newer stable desktop release exists" >&2; exit 1; }

inventory() {
  jq -ce 'select(.draft and (.prerelease | not)) | .assets | sort_by(.name) |
    select(map(.name) == ["OpenX6-Windows11-x64-native.zip","OpenX6-macOS-arm64.zip","SHA256SUMS"]) |
    select(all(.[]; .state == "uploaded" and .size > 0 and .size <= 2147483648 and
      (.id | type == "number") and ((.digest // "") | test("^sha256:[0-9a-f]{64}$")))) |
    map({id,name,digest,size})'
}
assets=$(inventory <<< "$draft")
dir=$(mktemp -d)
trap 'rm -rf -- "$dir"' EXIT
while IFS=$'\t' read -r asset name digest size; do
  gh api -H 'Accept: application/octet-stream' "$api/releases/assets/$asset" > "$dir/$name"
  [[ $(wc -c < "$dir/$name") -eq $size ]]
  echo "${digest#sha256:}  $dir/$name" | sha256sum -c -
done < <(jq -r '.[] | [.id,.name,.digest,.size] | @tsv' <<< "$assets")
python3 desktop-runtime/update/check_release.py "$dir"
id=$(jq -er .id <<< "$draft")
current=$(gh api "$api/releases/$id")
[[ $(jq -er .tag_name <<< "$current") == "$TAG" && $(inventory <<< "$current") == "$assets" ]]
gh api --method PATCH "$api/releases/$id" -F draft=false -f make_latest=true >/dev/null
