# process.sh

runtime_bin_path() {
    if [ -n "$RUNTIME_BIN_OVERRIDE" ] && [ -x "$RUNTIME_BIN_OVERRIDE" ]; then
        printf "%s" "$RUNTIME_BIN_OVERRIDE"
        return 0
    fi

    if [ -x "$BIN_PATH" ]; then
        printf "%s" "$BIN_PATH"
        return 0
    fi

    bin="$(persistent_bin_path 2>/dev/null || true)"
    if [ -n "$bin" ] && [ -x "$bin" ]; then
        printf "%s" "$bin"
        return 0
    fi

    return 1
}

pid_matches_binary() {
    pid="$1"
    path="$2"
    [ -n "$pid" ] || return 1
    [ -n "$path" ] || return 1

    canonical_bin="$(canonical_path "$path")"
    proc_exe="$PROC_ROOT/$pid/exe"

    if [ -e "$proc_exe" ]; then
        proc_path="$(canonical_path "$proc_exe" 2>/dev/null || true)"
        [ -n "$proc_path" ] || return 1
        [ "$proc_path" = "$canonical_bin" ]
        return $?
    fi

    if command -v lsof >/dev/null 2>&1; then
        lsof -nP -a -p "$pid" -iTCP:"$LISTEN_PORT" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    fi

    if command -v ps >/dev/null 2>&1; then
        ps -p "$pid" -o command= 2>/dev/null | grep -F -- "$path" >/dev/null 2>&1
        return $?
    fi

    kill -0 "$pid" 2>/dev/null
}

matching_pids_for_path() {
    path="$1"
    [ -n "$path" ] || return 1

    matches=""

    pid_from_file="$(read_first_line "$PID_FILE" 2>/dev/null || true)"
    if [ -n "$pid_from_file" ] && pid_matches_binary "$pid_from_file" "$path"; then
        matches="${matches}${matches:+\\n}${pid_from_file}"
    fi

    if command -v pgrep >/dev/null 2>&1; then
        pids="$(pgrep -f "$path" 2>/dev/null || true)"
        for pid in $pids; do
            pid_matches_binary "$pid" "$path" || continue
            matches="${matches}${matches:+\\n}${pid}"
        done
    fi

    if command -v pidof >/dev/null 2>&1; then
        pids="$(pidof "$(basename "$path")" 2>/dev/null || true)"
        for pid in $pids; do
            pid_matches_binary "$pid" "$path" || continue
            matches="${matches}${matches:+\\n}${pid}"
        done
    fi

    [ -n "$matches" ] || return 1
    printf "%b\n" "$matches" | awk 'NF && !seen[$0]++'
}

is_running() {
    current_pids >/dev/null 2>&1
}

current_pids() {
    all_pids=""
    for path in "$(runtime_bin_path 2>/dev/null || true)" "$BIN_PATH" "$(persistent_bin_path 2>/dev/null || true)"; do
        [ -n "$path" ] || continue
        pids="$(matching_pids_for_path "$path" 2>/dev/null || true)"
        [ -n "$pids" ] || continue
        all_pids="${all_pids}${all_pids:+\\n}${pids}"
    done

    [ -n "$all_pids" ] || return 1
    printf "%b\n" "$all_pids" | awk 'NF && !seen[$0]++'
}

port_in_use() {
    if command -v lsof >/dev/null 2>&1; then
        lsof -nP -iTCP:"$LISTEN_PORT" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    fi

    if command -v ss >/dev/null 2>&1; then
        ss -ltn 2>/dev/null | awk -v p="$LISTEN_PORT" 'NR>1 {n=$4; sub(/^.*:/, "", n); if (n == p) found=1} END {exit(found ? 0 : 1)}' && return 0
    fi

    if command -v netstat >/dev/null 2>&1; then
        netstat -ltn 2>/dev/null | awk -v p="$LISTEN_PORT" 'NR>2 {n=$4; sub(/^.*:/, "", n); if (n == p) found=1} END {exit(found ? 0 : 1)}' && return 0
    fi

    return 1
}

named_proxy_pids() {
    path="$(runtime_bin_path 2>/dev/null || true)"
    [ -n "$path" ] || return 1
    name="$(basename "$path")"
    [ -n "$name" ] || return 1

    if command -v pidof >/dev/null 2>&1; then
        pids="$(pidof "$name" 2>/dev/null || true)"
        [ -n "$pids" ] || return 1
        printf "%s\n" "$pids"
        return 0
    fi

    if command -v pgrep >/dev/null 2>&1; then
        pids="$(pgrep -x "$name" 2>/dev/null || true)"
        [ -n "$pids" ] || return 1
        printf "%s\n" "$pids"
        return 0
    fi

    return 1
}

