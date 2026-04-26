#!/bin/sh

set -eu

usage() {
    cat <<'EOF'
Usage:
  sh scripts/profile-pprof.sh [options]

Options:
  --addr URL            pprof base URL (default: http://127.0.0.1:6060)
  --bin PATH            binary to use for stack symbolization (default: auto-detect)
  --steps N             number of capture steps (default: 3)
  --labels a,b,c        comma-separated labels for steps
  --sleep SECONDS       auto-capture every N seconds instead of waiting for Enter
  --top N               show top N goroutine groups and heap allocators (default: 8)
  --out DIR             output directory for snapshots and reports
  --keep-raw            keep raw pprof files in output directory
  --strict-exit         exit 1 when analysis finds a likely leak candidate
  -h, --help            show this help

Examples:
  sh scripts/profile-pprof.sh --steps 3 --labels baseline,load,after_restart
  sh scripts/profile-pprof.sh --addr http://127.0.0.1:6060 --steps 4 --sleep 15
EOF
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        printf '%s\n' "missing required command: $1" >&2
        exit 1
    fi
}

sanitize_label() {
    printf '%s' "$1" | tr ' /' '__' | tr -cd '[:alnum:]_.-'
}

step_label() {
    _step="$1"
    if [ -n "$LABELS" ]; then
        printf '%s' "$LABELS" | awk -F',' -v idx="$_step" '
            {
                if (idx <= NF) {
                    gsub(/^[[:space:]]+|[[:space:]]+$/, "", $idx)
                    print $idx
                }
            }
        '
        return 0
    fi
    printf 'step-%s' "$_step"
}

capture_index_metric() {
    _html="$1"
    _name="$2"
    awk -v key="$_name" '
        index($0, "href='\''" key "?debug=1'\''") > 0 {
            line = $0
            gsub(/[^0-9]/, "", line)
            print line
            found = 1
        }
        END {
            if (!found) {
                print 0
            }
        }
    ' "$_html"
}

detect_profile_bin() {
    if [ -n "$PROFILE_BIN" ] && [ -x "$PROFILE_BIN" ]; then
        printf '%s' "$PROFILE_BIN"
        return 0
    fi
    if [ -n "${BIN_PATH:-}" ] && [ -x "${BIN_PATH:-}" ]; then
        printf '%s' "$BIN_PATH"
        return 0
    fi
    if [ -x "./tg-ws-proxy" ]; then
        printf '%s' "./tg-ws-proxy"
        return 0
    fi
    return 1
}

normalize_symbol_name() {
    printf '%s' "$1" | sed \
        -e 's/ (in [^)]*)//g' \
        -e 's/[[:space:]]\++[[:space:]][0-9][0-9]*$//' \
        -e 's/+0x[[:xdigit:]]\{1,\}$//'
}

resolve_goroutine_symbols() {
    _in="$1"
    _out="$2"

    : >"$_out"

    _bin="$(detect_profile_bin 2>/dev/null || true)"
    [ -n "$_bin" ] || return 0

    if command -v atos >/dev/null 2>&1; then
        awk '
            {
                for (i = 1; i <= NF; i++) {
                    if ($i ~ /^0x[[:xdigit:]]+$/ && !seen[$i]++) {
                        print $i
                    }
                }
            }
        ' "$_in" | while IFS= read -r _addr; do
            [ -n "$_addr" ] || continue
            _symbol="$(atos -o "$_bin" "$_addr" 2>/dev/null || true)"
            [ -n "$_symbol" ] || continue
            _symbol="$(normalize_symbol_name "$_symbol")"
            [ -n "$_symbol" ] || continue
            printf '%s\t%s\n' "$_addr" "$_symbol"
        done >"$_out"
    fi
}

summarize_goroutines() {
    _in="$1"
    _symmap="$2"
    _out="$3"
    awk -v symmap="$_symmap" '
        BEGIN {
            RS = ""
            FS = "\n"
            while ((getline line < symmap) > 0) {
                split(line, parts, "\t")
                addr = parts[1]
                sub(/^[^\t]+\t/, "", line)
                syms[addr] = line
            }
            close(symmap)
        }
        function trim(s) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
            return s
        }
        function clean_group(s) {
            s = trim(s)
            sub(/^# */, "", s)
            sub(/^created by /, "", s)
            sub(/ \(in [^)]*\).*/, "", s)
            sub(/[[:space:]]\++.*/, "", s)
            sub(/\+.*/, "", s)
            return trim(s)
        }
        function classify_line(line,   count, fields, idx, token, addr, candidate) {
            line = trim(line)
            if (line == "" || index(line, "created by ") == 1) {
                return ""
            }
            count = split(line, fields, "\t")
            for (idx = 1; idx <= count; idx++) {
                token = clean_group(fields[idx])
                if (token == "" || token == "#") {
                    continue
                }
                if (token ~ /^0x[[:xdigit:]]+$/) {
                    addr = token
                    continue
                }
                if (token ~ /^tg-ws-proxy\//) {
                    return token
                }
                if (candidate == "" && token ~ /[[:alpha:]_]/) {
                    candidate = token
                }
            }
            if (candidate != "") {
                return candidate
            }
            if (addr != "" && (addr in syms)) {
                return clean_group(syms[addr])
            }
            return ""
        }
        NR == 1 {
            next
        }
        {
            count = 0
            if (match($1, /^([0-9]+) @/)) {
                count = substr($1, 1, RLENGTH)
                sub(/ @.*/, "", count)
            }
            if (count == 0) {
                next
            }

            group = "runtime/other"
            for (i = 2; i <= NF; i++) {
                candidate = classify_line($i)
                if (candidate == "") {
                    continue
                }
                if (index(candidate, "tg-ws-proxy/") == 1) {
                    group = candidate
                    break
                }
                if (group == "runtime/other") {
                    group = candidate
                }
            }
            counts[group] += count
        }
        END {
            for (group in counts) {
                print counts[group] "\t" group
            }
        }
    ' "$_in" | sort -nr >"$_out"
}

