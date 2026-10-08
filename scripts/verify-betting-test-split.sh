#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root/backend"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

list_tests() {
  go test "$@" | sed -n '/^Test/p' | LC_ALL=C sort
}

list_tests -list '^Test' ./internal/betting >"$tmp_dir/all"
list_tests -list '^TestCommission' ./internal/betting >"$tmp_dir/commission"

if [[ ! -s "$tmp_dir/all" || ! -s "$tmp_dir/commission" ]]; then
  echo 'betting split verification requires tests in both groups' >&2
  exit 1
fi

if [[ "$(uniq -d "$tmp_dir/all")" != '' ]]; then
  echo 'go test -list returned duplicate betting test names' >&2
  exit 1
fi

if grep -Ev '^TestCommission' "$tmp_dir/commission" >/dev/null; then
  echo 'commission group contains a test outside ^TestCommission' >&2
  exit 1
fi

grep '^TestCommission' "$tmp_dir/all" >"$tmp_dir/expected-commission" || true
if ! diff -u "$tmp_dir/expected-commission" "$tmp_dir/commission"; then
  echo 'commission list does not match the ^TestCommission partition of the full inventory' >&2
  exit 1
fi

comm -23 "$tmp_dir/all" "$tmp_dir/commission" >"$tmp_dir/non-commission"
if [[ ! -s "$tmp_dir/non-commission" ]]; then
  echo 'betting split verification requires non-commission tests' >&2
  exit 1
fi

if comm -12 "$tmp_dir/commission" "$tmp_dir/non-commission" | grep -q .; then
  echo 'betting test groups overlap' >&2
  exit 1
fi

cat "$tmp_dir/commission" "$tmp_dir/non-commission" | LC_ALL=C sort -u >"$tmp_dir/union"
if ! diff -u "$tmp_dir/all" "$tmp_dir/union"; then
  echo 'betting test groups do not cover the complete go test -list inventory' >&2
  exit 1
fi

printf 'Verified betting test split: %s commission, %s non-commission, %s total.\n' \
  "$(wc -l <"$tmp_dir/commission" | tr -d ' ')" \
  "$(wc -l <"$tmp_dir/non-commission" | tr -d ' ')" \
  "$(wc -l <"$tmp_dir/all" | tr -d ' ')"