prompt_stop_detected_proxy_for_busy_port() {
    pids="$(named_proxy_pids 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]*$//' || true)"
    if [ -z "$pids" ]; then
        show_header
        printf "%sPort %s is already busy%s\n\n" "$C_RED" "$LISTEN_PORT" "$C_RESET"
        printf "Free the port first or change LISTEN_PORT\n"
        pause
        return 1
    fi

    show_header
    printf "%sPort %s is already busy%s\n\n" "$C_RED" "$LISTEN_PORT" "$C_RESET"
    printf "Detected running %s process: %s\n" "$APP_NAME" "$pids"
    printf "Stop it and try again? [y/N]: "
    IFS= read -r busy_choice

    case "$busy_choice" in
        y|Y|yes|YES)
            for pid in $pids; do
                kill "$pid" 2>/dev/null || true
            done
            sleep 1
            for pid in $pids; do
                if kill -0 "$pid" 2>/dev/null; then
                    kill -9 "$pid" 2>/dev/null || true
                fi
            done

            if port_in_use; then
                printf "\n%sPort %s is still busy%s\n\n" "$C_RED" "$LISTEN_PORT" "$C_RESET"
                printf "Free the port first or change LISTEN_PORT\n"
                pause
                return 1
            fi
            return 0
            ;;
        *)
            printf "\n"
            printf "Free the port first or change LISTEN_PORT\n"
            pause
            return 1
            ;;
    esac
}

prompt_restart_running_proxy() {
    pids="$(current_pids 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]*$//' || true)"
    [ -n "$pids" ] || return 1

    show_header
    printf "%s%s is already running%s\n\n" "$C_YELLOW" "$APP_NAME" "$C_RESET"
    printf "Detected running process: %s\n" "$pids"
    printf "Stop it and start again? [y/N]: "
    IFS= read -r running_choice

    case "$running_choice" in
        y|Y|yes|YES)
            stop_running >/dev/null 2>&1 || true
            if is_running; then
                printf "\n%sFailed to stop the running process%s\n" "$C_RED" "$C_RESET"
                pause
                return 1
            fi
            return 0
            ;;
        *)
            pause
            return 1
            ;;
    esac
}

stop_running() {
    if ! is_running; then
        rm -f "$PID_FILE"
        return 1
    fi

    pids="$(current_pids)"
    [ -n "$pids" ] || return 1

    for pid in $pids; do
        kill "$pid" 2>/dev/null
    done
    sleep 1

    for pid in $pids; do
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null
        fi
    done
    rm -f "$PID_FILE"
    return 0
}

# _run_proxy_cmd fg|bg
# Builds the full proxy command from current settings and executes it.
# fg: runs directly (blocking). bg: runs in background, prints PID to stdout.
_run_proxy_cmd() {
    _rpc_mode="$1"
    _rpc_bin="$(runtime_bin_path 2>/dev/null || true)"
    [ -n "$_rpc_bin" ] || return 1

    set -- "$_rpc_bin" --host "$LISTEN_HOST" --port "$LISTEN_PORT" --pool-size "$POOL_SIZE"

    if [ "$PROXY_MODE" = "mtproto" ]; then
        set -- "$@" --mode mtproto --secret "$MT_SECRET"
        if [ -n "$MT_LINK_IP" ]; then
            set -- "$@" --link-ip "$MT_LINK_IP"
        fi
    else
        if [ -n "$SOCKS_USERNAME" ] && [ -n "$SOCKS_PASSWORD" ]; then
            set -- "$@" --username "$SOCKS_USERNAME" --password "$SOCKS_PASSWORD"
        fi
    fi

    if [ "$VERBOSE" = "1" ]; then
        set -- "$@" --verbose
    fi

    if [ -n "$DC_IPS" ]; then
        _rpc_old_ifs="$IFS"
        IFS=','
        for _rpc_dc in $DC_IPS; do
            _rpc_dc="$(printf "%s" "$_rpc_dc" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
            [ -n "$_rpc_dc" ] || continue
            set -- "$@" --dc-ip "$_rpc_dc"
        done
        IFS="$_rpc_old_ifs"
    fi

    if [ "$CF_PROXY" = "1" ] && [ -n "$CF_DOMAIN" ]; then
        set -- "$@" --cf-proxy --cf-domain "$CF_DOMAIN"
        if [ "$CF_PROXY_FIRST" = "1" ]; then
            set -- "$@" --cf-proxy-first
        fi
    fi

    if [ "$PROXY_MODE" = "mtproto" ] && [ -n "$MT_UPSTREAM_PROXIES" ]; then
        _rpc_old_ifs="$IFS"
        IFS=','
        for _rpc_up in $MT_UPSTREAM_PROXIES; do
            _rpc_up="$(printf "%s" "$_rpc_up" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
            [ -n "$_rpc_up" ] || continue
            set -- "$@" --mtproto-proxy "$_rpc_up"
        done
        IFS="$_rpc_old_ifs"
    fi

    case "$_rpc_mode" in
        bg)
            if command -v nohup >/dev/null 2>&1; then
                nohup "$@" >/dev/null 2>&1 &
            else
                "$@" >/dev/null 2>&1 &
            fi
            printf "%s" "$!"
            ;;
        *)
            "$@"
            ;;
    esac
}

run_binary() {
    _run_proxy_cmd fg
}

run_binary_background() {
    _run_proxy_cmd bg
}