extract_goroutine_total() {
    awk '
        NR == 1 {
            if (match($0, /total [0-9]+/)) {
                value = substr($0, RSTART, RLENGTH)
                sub(/total /, "", value)
                print value
                exit
            }
        }
    ' "$1"
}

write_heap_top() {
    _heap="$1"
    _sample="$2"
    _out="$3"
    if ! command -v go >/dev/null 2>&1; then
        printf 'go tool pprof is unavailable\n' >"$_out"
        return 0
    fi
    go tool pprof -top -sample_index="$_sample" -nodecount="$TOP" "$_heap" >"$_out" 2>&1 || true
}

extract_pprof_total() {
    awk '
        / total$/ {
            total = $(NF-1)
        }
        END {
            if (total == "") {
                print "n/a"
            } else {
                print total
            }
        }
    ' "$1"
}

filter_app_allocators() {
    _in="$1"
    _out="$2"
    awk '
        index($0, "tg-ws-proxy/") > 0 {
            print
            count++
            if (count >= limit) {
                exit
            }
        }
    ' limit="$TOP" "$_in" >"$_out"
}

write_delta() {
    _before="$1"
    _after="$2"
    _out="$3"
    awk '
        NR == FNR {
            before[$2] = $1
            keys[$2] = 1
            next
        }
        {
            after[$2] = $1
            keys[$2] = 1
        }
        END {
            for (key in keys) {
                delta = after[key] - before[key]
                if (delta != 0) {
                    abs = delta < 0 ? -delta : delta
                    print abs "\t" delta "\t" after[key] "\t" before[key] "\t" key
                }
            }
        }
    ' "$_before" "$_after" | sort -nr | awk -F'\t' '
        NR <= limit {
            printf "%s\t%s\t%s\t%s\n", $2, $3, $4, $5
        }
    ' limit="$TOP" >"$_out"
}

