#!/usr/bin/env bash

set -uo pipefail

threshold_days=7
threshold_secs=$((threshold_days * 24 * 60 * 60))
bin_dir="${HOME:-/home/polis}/.local/bin"
source_root="/home/polis/tools"

tools=(
  br
  gate
  relay
  work
  argus
  loop
  truthsayer
  oathkeeper
)

repos=(
  beads-polis
  gate
  relay
  work
  argus
  learning-loop
  truthsayer
  oathkeeper
)

format_timestamp() {
  local ts="$1"
  if [[ "$ts" == "-" ]]; then
    printf '%s' "$ts"
    return
  fi

  date -u -d "@$ts" '+%Y-%m-%d %H:%M:%S UTC'
}

format_duration() {
  local total_secs="$1"
  if (( total_secs < 0 )); then
    total_secs=$(( -total_secs ))
  fi

  local days=$(( total_secs / 86400 ))
  local hours=$(( (total_secs % 86400) / 3600 ))
  local minutes=$(( (total_secs % 3600) / 60 ))

  if (( days > 0 )); then
    printf '%dd %dh' "$days" "$hours"
    return
  fi

  if (( hours > 0 )); then
    printf '%dh %dm' "$hours" "$minutes"
    return
  fi

  printf '%dm' "$minutes"
}

failures=0

printf 'Binary freshness report (stale if binary trails source HEAD by more than %d days)\n' "$threshold_days"
printf '%-7s %-12s %-24s %-24s %s\n' "STATUS" "BINARY" "INSTALLED" "SOURCE_HEAD" "DETAIL"

for i in "${!tools[@]}"; do
  tool="${tools[$i]}"
  repo="${repos[$i]}"
  bin_path="${bin_dir}/${tool}"
  repo_path="${source_root}/${repo}"

  status="FRESH"
  bin_ts="-"
  repo_ts="-"
  detail=""

  if [[ ! -e "$bin_path" ]]; then
    status="ERROR"
    detail="missing binary at ${bin_path}"
    failures=1
  elif ! bin_ts="$(stat -c %Y "$bin_path" 2>/dev/null)"; then
    status="ERROR"
    bin_ts="-"
    detail="failed to read mtime for ${bin_path}"
    failures=1
  elif ! repo_ts="$(git -C "$repo_path" log -1 --format=%ct 2>/dev/null)"; then
    status="ERROR"
    repo_ts="-"
    detail="failed to read HEAD commit time for ${repo_path}"
    failures=1
  elif [[ -z "$repo_ts" ]]; then
    status="ERROR"
    repo_ts="-"
    detail="no HEAD commit found for ${repo_path}"
    failures=1
  else
    lag_secs=$(( repo_ts - bin_ts ))
    if (( lag_secs > threshold_secs )); then
      status="STALE"
      detail="$(format_duration "$lag_secs") behind source HEAD"
      failures=1
    elif (( lag_secs > 0 )); then
      detail="$(format_duration "$lag_secs") behind source HEAD"
    elif (( lag_secs < 0 )); then
      detail="$(format_duration "$lag_secs") newer than source HEAD"
    else
      detail="matches source HEAD timestamp"
    fi
  fi

  printf '%-7s %-12s %-24s %-24s %s\n' \
    "$status" \
    "$tool" \
    "$(format_timestamp "$bin_ts")" \
    "$(format_timestamp "$repo_ts")" \
    "$detail"
done

if (( failures != 0 )); then
  printf 'Result: FAIL\n'
  exit 1
fi

printf 'Result: PASS\n'
