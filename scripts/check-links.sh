#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

failed=0
while IFS= read -r file; do
  while IFS= read -r raw; do
    target=${raw#*](}
    target=${target%)}
    target=${target#<}; target=${target%>}
    target=${target%%#*}
    target=${target//%20/ }
    [[ -z $target || $target == \#* || $target == *://* || $target == mailto:* ]] && continue
    path="$(dirname "$file")/$target"
    if [[ ! -e $path ]]; then
      echo "$file: missing local link target '$target'" >&2
      failed=1
    fi
  done < <(grep -oE '\[[^]]*\]\([^)]+\)' "$file" || true)
done < <(git ls-files '*.md')
exit "$failed"