print_step_summary() {
    _step="$1"
    _label="$2"
    _dir="$3"

    goroutines="$(cat "$_dir/goroutines_total.txt")"
    heap_space="$(cat "$_dir/heap_space_total.txt")"
    heap_objects="$(cat "$_dir/heap_objects_total.txt")"
    index_heap="$(cat "$_dir/index_heap_count.txt")"
    index_threads="$(cat "$_dir/index_threadcreate_count.txt")"

    printf 'Step %s [%s]\n' "$_step" "$_label"
    printf '  goroutines total : %s\n' "$goroutines"
    printf '  heap inuse space : %s\n' "$heap_space"
    printf '  heap inuse objs  : %s\n' "$heap_objects"
    printf '  heap profile rows: %s\n' "$index_heap"
    printf '  threads created  : %s\n' "$index_threads"
    printf '  top goroutine groups:\n'
    if [ -s "$_dir/goroutine_groups_top.txt" ]; then
        sed 's/^/    /' "$_dir/goroutine_groups_top.txt"
    else
        printf '    none\n'
    fi
    printf '  top heap allocators:\n'
    if [ -s "$_dir/heap_space_app_top.txt" ]; then
        sed 's/^/    /' "$_dir/heap_space_app_top.txt"
    else
        printf '    none from tg-ws-proxy package frames\n'
    fi
}

analyze_lingering_groups() {
    _before="$1"
    _after="$2"
    awk '
        NR == FNR {
            before[$2] = $1
            keys[$2] = 1
            next
        }
        {
            after[$2] = $1
            keys[$2] = 1
        }
        END {
            for (key in keys) {
                delta = after[key] - before[key]
                if (delta > 0) {
                    print delta "\t" after[key] "\t" before[key] "\t" key
                }
            }
        }
    ' "$_before" "$_after" | sort -nr
}

analyze_run() {
    _out="$1"
    _first_dir="$2"
    _last_dir="$3"

    _base_g="$(cat "$_first_dir/goroutines_total.txt")"
    _final_g="$(cat "$_last_dir/goroutines_total.txt")"
    _base_threads="$(cat "$_first_dir/index_threadcreate_count.txt")"
    _final_threads="$(cat "$_last_dir/index_threadcreate_count.txt")"
    _g_delta=$((_final_g - _base_g))
    _thread_delta=$((_final_threads - _base_threads))

    analyze_lingering_groups "$_first_dir/goroutine_groups.txt" "$_last_dir/goroutine_groups.txt" >"$_out"

    _lingering_count="$(awk 'END{print NR+0}' "$_out")"
    _lingering_sum="$(awk '{sum += $1} END {print sum+0}' "$_out")"
    _wsbridge_sum="$(awk 'index($4, "internal/wsbridge.") > 0 {sum += $1} END {print sum+0}' "$_out")"
    _socks5_sum="$(awk 'index($4, "internal/socks5.") > 0 {sum += $1} END {print sum+0}' "$_out")"

    _status="stable"
    _reason="no positive goroutine delta relative to baseline"

    if [ "$_g_delta" -gt 0 ] || [ "$_lingering_sum" -gt 0 ]; then
        _status="suspicious"
        _reason="final goroutine count or grouped stacks stayed above baseline"
    fi

    if [ "$_g_delta" -ge 10 ] || [ "$_lingering_sum" -ge 10 ]; then
        _status="likely leak candidate"
        _reason="meaningful goroutine growth remained by the final step"
    fi

    if [ "$_wsbridge_sum" -ge 4 ] || [ "$_socks5_sum" -ge 4 ]; then
        _status="likely leak candidate"
        _reason="application goroutines in wsbridge/socks5 remained above baseline"
    fi

    printf 'analysis\n'
    printf '  status            : %s\n' "$_status"
    printf '  reason            : %s\n' "$_reason"
    printf '  baseline goroutines: %s\n' "$_base_g"
    printf '  final goroutines   : %s\n' "$_final_g"
    printf '  goroutine delta    : %+d\n' "$_g_delta"
    printf '  threadcreate delta : %+d\n' "$_thread_delta"
    printf '  lingering groups   : %s\n' "$_lingering_count"
    printf '  lingering sum      : %s\n' "$_lingering_sum"
    printf '  wsbridge lingering : %s\n' "$_wsbridge_sum"
    printf '  socks5 lingering   : %s\n' "$_socks5_sum"
    printf '  verdict            : '
    case "$_status" in
        stable)
            printf 'no strong leak signal in captured steps\n'
            ;;
        suspicious)
            printf 'repeat with one post-load cooldown step to confirm cleanup\n'
            ;;
        *)
            printf 'inspect lingering groups first, especially wsbridge/socks5\n'
            ;;
    esac
    if [ -s "$_out" ]; then
        printf '  lingering details:\n'
        awk -F'\t' '
            NR <= limit {
                printf "    delta=%+d now=%s prev=%s %s\n", $1, $2, $3, $4
            }
        ' limit="$TOP" "$_out"
    fi

    if [ "$_status" = "likely leak candidate" ]; then
        return 2
    fi
    if [ "$_status" = "suspicious" ]; then
        return 1
    fi
    return 0
}

