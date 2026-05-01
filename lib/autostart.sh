#!/bin/sh
# autostart.sh

autostart_enabled() {
    [ -f "$INIT_SCRIPT_PATH" ] || return 1
    ls "$RC_D_DIR"/*"$(basename "$INIT_SCRIPT_PATH")" >/dev/null 2>&1 || return 1
    bin_path="$(persistent_bin_path 2>/dev/null || true)"
    [ -n "$bin_path" ] || return 1
    [ -x "$bin_path" ] || return 1
    [ -r "$PERSIST_CONFIG_FILE" ]
}

write_init_script() {
    mkdir -p "$(dirname "$INIT_SCRIPT_PATH")" || return 1
    {
        printf '%s\n' "#!/bin/sh $RC_COMMON_PATH"
        printf '%s\n' 'START=95'
        printf '%s\n' 'STOP=10'
        printf '%s\n' 'USE_PROCD=1'
        printf '%s\n' "CONFIG_FILE='$PERSIST_CONFIG_FILE'"
        printf '\n'
        printf '%s\n' 'start_service() {'
        printf '%s\n' '    [ -r "$CONFIG_FILE" ] || return 1'
        printf '%s\n' '    . "$CONFIG_FILE"'
        printf '%s\n' '    [ -x "$BIN" ] || return 1'
        printf '%s\n' '    [ -n "$HOST" ] || HOST="0.0.0.0"'
        printf '%s\n' '    [ -n "$PORT" ] || PORT="1080"'
        printf '%s\n' '    [ -n "$POOL_SIZE" ] || POOL_SIZE="4"'
        printf '%s\n' '    USERNAME="${USERNAME:-}"'
        printf '%s\n' '    PASSWORD="${PASSWORD:-}"'
        printf '%s\n' '    DC_IPS="${DC_IPS:-}"'
        printf '%s\n' '    CF_PROXY="${CF_PROXY:-0}"'
        printf '%s\n' '    CF_PROXY_FIRST="${CF_PROXY_FIRST:-0}"'
        printf '%s\n' '    CF_BALANCE="${CF_BALANCE:-1}"'
        printf '%s\n' '    CF_DOMAIN="${CF_DOMAIN:-}"'
        printf '%s\n' "    CF_BUILTIN_DOMAINS_OBF='${CF_BUILTIN_DOMAINS_OBF}'"
        printf '%s\n' '    PROXY_MODE="${PROXY_MODE:-socks5}"'
        printf '%s\n' '    MT_SECRET="${MT_SECRET:-}"'
        printf '%s\n' '    MT_LINK_IP="${MT_LINK_IP:-}"'
        printf '%s\n' '    MT_UPSTREAM_PROXIES="${MT_UPSTREAM_PROXIES:-}"'
        printf '%s\n' '    cf_builtin_domains() {'
        printf '%s\n' '        if [ -n "${CF_BUILTIN_DOMAINS:-}" ]; then'
        printf '%s\n' '            printf "%s" "$CF_BUILTIN_DOMAINS"'
        printf '%s\n' '            return 0'
        printf '%s\n' '        fi'
        printf '%s\n' "        printf '%b' \"\${CF_BUILTIN_DOMAINS_OBF:-}\""
        printf '%s\n' '    }'
        printf '%s\n' '    normalize_cf_domain_list() {'
        printf '%s\n' '        value="$1"'
        printf '%s\n' '        [ -n "$value" ] || return 1'
        printf '%s\n' "        awk -v input=\"\$value\" '"
        printf '%s\n' '            function trim(s) {'
        printf '%s\n' '                gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)'
        printf '%s\n' '                return s'
        printf '%s\n' '            }'
        printf '%s\n' '            BEGIN {'
        printf '%s\n' '                count = split(input, parts, ",")'
        printf '%s\n' '                out = ""'
        printf '%s\n' '                for (i = 1; i <= count; i++) {'
        printf '%s\n' '                    part = trim(parts[i])'
        printf '%s\n' '                    if (part == "" || seen[part]++) continue'
        printf '%s\n' '                    out = (out == "" ? part : out "," part)'
        printf '%s\n' '                }'
        printf '%s\n' '                if (out == "") exit 1'
        printf '%s\n' '                print out'
        printf '%s\n' '            }'
        printf '%s\n' "        '"
        printf '%s\n' '    }'
        printf '%s\n' '    custom_cf_domains="$(normalize_cf_domain_list "${CF_DOMAIN:-}" 2>/dev/null || true)"'
        printf '%s\n' '    if [ -n "$custom_cf_domains" ]; then'
        printf '%s\n' '        resolved_cf_domains="$custom_cf_domains"'
        printf '%s\n' '        resolved_cf_domain_source="custom"'
        printf '%s\n' '    else'
        printf '%s\n' '        resolved_cf_domains="$(cf_builtin_domains)"'
        printf '%s\n' '        resolved_cf_domain_source="built-in"'
        printf '%s\n' '    fi'
        printf '%s\n' '    if [ "$PROXY_MODE" = "mtproto" ]; then'
        printf '%s\n' '        [ -n "$MT_SECRET" ] || return 1'
        printf '%s\n' '    else'
        printf '%s\n' '        if { [ -n "$USERNAME" ] && [ -z "$PASSWORD" ]; } || { [ -z "$USERNAME" ] && [ -n "$PASSWORD" ]; }; then'
        printf '%s\n' '            return 1'
        printf '%s\n' '        fi'
        printf '%s\n' '    fi'
        printf '%s\n' '    set -- "$BIN" --host "$HOST" --port "$PORT" --pool-size "$POOL_SIZE"'
        printf '%s\n' '    if [ "$PROXY_MODE" = "mtproto" ]; then'
        printf '%s\n' '        set -- "$@" --mode mtproto --secret "$MT_SECRET"'
        printf '%s\n' '        if [ -n "$MT_LINK_IP" ]; then'
        printf '%s\n' '            set -- "$@" --link-ip "$MT_LINK_IP"'
        printf '%s\n' '        fi'
        printf '%s\n' '    else'
        printf '%s\n' '        if [ -n "$USERNAME" ] && [ -n "$PASSWORD" ]; then'
        printf '%s\n' '            set -- "$@" --username "$USERNAME" --password "$PASSWORD"'
        printf '%s\n' '        fi'
        printf '%s\n' '    fi'
        printf '%s\n' '    if [ "${VERBOSE:-0}" = "1" ]; then'
        printf '%s\n' '        set -- "$@" --verbose'
        printf '%s\n' '    fi'
        printf '%s\n' '    if [ -n "$DC_IPS" ]; then'
        printf '%s\n' '        old_ifs="$IFS"'
        printf '%s\n' "        IFS=','"
        printf '%s\n' '        for entry in $DC_IPS; do'
        printf '%s\n' '            entry="$(printf "%s" "$entry" | sed '"'"'s/^[[:space:]]*//;s/[[:space:]]*$//'"'"')"'
        printf '%s\n' '            [ -n "$entry" ] || continue'
        printf '%s\n' '            set -- "$@" --dc-ip "$entry"'
        printf '%s\n' '        done'
        printf '%s\n' '        IFS="$old_ifs"'
        printf '%s\n' '    fi'
        printf '%s\n' '    if [ "$CF_PROXY" = "1" ] && [ -n "$resolved_cf_domains" ]; then'
        printf '%s\n' '        set -- "$@" --cf-proxy --cf-domain "$resolved_cf_domains"'
        printf '%s\n' '        if [ "$CF_PROXY_FIRST" = "1" ]; then'
        printf '%s\n' '            set -- "$@" --cf-proxy-first'
        printf '%s\n' '        fi'
        printf '%s\n' '        if [ "$CF_BALANCE" = "1" ] && "$BIN" --help 2>&1 | grep -F -- "  -cf-balance" >/dev/null 2>&1; then'
        printf '%s\n' '            set -- "$@" --cf-balance'
        printf '%s\n' '        fi'
        printf '%s\n' '    fi'
        printf '%s\n' '    if [ "$PROXY_MODE" = "mtproto" ] && [ -n "$MT_UPSTREAM_PROXIES" ]; then'
        printf '%s\n' '        old_ifs="$IFS"'
        printf '%s\n' "        IFS=','"
        printf '%s\n' '        for up_entry in $MT_UPSTREAM_PROXIES; do'
        printf '%s\n' '            up_entry="$(printf "%s" "$up_entry" | sed '"'"'s/^[[:space:]]*//;s/[[:space:]]*$//'"'"')"'
        printf '%s\n' '            [ -n "$up_entry" ] || continue'
        printf '%s\n' '            set -- "$@" --mtproto-proxy "$up_entry"'
        printf '%s\n' '        done'
        printf '%s\n' '        IFS="$old_ifs"'
        printf '%s\n' '    fi'
        printf '%s\n' '    procd_open_instance'
        printf '%s\n' '    procd_set_param command "$@"'
        printf '%s\n' '    procd_set_param env TG_WS_PROXY_CF_DOMAIN_SOURCE="$resolved_cf_domain_source"'
        printf '%s\n' '    procd_set_param respawn'
        printf '%s\n' '    procd_set_param stdout 1'
        printf '%s\n' '    procd_set_param stderr 1'
        printf '%s\n' '    procd_close_instance'
        printf '%s\n' '}'
    } > "$INIT_SCRIPT_PATH" || return 1

    chmod +x "$INIT_SCRIPT_PATH" || return 1
}