start_proxy() {
    if ! auth_settings_valid; then
        show_header
        show_invalid_auth_settings
        pause
        return 1
    fi

    bin_path="$(runtime_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        show_header
        printf "%s%s binary is not installed%s\n" "$C_RED" "$APP_NAME" "$C_RESET"
        pause
        return 1
    fi

    if is_running; then
        prompt_restart_running_proxy || return 1
    fi

    if port_in_use; then
        prompt_stop_detected_proxy_for_busy_port || return 1
    fi

    show_header
    show_environment_checks
    printf "\n"
    printf "%sStarting %s in terminal%s\n\n" "$C_GREEN" "$APP_NAME" "$C_RESET"
    printf "Logs will be printed here.\n"
    printf "Stop with Ctrl+C\n"
    printf "Bind: %s:%s\n\n" "$LISTEN_HOST" "$LISTEN_PORT"
    show_telegram_settings
    printf "\n"
    interrupted="0"
    run_binary &
    child_pid="$!"
    mkdir -p "$(dirname "$PID_FILE")" >/dev/null 2>&1 || true
    printf "%s\n" "$child_pid" > "$PID_FILE" 2>/dev/null || true
    trap 'interrupted="1"; kill -INT "$child_pid" 2>/dev/null' INT
    wait "$child_pid"
    code="$?"
    rm -f "$PID_FILE"
    trap - INT
    printf "\n%s%s exited with code %s%s\n" "$C_YELLOW" "$APP_NAME" "$code" "$C_RESET"
    if [ "$interrupted" = "1" ]; then
        printf "Returned to menu after Ctrl+C\n"
    fi
    pause
}

start_proxy_background() {
    if ! auth_settings_valid; then
        show_header
        show_invalid_auth_settings
        pause
        return 1
    fi

    bin_path="$(runtime_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        show_header
        printf "%s%s binary is not installed%s\n" "$C_RED" "$APP_NAME" "$C_RESET"
        pause
        return 1
    fi

    if is_running; then
        prompt_restart_running_proxy || return 1
    fi

    if port_in_use; then
        prompt_stop_detected_proxy_for_busy_port || return 1
    fi

    show_header
    show_environment_checks
    printf "\n"
    printf "%sStarting %s in background%s\n\n" "$C_GREEN" "$APP_NAME" "$C_RESET"
    printf "Logs will not be printed in this session.\n"
    printf "Bind: %s:%s\n\n" "$LISTEN_HOST" "$LISTEN_PORT"

    child_pid="$(run_binary_background)" || return 1
    mkdir -p "$(dirname "$PID_FILE")" >/dev/null 2>&1 || true
    printf "%s\n" "$child_pid" > "$PID_FILE" 2>/dev/null || true
    sleep 1

    if kill -0 "$child_pid" 2>/dev/null; then
        printf "Background process pid:\n  %s\n" "$child_pid"
        pause
        return 0
    fi

    wait "$child_pid" 2>/dev/null
    code="$?"
    rm -f "$PID_FILE"
    printf "%sBackground start failed%s\n\n" "$C_RED" "$C_RESET"
    printf "Process exited with code: %s\n" "$code"
    pause
    return 1
}

stop_proxy() {
    show_header
    if stop_running; then
        printf "%sProxy stopped%s\n" "$C_GREEN" "$C_RESET"
    else
        printf "%s%s is not running%s\n" "$C_YELLOW" "$APP_NAME" "$C_RESET"
    fi
    pause
}

restart_proxy() {
    stop_running >/dev/null 2>&1 || true
    start_proxy
}

restart_running_proxy_for_updated_settings() {
    if autostart_enabled; then
        "$INIT_SCRIPT_PATH" restart >/dev/null 2>&1 && return 0
        "$INIT_SCRIPT_PATH" stop >/dev/null 2>&1 || true
        "$INIT_SCRIPT_PATH" start >/dev/null 2>&1 && return 0
        return 1
    fi

    stop_running >/dev/null 2>&1 || true
    child_pid="$(run_binary_background)" || return 1
    mkdir -p "$(dirname "$PID_FILE")" >/dev/null 2>&1 || true
    printf "%s\n" "$child_pid" > "$PID_FILE" 2>/dev/null || true
    sleep 1

    if kill -0 "$child_pid" 2>/dev/null; then
        return 0
    fi

    wait "$child_pid" 2>/dev/null
    rm -f "$PID_FILE"
    return 1
}

prompt_restart_proxy_for_updated_settings() {
    if ! is_running; then
        return 0
    fi

    printf "\nProxy is currently running.\n"
    printf "Restart now to apply the new settings? [y/N]: "
    IFS= read -r restart_choice

    case "$restart_choice" in
        y|Y|yes|YES|Yes)
            if restart_running_proxy_for_updated_settings; then
                printf "\n%sProxy restarted with the updated settings%s\n" "$C_GREEN" "$C_RESET"
            else
                printf "\n%sProxy restart failed. Restart it manually to apply the new settings.%s\n" "$C_RED" "$C_RESET"
            fi
            ;;
        *)
            printf "\nRestart skipped. New settings will apply on the next start.\n"
            ;;
    esac
}