label_role() {
    _label_lc="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
    case "$_label_lc" in
        *baseline*)
            printf 'baseline'
            ;;
        *cooldown*|*after_stop*|*after-stop*|*afterstop*|*idle*|*settled*)
            printf 'cooldown'
            ;;
        *restart*|*restarted*|*after_restart*|*after-restart*)
            printf 'restart'
            ;;
        *load*|*traffic*|*stress*|*busy*)
            printf 'load'
            ;;
        *)
            printf 'other'
            ;;
    esac
}

find_first_step_by_role() {
    _wanted="$1"
    _step=1
    while [ "$_step" -le "$STEPS" ]; do
        _label="$(step_label "$_step")"
        if [ "$(label_role "$_label")" = "$_wanted" ]; then
            printf '%s' "$_step"
            return 0
        fi
        _step=$((_step + 1))
    done
    return 1
}

find_next_step_by_role() {
    _from="$1"
    _wanted="$2"
    _step=$((_from + 1))
    while [ "$_step" -le "$STEPS" ]; do
        _label="$(step_label "$_step")"
        if [ "$(label_role "$_label")" = "$_wanted" ]; then
            printf '%s' "$_step"
            return 0
        fi
        _step=$((_step + 1))
    done
    return 1
}

step_dir_for() {
    _step="$1"
    _label="$(step_label "$_step")"
    _safe="$(sanitize_label "$_label")"
    printf '%s/step-%02d-%s' "$OUT_DIR" "$_step" "$_safe"
}

compare_steps_line() {
    _from_step="$1"
    _to_step="$2"
    _title="$3"
    _from_dir="$(step_dir_for "$_from_step")"
    _to_dir="$(step_dir_for "$_to_step")"
    _from_label="$(step_label "$_from_step")"
    _to_label="$(step_label "$_to_step")"
    _from_g="$(cat "$_from_dir/goroutines_total.txt")"
    _to_g="$(cat "$_to_dir/goroutines_total.txt")"
    analyze_lingering_groups "$_from_dir/goroutine_groups.txt" "$_to_dir/goroutine_groups.txt" >"$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt"
    _lingering_sum="$(awk '{sum += $1} END {print sum+0}' "$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt")"
    _wsbridge_sum="$(awk 'index($4, "internal/wsbridge.") > 0 {sum += $1} END {print sum+0}' "$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt")"
    _socks5_sum="$(awk 'index($4, "internal/socks5.") > 0 {sum += $1} END {print sum+0}' "$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt")"
    _delta=$((_to_g - _from_g))
    _status="ok"
    _reason="cleanup returned close to the previous state"
    if [ "$_delta" -gt 0 ] || [ "$_lingering_sum" -gt 0 ]; then
        _status="suspicious"
        _reason="positive goroutine delta remained after transition"
    fi
    if [ "$_delta" -ge 10 ] || [ "$_lingering_sum" -ge 10 ] || [ "$_wsbridge_sum" -ge 4 ] || [ "$_socks5_sum" -ge 4 ]; then
        _status="likely leak candidate"
        _reason="application goroutines remained after expected cleanup"
    fi
    printf '  %s: %s -> %s delta=%+d lingering=%s wsbridge=%s socks5=%s status=%s\n' \
        "$_title" "$_from_label" "$_to_label" "$_delta" "$_lingering_sum" "$_wsbridge_sum" "$_socks5_sum" "$_status"
    printf '    reason: %s\n' "$_reason"
    if [ -s "$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt" ]; then
        awk -F'\t' 'NR <= limit { printf "    lingering delta=%+d %s\n", $1, $4 }' limit="$TOP" "$TMP_SUMMARY_DIR/scenario-${_from_step}-${_to_step}.txt"
    fi
    if [ "$_status" = "likely leak candidate" ]; then
        return 2
    fi
    if [ "$_status" = "suspicious" ]; then
        return 1
    fi
    return 0
}

