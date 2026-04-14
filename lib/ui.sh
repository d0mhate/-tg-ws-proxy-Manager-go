# ui.sh

show_dc_ip_mapping_settings() {
    printf "%sTelegram DC mapping%s\n" "$C_BOLD" "$C_RESET"
    if [ -n "$DC_IPS" ]; then
        printf "  custom : %s\n" "$DC_IPS"
    else
        printf "  custom : <default>\n"
    fi
}

telegram_host() {
    case "$LISTEN_HOST" in
        0.0.0.0|"")
            ip="$(lan_ip)"
            if [ -n "$ip" ]; then
                printf "%s" "$ip"
            else
                printf "127.0.0.1"
            fi
            ;;
        127.0.0.1|localhost)
            printf "127.0.0.1"
            ;;
        *)
            printf "%s" "$LISTEN_HOST"
            ;;
    esac
}

show_header() {
    if [ "$COMMAND_MODE" = "0" ] && [ -t 1 ]; then
        clear
    fi
    version="$(installed_version 2>/dev/null || true)"
    if [ -z "$version" ]; then
        version="$(persistent_installed_version 2>/dev/null || true)"
    fi
    printf "%s+----------------------------------+%s\n" "$C_BLUE" "$C_RESET"
    if [ -n "$version" ]; then
        ver_len=${#version}
        pad=$((11 - ver_len))
        [ "$pad" -lt 1 ] && pad=1
        printf "%s|%s %s%s Go manager%s" "$C_BLUE" "$C_RESET" "$C_BOLD" "$APP_NAME" "$C_RESET"
        printf "%${pad}s %s%s%s|%s\n" "" "$C_DIM" "$version" "$C_RESET" "$C_BLUE"
    else
        printf "%s|%s %s%s Go manager%s            %s|%s\n" "$C_BLUE" "$C_RESET" "$C_BOLD" "$APP_NAME" "$C_RESET" "$C_BLUE" "$C_RESET"
    fi
    printf "%s+----------------------------------+%s\n\n" "$C_BLUE" "$C_RESET"
}

show_telegram_settings() {
    printf "%sTelegram SOCKS5%s\n" "$C_BOLD" "$C_RESET"
    printf "  host     : %s\n" "$(telegram_host)"
    printf "  port     : %s\n" "$LISTEN_PORT"
    if [ -n "$SOCKS_USERNAME" ]; then
        printf "  username : %s\n" "$SOCKS_USERNAME"
    else
        printf "  username : <empty>\n"
    fi
    printf "  password : %s\n" "$(password_display)"
    if [ -n "$DC_IPS" ]; then
        printf "  dc map   : %s\n" "$DC_IPS"
    else
        printf "  dc map   : <default>\n"
    fi
    if [ "$CF_PROXY" = "1" ]; then
        printf "  cf proxy : on\n"
    else
        printf "  cf proxy : off\n"
    fi
    if [ "$CF_PROXY_FIRST" = "1" ]; then
        printf "  cf order : first\n"
    else
        printf "  cf order : fallback\n"
    fi
    if [ -z "$CF_DOMAIN" ]; then
        printf "  cf domain: not set\n"
    else
        _cf_commas=$(printf '%s' "$CF_DOMAIN" | tr -cd ',' | wc -c | tr -d ' ')
        if [ "$_cf_commas" -eq 0 ]; then
            printf "  cf domain: %s\n" "$CF_DOMAIN"
        else
            _cf_count=$((_cf_commas + 1))
            printf "  cf domain: %d domains\n" "$_cf_count"
        fi
    fi
}

show_current_version() {
    version="$(installed_version 2>/dev/null || true)"
    if [ -z "$version" ]; then
        version="$(persistent_installed_version 2>/dev/null || true)"
    fi
    [ -n "$version" ] || version="-"
    printf "%sBinary version%s\n" "$C_BOLD" "$C_RESET"
    printf "  %s\n" "$version"
}

show_telegram_settings_compact() {
    host="$(telegram_host)"
    if [ -n "$SOCKS_USERNAME" ]; then
        if [ -n "$SOCKS_PASSWORD" ]; then
            auth_part="user:$SOCKS_USERNAME/<set>"
        else
            auth_part="user:$SOCKS_USERNAME"
        fi
    else
        auth_part="no auth"
    fi
    if [ -n "$DC_IPS" ]; then
        dc_part="dc:custom"
    else
        dc_part="dc:default"
    fi
    printf "  SOCKS5  %s:%s  %s  %s\n" "$host" "$LISTEN_PORT" "$auth_part" "$dc_part"

    if [ "$CF_PROXY" = "1" ]; then
        cf_on="${C_GREEN}on${C_RESET}"
    else
        cf_on="${C_DIM}off${C_RESET}"
    fi
    if [ "$CF_PROXY_FIRST" = "1" ]; then
        cf_order="first"
    else
        cf_order="fallback"
    fi
    if [ -z "$CF_DOMAIN" ]; then
        cf_domain_part="domain:none"
    else
        _cf_commas=$(printf '%s' "$CF_DOMAIN" | tr -cd ',' | wc -c | tr -d ' ')
        if [ "$_cf_commas" -eq 0 ]; then
            cf_domain_part="domain:$CF_DOMAIN"
        else
            _cf_count=$((_cf_commas + 1))
            cf_domain_part="domain:${_cf_count} set"
        fi
    fi
    printf "  CF      %s / %s / %s\n" "$cf_on" "$cf_order" "$cf_domain_part"
}

show_update_source_settings() {
    channel="$(selected_update_channel 2>/dev/null || printf release)"
    ref="$(selected_update_ref 2>/dev/null || printf latest)"
    printf "%sUpdate source%s\n" "$C_BOLD" "$C_RESET"
    printf "  mode     : %s\n" "$channel"
    printf "  ref      : %s\n" "$ref"
}

main_menu_track_label() {
    channel="$(selected_update_channel 2>/dev/null || printf release)"
    ref="$(selected_update_ref 2>/dev/null || printf latest)"

    if [ "$channel" = "preview" ]; then
        printf "preview/%s" "$ref"
        return 0
    fi

    printf "release/%s" "$ref"
}

show_quick_commands() {
    printf "%sQuick commands%s\n" "$C_BOLD" "$C_RESET"
    printf "  sh %s install\n" "$0"
    printf "  sh %s update\n" "$0"
    printf "  sh %s persist\n" "$0"
    printf "  sh %s enable-autostart\n" "$0"
    printf "  sh %s disable-autostart\n" "$0"
    printf "  sh %s start\n" "$0"
    printf "  sh %s start-background\n" "$0"
    printf "  sh %s stop\n" "$0"
    printf "  sh %s restart\n" "$0"
    printf "  sh %s status\n" "$0"
    printf "  sh %s quick\n" "$0"
    printf "  sh %s telegram\n" "$0"
    printf "  sh %s remove\n" "$0"
    printf "  sh %s help\n" "$0"
    if launcher="$(current_launcher_path 2>/dev/null)"; then
        printf "  %s\n" "$launcher"
    fi
}

show_status() {
    if [ -x "$BIN_PATH" ]; then
        install_state="${C_GREEN}installed${C_RESET}"
    else
        install_state="${C_RED}not installed${C_RESET}"
    fi

    if has_persistent_install; then
        persistent_state="${C_GREEN}installed${C_RESET}"
        persistent_bin="$(persistent_bin_path 2>/dev/null || true)"
        persistent_dir="$(persistent_install_dir 2>/dev/null || true)"
        persistent_version="$(persistent_installed_version 2>/dev/null || true)"
    else
        persistent_state="${C_RED}not installed${C_RESET}"
        persistent_bin="-"
        persistent_dir="-"
        persistent_version="-"
    fi

    if is_running; then
        pid="$(current_pids | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
        run_state="${C_GREEN}running${C_RESET}"
    else
        pid="-"
        run_state="${C_RED}stopped${C_RESET}"
    fi

    if [ "$VERBOSE" = "1" ]; then
        verbose_state="${C_GREEN}on${C_RESET}"
    else
        verbose_state="${C_DIM}off${C_RESET}"
    fi

    version="$(installed_version 2>/dev/null || true)"
    if [ -z "$version" ]; then
        version="$persistent_version"
    fi
    [ -n "$version" ] || version="-"

    if autostart_enabled; then
        autostart_state="${C_GREEN}enabled${C_RESET}"
    elif [ -f "$INIT_SCRIPT_PATH" ]; then
        autostart_state="${C_YELLOW}installed but disabled${C_RESET}"
    else
        autostart_state="${C_RED}not configured${C_RESET}"
    fi

    printf "%sStatus%s\n" "$C_BOLD" "$C_RESET"
    printf "  tmp bin   : %s\n" "$install_state"
    printf "  persist   : %s\n" "$persistent_state"
    printf "  process   : %s\n" "$run_state"
    printf "  pid       : %s\n" "$pid"
    printf "  bin ver   : %s\n" "$version"
    printf "  source    : %s\n" "$SOURCE_BIN"
    printf "  asset     : %s\n" "$(resolved_binary_name)"
    printf "  src mode  : %s\n" "$(selected_update_channel 2>/dev/null || printf release)"
    printf "  ref       : %s\n" "$(selected_update_ref 2>/dev/null || printf latest)"
    printf "  release   : %s\n" "$(resolved_release_url)"
    printf "  tmp path  : %s\n" "$BIN_PATH"
    printf "  persist dir: %s\n" "$persistent_dir"
    printf "  persist bin: %s\n" "$persistent_bin"
    printf "  autostart : %s\n" "$autostart_state"
    if launcher="$(current_launcher_path 2>/dev/null)"; then
        printf "  launcher  : %s\n" "$launcher"
    else
        printf "  launcher  : %s\n" "-"
    fi
    printf "  listen    : %s:%s\n" "$LISTEN_HOST" "$LISTEN_PORT"
    printf "  mode      : terminal logs only\n"
    printf "  verbose   : %s\n" "$verbose_state"
    if is_openwrt; then
        printf "  system    : OpenWrt\n"
    else
        printf "  system    : not detected as OpenWrt\n"
    fi
    arch="$(openwrt_arch)"
    printf "  arch      : %s\n" "${arch:--}"
    free_kb="$(tmp_available_kb)"
    printf "  tmp free  : %s KB\n" "${free_kb:--}"
}

show_telegram_only() {
    show_header
    show_telegram_settings
    printf "\nLogs are printed directly in the terminal while %s is running.\n" "$APP_NAME"
    pause
}

show_quick_only() {
    show_header
    show_quick_commands
    pause
}

menu_proxy_action_label() {
    if [ "$1" = "1" ]; then
        printf "Stop proxy"
    else
        printf "Start proxy"
    fi
}

menu_autostart_action_label() {
    if [ "$1" = "1" ]; then
        printf "Disable autostart"
    else
        printf "Enable autostart"
    fi
}

show_menu_summary() {
    if [ "$1" = "1" ]; then
        proxy_state="${C_GREEN}running${C_RESET}"
    else
        proxy_state="${C_RED}stopped${C_RESET}"
    fi

    if [ "$2" = "1" ]; then
        autostart_state="${C_GREEN}enabled${C_RESET}"
    else
        autostart_state="${C_RED}disabled${C_RESET}"
    fi

    if [ "$VERBOSE" = "1" ]; then
        verbose_state="${C_GREEN}on${C_RESET}"
    else
        verbose_state="${C_DIM}off${C_RESET}"
    fi

    printf "  proxy: %s | autostart: %s | verbose: %s | track: %s\n" \
        "$proxy_state" "$autostart_state" "$verbose_state" "$(main_menu_track_label)"
}

show_help() {
    show_header
    printf "%sUsage%s\n" "$C_BOLD" "$C_RESET"
    printf "  sh %s                start menu mode\n" "$0"
    printf "  sh %s install        install or update binary\n" "$0"
    printf "  sh %s update         update from configured source\n" "$0"
    printf "  sh %s enable-autostart   enable OpenWrt autostart\n" "$0"
    printf "  sh %s disable-autostart  disable OpenWrt autostart\n" "$0"
    printf "  sh %s start          run proxy in terminal\n" "$0"
    printf "  sh %s start-background start proxy in background\n" "$0"
    printf "  sh %s stop           stop running proxy\n" "$0"
    printf "  sh %s restart        restart proxy in terminal\n" "$0"
    printf "  sh %s status         show status\n" "$0"
    printf "  sh %s quick          show quick commands\n" "$0"
    printf "  sh %s telegram       show Telegram SOCKS5 settings\n" "$0"
    printf "  sh %s remove         remove installed binary\n" "$0"
    printf "  sh %s help           show this help\n" "$0"
    pause
}