enable_autostart() {
    show_header
    started_now="0"
    start_note=""

    if ! auth_settings_valid; then
        show_invalid_auth_settings
        pause
        return 1
    fi

    if ! is_openwrt; then
        printf "%sAutostart is only supported on OpenWrt%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    bin_path="$(persistent_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        need_kb="$(required_tmp_download_kb)"
        if ! check_tmp_space "$need_kb"; then
            free_kb="$(tmp_available_kb)"
            printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
            printf "Required: %s KB\n" "$need_kb"
            printf "Available: %s KB\n" "${free_kb:-unknown}"
            pause
            return 1
        fi

        if ! ensure_source_binary_current; then
            pause
            return 1
        fi

        launcher_path="$(install_persistent_binary 2>/dev/null || true)"
        if [ -z "$launcher_path" ]; then
            show_persistent_install_failure
            pause
            return 1
        fi
        bin_path="$(persistent_bin_path 2>/dev/null || true)"
        printf "%sPersistent copy installed automatically%s\n\n" "$C_GREEN" "$C_RESET"
        printf "Persistent binary:\n  %s\n" "$bin_path"
        printf "Launcher:\n  %s\n\n" "$launcher_path"
    fi

    write_autostart_config "$bin_path" || return 1
    write_init_script || return 1

    if ! "$INIT_SCRIPT_PATH" enable >/dev/null 2>&1; then
        printf "%sFailed to enable init.d service%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    if ! is_running; then
        if "$INIT_SCRIPT_PATH" start >/dev/null 2>&1; then
            started_now="1"
        else
            start_note="Autostart was enabled, but the service did not start immediately"
        fi
    fi

    printf "%sAutostart enabled%s\n\n" "$C_GREEN" "$C_RESET"
    printf "Service:\n  %s\n" "$INIT_SCRIPT_PATH"
    printf "Binary:\n  %s\n" "$bin_path"
    if [ "$started_now" = "1" ]; then
        printf "\nCurrent state:\n  service started now\n"
    elif [ -n "$start_note" ]; then
        printf "\n%s%s%s\n" "$C_YELLOW" "$start_note" "$C_RESET"
    fi
    pause
}

disable_autostart() {
    show_header

    preserved_update_channel="$(selected_update_channel 2>/dev/null || printf release)"
    preserved_update_ref="$(selected_update_ref 2>/dev/null || printf latest)"
    persist_dir="$(persistent_install_dir 2>/dev/null || true)"
    if [ ! -f "$INIT_SCRIPT_PATH" ] && [ -z "$persist_dir" ]; then
        printf "%sAutostart is not configured%s\n" "$C_YELLOW" "$C_RESET"
        pause
        return 0
    fi

    if [ -f "$INIT_SCRIPT_PATH" ]; then
        "$INIT_SCRIPT_PATH" disable >/dev/null 2>&1 || true
        "$INIT_SCRIPT_PATH" stop >/dev/null 2>&1 || true
    fi
    if [ -n "$persist_dir" ]; then
        rm -rf "$persist_dir"
    fi
    rm -rf "$PERSIST_STATE_DIR"
    case "$preserved_update_channel:$preserved_update_ref" in
        preview:*)
            write_update_source_state "preview" "$preserved_update_ref" >/dev/null 2>&1 || true
            ;;
        release:latest|release:)
            ;;
        release:*)
            write_update_source_state "release" "$preserved_update_ref" >/dev/null 2>&1 || true
            ;;
    esac
    rm -f "$INIT_SCRIPT_PATH"

    if [ -x "$BIN_PATH" ]; then
        install_launcher "$0" >/dev/null 2>&1 || true
    else
        rm -f "$LAUNCHER_PATH" "/tmp/$LAUNCHER_NAME"
    fi

    printf "%sAutostart disabled and persistent copy removed%s\n" "$C_GREEN" "$C_RESET"
    pause
}