analyze_scenarios() {
    _worst=0
    printf 'scenario analysis\n'
    _load_step="$(find_first_step_by_role load 2>/dev/null || true)"
    _cooldown_step=""
    _restart_step=""
    _post_restart_cooldown=""

    if [ -n "$_load_step" ]; then
        _cooldown_step="$(find_next_step_by_role "$_load_step" cooldown 2>/dev/null || true)"
    fi
    _restart_step="$(find_first_step_by_role restart 2>/dev/null || true)"
    if [ -n "$_restart_step" ]; then
        _post_restart_cooldown="$(find_next_step_by_role "$_restart_step" cooldown 2>/dev/null || true)"
    fi

    if [ -n "$_load_step" ] && [ -n "$_cooldown_step" ]; then
        if compare_steps_line "$_load_step" "$_cooldown_step" "load cleanup"; then
            :
        else
            _rc=$?
            [ "$_rc" -le "$_worst" ] || _worst="$_rc"
        fi
    else
        printf '  load cleanup: skipped, need labels with load and later cooldown\n'
    fi

    if [ -n "$_restart_step" ]; then
        _baseline_step="$(find_first_step_by_role baseline 2>/dev/null || true)"
        if [ -n "$_baseline_step" ]; then
            if compare_steps_line "$_baseline_step" "$_restart_step" "baseline to restart"; then
                :
            else
                _rc=$?
                [ "$_rc" -le "$_worst" ] || _worst="$_rc"
            fi
        fi
    else
        printf '  baseline to restart: skipped, need a restart label\n'
    fi

    if [ -n "$_restart_step" ] && [ -n "$_post_restart_cooldown" ]; then
        if compare_steps_line "$_restart_step" "$_post_restart_cooldown" "post-restart cleanup"; then
            :
        else
            _rc=$?
            [ "$_rc" -le "$_worst" ] || _worst="$_rc"
        fi
    else
        printf '  post-restart cleanup: skipped, need restart and later cooldown labels\n'
    fi

    return "$_worst"
}

ADDR="http://127.0.0.1:6060"
PROFILE_BIN=""
STEPS=3
LABELS=""
SLEEP_SECS=""
TOP=8
OUT_DIR=""
KEEP_RAW=0
STRICT_EXIT=0

while [ "$#" -gt 0 ]; do
    case "$1" in
        --addr)
            ADDR="${2:?missing value for --addr}"
            shift 2
            ;;
        --bin)
            PROFILE_BIN="${2:?missing value for --bin}"
            shift 2
            ;;
        --steps)
            STEPS="${2:?missing value for --steps}"
            shift 2
            ;;
        --labels)
            LABELS="${2:?missing value for --labels}"
            shift 2
            ;;
        --sleep)
            SLEEP_SECS="${2:?missing value for --sleep}"
            shift 2
            ;;
        --top)
            TOP="${2:?missing value for --top}"
            shift 2
            ;;
        --out)
            OUT_DIR="${2:?missing value for --out}"
            shift 2
            ;;
        --keep-raw)
            KEEP_RAW=1
            shift
            ;;
        --strict-exit)
            STRICT_EXIT=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            printf '%s\n' "unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

require_cmd curl
require_cmd awk
require_cmd sed
require_cmd sort

case "$STEPS" in
    ''|*[!0-9]*)
        printf '%s\n' '--steps must be a positive integer' >&2
        exit 1
        ;;
esac

case "$TOP" in
    ''|*[!0-9]*)
        printf '%s\n' '--top must be a positive integer' >&2
        exit 1
        ;;
esac

if [ "$STEPS" -le 0 ] || [ "$TOP" -le 0 ]; then
    printf '%s\n' '--steps and --top must be greater than zero' >&2
    exit 1
fi

if [ -n "$SLEEP_SECS" ]; then
    case "$SLEEP_SECS" in
        ''|*[!0-9]*)
            printf '%s\n' '--sleep must be a non-negative integer' >&2
            exit 1
            ;;
    esac
fi

if [ -z "$OUT_DIR" ]; then
    OUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/tg-ws-proxy-pprof.XXXXXX")"
else
    mkdir -p "$OUT_DIR"
fi

TMP_SUMMARY_DIR="$OUT_DIR/summary"
mkdir -p "$TMP_SUMMARY_DIR"

printf 'pprof profiler\n'
printf '  addr      : %s\n' "$ADDR"
printf '  steps     : %s\n' "$STEPS"
if [ -n "$SLEEP_SECS" ]; then
    printf '  mode      : auto every %ss\n' "$SLEEP_SECS"
else
    printf '  mode      : wait for Enter\n'
fi
printf '  out       : %s\n' "$OUT_DIR"

curl -fsS "$ADDR/debug/pprof/" >/dev/null

step=1
while [ "$step" -le "$STEPS" ]; do
    label="$(step_label "$step")"
    safe_label="$(sanitize_label "$label")"
    step_dir="$OUT_DIR/step-$(printf '%02d' "$step")-$safe_label"
    mkdir -p "$step_dir"

    if [ -n "$SLEEP_SECS" ]; then
        if [ "$step" -gt 1 ] || [ "$SLEEP_SECS" -gt 0 ]; then
            printf '\nwaiting %ss before step %s [%s]\n' "$SLEEP_SECS" "$step" "$label"
            sleep "$SLEEP_SECS"
        fi
    else
        printf '\nstep %s/%s [%s]\n' "$step" "$STEPS" "$label"
        printf 'press Enter when ready to capture... '
        IFS= read -r _unused || true
    fi

    printf 'capturing step %s [%s]\n' "$step" "$label"
    curl -fsS "$ADDR/debug/pprof/" >"$step_dir/index.html"
    curl -fsS "$ADDR/debug/pprof/goroutine?debug=1" >"$step_dir/goroutine.txt"
    curl -fsS "$ADDR/debug/pprof/heap?gc=1" >"$step_dir/heap.pb.gz"

    extract_goroutine_total "$step_dir/goroutine.txt" >"$step_dir/goroutines_total.txt"
    capture_index_metric "$step_dir/index.html" "heap" >"$step_dir/index_heap_count.txt"
    capture_index_metric "$step_dir/index.html" "threadcreate" >"$step_dir/index_threadcreate_count.txt"

    resolve_goroutine_symbols "$step_dir/goroutine.txt" "$step_dir/goroutine_symbols.txt"
    summarize_goroutines "$step_dir/goroutine.txt" "$step_dir/goroutine_symbols.txt" "$step_dir/goroutine_groups.txt"
    head -n "$TOP" "$step_dir/goroutine_groups.txt" >"$step_dir/goroutine_groups_top.txt"

    write_heap_top "$step_dir/heap.pb.gz" inuse_space "$step_dir/heap_space_top.txt"
    write_heap_top "$step_dir/heap.pb.gz" inuse_objects "$step_dir/heap_objects_top.txt"
    extract_pprof_total "$step_dir/heap_space_top.txt" >"$step_dir/heap_space_total.txt"
    extract_pprof_total "$step_dir/heap_objects_top.txt" >"$step_dir/heap_objects_total.txt"
    filter_app_allocators "$step_dir/heap_space_top.txt" "$step_dir/heap_space_app_top.txt"

    print_step_summary "$step" "$label" "$step_dir" | tee "$step_dir/summary.txt"

    if [ "$KEEP_RAW" -ne 1 ]; then
        rm -f "$step_dir/index.html" "$step_dir/goroutine.txt" "$step_dir/heap.pb.gz"
    fi

    step=$((step + 1))
done

REPORT="$OUT_DIR/report.txt"
FINAL_STATUS=0
{
    printf 'pprof comparison report\n'
    printf 'addr: %s\n' "$ADDR"
    printf 'steps: %s\n' "$STEPS"
    printf '\n'

    step=1
    while [ "$step" -le "$STEPS" ]; do
        label="$(step_label "$step")"
        safe_label="$(sanitize_label "$label")"
        step_dir="$OUT_DIR/step-$(printf '%02d' "$step")-$safe_label"
        print_step_summary "$step" "$label" "$step_dir"
        printf '\n'
        step=$((step + 1))
    done

    step=2
    while [ "$step" -le "$STEPS" ]; do
        prev=$((step - 1))
        prev_label="$(step_label "$prev")"
        prev_safe="$(sanitize_label "$prev_label")"
        prev_dir="$OUT_DIR/step-$(printf '%02d' "$prev")-$prev_safe"

        label="$(step_label "$step")"
        safe_label="$(sanitize_label "$label")"
        step_dir="$OUT_DIR/step-$(printf '%02d' "$step")-$safe_label"

        prev_g="$(cat "$prev_dir/goroutines_total.txt")"
        curr_g="$(cat "$step_dir/goroutines_total.txt")"
        prev_heap_space="$(cat "$prev_dir/heap_space_total.txt")"
        curr_heap_space="$(cat "$step_dir/heap_space_total.txt")"
        prev_heap_objects="$(cat "$prev_dir/heap_objects_total.txt")"
        curr_heap_objects="$(cat "$step_dir/heap_objects_total.txt")"
        prev_threads="$(cat "$prev_dir/index_threadcreate_count.txt")"
        curr_threads="$(cat "$step_dir/index_threadcreate_count.txt")"

        write_delta "$prev_dir/goroutine_groups.txt" "$step_dir/goroutine_groups.txt" "$TMP_SUMMARY_DIR/delta-$prev-$step.txt"

        printf '%s -> %s\n' "$prev_label" "$label"
        printf '  goroutines total : %s -> %s (delta %+d)\n' "$prev_g" "$curr_g" $((curr_g - prev_g))
        printf '  heap inuse space : %s -> %s\n' "$prev_heap_space" "$curr_heap_space"
        printf '  heap inuse objs  : %s -> %s\n' "$prev_heap_objects" "$curr_heap_objects"
        printf '  threads created  : %s -> %s (delta %+d)\n' "$prev_threads" "$curr_threads" $((curr_threads - prev_threads))
        printf '  goroutine group deltas:\n'
        if [ -s "$TMP_SUMMARY_DIR/delta-$prev-$step.txt" ]; then
            awk -F'\t' '
                {
                    printf "    delta=%+d now=%s prev=%s %s\n", $1, $2, $3, $4
                }
            ' "$TMP_SUMMARY_DIR/delta-$prev-$step.txt"
        else
            printf '    no group changes\n'
        fi
        printf '\n'

        step=$((step + 1))
    done

    if [ "$STEPS" -ge 2 ]; then
        first_label="$(step_label 1)"
        first_safe="$(sanitize_label "$first_label")"
        first_dir="$OUT_DIR/step-01-$first_safe"

        last_label="$(step_label "$STEPS")"
        last_safe="$(sanitize_label "$last_label")"
        last_dir="$OUT_DIR/step-$(printf '%02d' "$STEPS")-$last_safe"

        write_delta "$first_dir/goroutine_groups.txt" "$last_dir/goroutine_groups.txt" "$TMP_SUMMARY_DIR/delta-first-last.txt"

        printf 'baseline -> final lingering groups\n'
        if [ -s "$TMP_SUMMARY_DIR/delta-first-last.txt" ]; then
            awk -F'\t' '
                $1 > 0 {
                    printf "  delta=%+d now=%s prev=%s %s\n", $1, $2, $3, $4
                    found = 1
                }
                END {
                    if (!found) {
                        print "  no positive goroutine deltas relative to baseline"
                    }
                }
            ' "$TMP_SUMMARY_DIR/delta-first-last.txt"
        else
            printf '  no goroutine group changes between baseline and final step\n'
        fi

        printf '\n'
        if analyze_run "$TMP_SUMMARY_DIR/analysis-first-last.txt" "$first_dir" "$last_dir"; then
            :
        else
            FINAL_STATUS=$?
        fi
        printf '\n'
        if analyze_scenarios; then
            :
        else
            scenario_status=$?
            [ "$scenario_status" -lt "$FINAL_STATUS" ] || FINAL_STATUS="$scenario_status"
        fi
    fi
} | tee "$REPORT"

printf '\nreport saved to %s\n' "$REPORT"

if [ "$STRICT_EXIT" -eq 1 ] && [ "$FINAL_STATUS" -ge 2 ]; then
    exit 1
fi
