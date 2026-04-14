# menu.sh

can_use_arrow_update_source_picker() {
    if [ -n "$FORCE_ARROW_UPDATE_SOURCE_PICKER" ]; then
        return 0
    fi

    [ -t 0 ] || return 1
    [ -t 2 ] || return 1
    [ "${TERM:-}" != "dumb" ] || return 1
    command -v stty >/dev/null 2>&1 || return 1
}

can_use_numbered_update_source_picker() {
    if [ -n "$FORCE_NUMBERED_UPDATE_SOURCE_PICKER" ]; then
        return 0
    fi

    [ -t 0 ] || return 1
    [ -t 2 ] || return 1
    [ "${TERM:-}" != "dumb" ] || return 1
}

read_picker_hex_byte() {
    dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n'
}

draw_update_source_picker() {
    current="$1"
    if [ "$current" = "preview" ]; then
        release_prefix="  "
        preview_prefix="> "
    else
        release_prefix="> "
        preview_prefix="  "
    fi

    printf "Mode (use arrows, Enter to confirm):\n" >&2
    printf "%srelease\n" "$release_prefix" >&2
    printf "%spreview\n" "$preview_prefix" >&2
}

choose_update_source_mode_numbered() {
    current="${1:-release}"

    printf "Mode:\n" >&2
    printf "  1) release\n" >&2
    printf "  2) preview\n" >&2
    printf "Select mode [1-2] (Enter for %s): " "$current" >&2
    IFS= read -r selected_mode

    case "$selected_mode" in
        "")
            selected_mode="$current"
            ;;
        1|release)
            selected_mode="release"
            ;;
        2|preview)
            selected_mode="preview"
            ;;
    esac

    printf "%s" "$selected_mode"
}

prompt_manual_release_tag() {
    current_tag="$(normalize_version "${1:-}" 2>/dev/null || true)"

    if [ -n "$current_tag" ]; then
        printf "Release tag (Enter to keep %s): " "$current_tag" >&2
    else
        printf "Release tag (for example: v1.1.28): " >&2
    fi

    IFS= read -r typed_tag
    if [ -z "$typed_tag" ]; then
        if [ -n "$current_tag" ]; then
            printf "%s" "$current_tag"
            return 0
        fi
        printf "\n%sRelease tag cannot be empty here%s\n" "$C_RED" "$C_RESET" >&2
        return 1
    fi

    normalized_tag="$(normalize_version "$typed_tag" 2>/dev/null || true)"
    if [ -z "$normalized_tag" ]; then
        printf "\n%sRelease tag must look like v1.1.28%s\n" "$C_RED" "$C_RESET" >&2
        return 1
    fi
    if ! release_tag_meets_minimum "$normalized_tag"; then
        printf "\n%sRelease tag must be %s or newer%s\n" "$C_RED" "$MIN_PINNED_RELEASE_TAG" "$C_RESET" >&2
        return 1
    fi

    printf "%s" "$normalized_tag"
}

choose_release_ref_numbered() {
    current_ref="${1:-latest}"
    current_tag="$(normalize_version "$current_ref" 2>/dev/null || true)"
    tags="$(recent_release_tags 8 2>/dev/null || true)"

    if [ -n "$current_tag" ] && ! printf "%s\n" "$tags" | grep -Fx "$current_tag" >/dev/null 2>&1; then
        if [ -n "$tags" ]; then
            tags="$(printf '%s\n%s' "$current_tag" "$tags")"
        else
            tags="$current_tag"
        fi
    fi

    printf "Release ref:\n" >&2
    printf "  1) latest\n" >&2

    option_count=1
    old_ifs="$IFS"
    IFS='
'
    for tag in $tags; do
        [ -n "$tag" ] || continue
        option_count=$((option_count + 1))
        printf "  %s) %s\n" "$option_count" "$tag" >&2
    done
    IFS="$old_ifs"

    manual_option=$((option_count + 1))
    printf "  %s) enter tag manually\n" "$manual_option" >&2
    printf "Select release ref [1-%s] (Enter for %s): " "$manual_option" "$current_ref" >&2
    IFS= read -r selected_ref

    case "$selected_ref" in
        "")
            case "$current_ref" in
                latest|"")
                    printf ""
                    return 0
                    ;;
                *)
                    printf "%s" "$current_ref"
                    return 0
                    ;;
            esac
            ;;
        1|latest)
            printf ""
            return 0
            ;;
        "$manual_option"|m|M|manual)
            prompt_manual_release_tag "$current_tag"
            return $?
            ;;
    esac

    chosen_tag="$(printf "latest\n%s\n" "$tags" | sed -n "${selected_ref}p" 2>/dev/null || true)"
    case "$chosen_tag" in
        "")
            printf "\n%sUnknown release ref selection%s\n" "$C_RED" "$C_RESET" >&2
            return 1
            ;;
        latest)
            printf ""
            return 0
            ;;
        *)
            printf "%s" "$chosen_tag"
            return 0
            ;;
    esac
}

choose_release_ref() {
    current_ref="${1:-latest}"

    if can_use_numbered_update_source_picker; then
        choose_release_ref_numbered "$current_ref"
        return $?
    fi

    printf "Release ref (empty/latest for latest, or tag like v1.1.28): " >&2
    IFS= read -r new_ref
    case "$new_ref" in
        ""|latest)
            printf ""
            return 0
            ;;
        *)
            normalized_tag="$(normalize_version "$new_ref" 2>/dev/null || true)"
            if [ -z "$normalized_tag" ]; then
                printf "\n%sRelease tag must look like v1.1.28%s\n" "$C_RED" "$C_RESET" >&2
                return 1
            fi
            printf "%s" "$normalized_tag"
            return 0
            ;;
    esac
}

choose_update_source_mode() {
    current="${1:-release}"

    if can_use_arrow_update_source_picker; then
        :
    elif can_use_numbered_update_source_picker; then
        choose_update_source_mode_numbered "$current"
        return 0
    else
        printf "Mode [release/preview] (Enter for %s): " "$current" >&2
        IFS= read -r selected_mode
        if [ -z "$selected_mode" ]; then
            selected_mode="$current"
        fi
        printf "%s" "$selected_mode"
        return 0
    fi

    restore_stty=""
    if [ -z "$FORCE_ARROW_UPDATE_SOURCE_PICKER" ]; then
        restore_stty="$(stty -g 2>/dev/null || true)"
        if [ -z "$restore_stty" ]; then
            printf "Mode [release/preview] (Enter for %s): " "$current" >&2
            IFS= read -r selected_mode
            if [ -z "$selected_mode" ]; then
                selected_mode="$current"
            fi
            printf "%s" "$selected_mode"
            return 0
        fi
        stty -echo -icanon min 1 time 0 2>/dev/null || true
    fi

    redraw="0"
    while true; do
        if [ "$redraw" = "1" ]; then
            printf '\033[3A\033[J' >&2
        fi
        draw_update_source_picker "$current"
        redraw="1"

        first_hex="$(read_picker_hex_byte)"
        case "$first_hex" in
            0a|0d)
                break
                ;;
            1b)
                second_hex="$(read_picker_hex_byte)"
                third_hex="$(read_picker_hex_byte)"
                case "$second_hex$third_hex" in
                    5b41|5b44)
                        current="release"
                        ;;
                    5b42|5b43)
                        current="preview"
                        ;;
                esac
                ;;
        esac
    done

    if [ -n "$restore_stty" ]; then
        stty "$restore_stty" 2>/dev/null || true
    fi

    printf "\n" >&2
    printf "%s" "$current"
}

configure_socks_auth() {
    show_header
    show_telegram_settings
    printf "\nLeave username empty to disable SOCKS5 auth.\n"
    printf "Username: "
    IFS= read -r new_username

    if [ -z "$new_username" ]; then
        SOCKS_USERNAME=""
        SOCKS_PASSWORD=""
        write_settings_config || return 1
        printf "\n%sSOCKS5 auth disabled%s\n" "$C_GREEN" "$C_RESET"
        prompt_restart_proxy_for_updated_settings
        pause
        return 0
    fi

    printf "Password: "
    IFS= read -r new_password
    if [ -z "$new_password" ]; then
        printf "\n%sPassword cannot be empty when username is set%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    SOCKS_USERNAME="$new_username"
    SOCKS_PASSWORD="$new_password"
    write_settings_config || return 1

    printf "\n%sSOCKS5 auth saved%s\n" "$C_GREEN" "$C_RESET"
    prompt_restart_proxy_for_updated_settings
    pause
}

configure_dc_ip_mapping() {
    show_header
    show_dc_ip_mapping_settings
    printf "\nEnter DC:IP mappings separated by commas.\n"
    printf "Example:\n"
    printf "  2:149.154.167.220, 4:149.154.167.220\n"
    printf "\nUse 'default' to clear custom mapping.\n"
    printf "DC mapping (empty to keep current): "
    IFS= read -r new_dc_ips

    if [ -z "$new_dc_ips" ]; then
        printf "\nNo changes made.\n"
        pause
        return 0
    fi

    case "$new_dc_ips" in
        default|DEFAULT|Default)
            DC_IPS=""
            write_settings_config || return 1
            printf "\n%sTelegram DC mapping reset to defaults%s\n" "$C_GREEN" "$C_RESET"
            prompt_restart_proxy_for_updated_settings
            pause
            return 0
            ;;
    esac

    normalized_dc_ips="$(normalize_dc_ip_list "$new_dc_ips" 2>/dev/null || true)"
    if [ -z "$normalized_dc_ips" ]; then
        printf "\n%sInvalid DC mapping. Use format DC:IP, DC:IP%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    DC_IPS="$normalized_dc_ips"
    write_settings_config || return 1

    printf "\n%sTelegram DC mapping saved%s\n" "$C_GREEN" "$C_RESET"
    prompt_restart_proxy_for_updated_settings
    pause
}

configure_update_source() {
    show_header
    show_update_source_settings
    printf "\nChoose update source.\n"
    new_channel="$(choose_update_source_mode "$(selected_update_channel 2>/dev/null || printf release)")"

    case "$new_channel" in
        release)
            new_ref="$(choose_release_ref "$(selected_update_ref 2>/dev/null || printf latest)")" || {
                pause
                return 1
            }

            UPDATE_CHANNEL="release"
            PREVIEW_BRANCH=""
            RELEASE_TAG="$new_ref"
            write_update_source_state "release" "$new_ref" || return 1
            if [ -n "$new_ref" ]; then
                printf "\n%sUpdate source saved: release %s%s\n" "$C_GREEN" "$new_ref" "$C_RESET"
            else
                printf "\n%sUpdate source saved: latest release%s\n" "$C_GREEN" "$C_RESET"
            fi
            ;;
        preview)
            current_preview_branch="$(selected_preview_branch_value 2>/dev/null || true)"
            if [ -n "$current_preview_branch" ]; then
                printf "Preview branch name (Enter to keep %s): " "$current_preview_branch"
            else
                printf "Preview branch name (for example: preview-channel): "
            fi
            IFS= read -r new_ref
            if [ -z "$new_ref" ]; then
                if [ -n "$current_preview_branch" ]; then
                    new_ref="$current_preview_branch"
                else
                    printf "\n%sPreview branch cannot be empty%s\n" "$C_RED" "$C_RESET"
                    pause
                    return 1
                fi
            fi

            UPDATE_CHANNEL="preview"
            PREVIEW_BRANCH="$new_ref"
            RELEASE_TAG=""
            write_update_source_state "preview" "$new_ref" || return 1
            printf "\n%sUpdate source saved: preview %s%s\n" "$C_GREEN" "$new_ref" "$C_RESET"
            ;;
        *)
            printf "\n%sUpdate mode must be release or preview%s\n" "$C_RED" "$C_RESET"
            pause
            return 1
            ;;
    esac

    pause
}

toggle_verbose() {
    if [ "$VERBOSE" = "1" ]; then
        VERBOSE="0"
    else
        VERBOSE="1"
    fi
    sync_autostart_config_if_enabled >/dev/null 2>&1 || true
}

toggle_cf_proxy() {
    if [ "$CF_PROXY" = "1" ]; then
        CF_PROXY="0"
    else
        CF_PROXY="1"
    fi
    write_settings_config >/dev/null 2>&1 || true
    sync_autostart_config_if_enabled >/dev/null 2>&1 || true
}

toggle_cf_proxy_first() {
    if [ "$CF_PROXY_FIRST" = "1" ]; then
        CF_PROXY_FIRST="0"
    else
        CF_PROXY_FIRST="1"
    fi
    write_settings_config >/dev/null 2>&1 || true
    sync_autostart_config_if_enabled >/dev/null 2>&1 || true
}

configure_cf_domain() {
    show_header
    printf "%sCloudflare proxy domain%s\n" "$C_BOLD" "$C_RESET"
    if [ -z "$CF_DOMAIN" ]; then
        printf "  current: not set\n"
    else
        _cf_commas=$(printf '%s' "$CF_DOMAIN" | tr -cd ',' | wc -c | tr -d ' ')
        if [ "$_cf_commas" -eq 0 ]; then
            printf "  current: %s\n" "$CF_DOMAIN"
        else
            printf "  current: %s\n" "$CF_DOMAIN"
        fi
    fi
    printf "\nEnter your Cloudflare domain(s), comma-separated (e.g. domain1.com,domain2.com).\n"
    printf "DNS records kws1..kws5 and kws203 must point to Telegram DC IPs.\n"
    printf "Use 'clear' to remove the domain.\n"
    printf "CF domain(s) (empty to keep current): "
    IFS= read -r new_cf_domain

    if [ -z "$new_cf_domain" ]; then
        printf "\nNo changes made.\n"
        pause
        return 0
    fi

    case "$new_cf_domain" in
        clear|CLEAR|Clear)
            CF_DOMAIN=""
            write_settings_config || return 1
            printf "\n%sCloudflare domain cleared%s\n" "$C_GREEN" "$C_RESET"
            prompt_restart_proxy_for_updated_settings
            pause
            return 0
            ;;
    esac

    CF_DOMAIN="$new_cf_domain"
    write_settings_config || return 1
    printf "\n%sCloudflare domain saved%s\n" "$C_GREEN" "$C_RESET"
    prompt_restart_proxy_for_updated_settings
    pause
}

check_cf_endpoint() {
    host="$1"

    if command -v openssl >/dev/null 2>&1; then
        if command -v timeout >/dev/null 2>&1; then
            output="$(
                printf 'GET /apiws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: binary\r\n\r\n' "$host" |
                    timeout 8 openssl s_client -quiet -servername "$host" -connect "$host:443" 2>&1 || true
            )"
        elif command -v perl >/dev/null 2>&1; then
            output="$(
                printf 'GET /apiws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: binary\r\n\r\n' "$host" |
                    perl -e 'alarm 8; exec @ARGV' openssl s_client -quiet -servername "$host" -connect "$host:443" 2>&1 || true
            )"
        else
            output="$(
                printf 'GET /apiws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: binary\r\n\r\n' "$host" |
                    openssl s_client -quiet -servername "$host" -connect "$host:443" 2>&1 || true
            )"
        fi
    elif command -v curl >/dev/null 2>&1; then
        output="$(curl -s --max-time 8 --http1.1 \
            -H "Upgrade: websocket" \
            -H "Connection: Upgrade" \
            -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
            -H "Sec-WebSocket-Version: 13" \
            -H "Sec-WebSocket-Protocol: binary" \
            -D - -o /dev/null \
            "https://$host/apiws" 2>/dev/null || true)"
    else
        printf "  %-24s failed            openssl or curl not found\n" "$host"
        return 1
    fi

    status_line="$(printf "%s\n" "$output" | sed -n '/^HTTP\//{p;q;}')"
    if [ -n "$status_line" ]; then
        case "$status_line" in
            *"101 Switching Protocols"*)
                printf "  %-24s tcp ok | tls ok | ws upgrade ok\n" "$host"
                return 0
                ;;
            *)
                printf "  %-24s tcp ok | tls ok | ws upgrade failed\n" "$host"
                return 1
                ;;
        esac
    fi

    if printf "%s" "$output" | grep -E "CONNECTED|Verification: OK|SSL handshake has read|depth=" >/dev/null 2>&1; then
        printf "  %-24s failed            no HTTP response after tcp/tls connect\n" "$host"
        return 1
    fi

    printf "  %-24s failed            connection error\n" "$host"
    return 1
}

check_cf_domain() {
    show_header
    printf "%sCheck Cloudflare domain%s\n" "$C_BOLD" "$C_RESET"
    if [ -z "$CF_DOMAIN" ]; then
        printf "  current: not set\n"
    else
        printf "  current: %s\n" "$CF_DOMAIN"
    fi
    printf "\nEnter domain to check or press Enter to use current.\n"
    printf "Domain: "
    IFS= read -r check_domain

    if [ -z "$check_domain" ]; then
        check_domain="$CF_DOMAIN"
    fi
    if [ -z "$check_domain" ]; then
        printf "\n%sNo Cloudflare domain set%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    printf "\nChecking %s\n\n" "$check_domain"
    printf "Requests:\n"
    for prefix in kws1 kws2 kws3 kws4 kws5 kws203; do
        printf "  WS GET https://%s.%s/apiws\n" "$prefix" "$check_domain"
    done
    printf "\nResults:\n"
    ok_count=0
    ok_hosts=""
    for prefix in kws1 kws2 kws3 kws4 kws5 kws203; do
        host="$prefix.$check_domain"
        printf "  %-24s checking...\n" "$host"
        if check_cf_endpoint "$host"; then
            ok_count=$((ok_count + 1))
            if [ -z "$ok_hosts" ]; then
                ok_hosts="$host"
            else
                ok_hosts="$ok_hosts\\n$host"
            fi
        fi
    done

    printf "\n"
    if [ "$ok_count" -eq 6 ]; then
        printf "%sCloudflare proxy: all tested hosts support websocket upgrade%s\n" "$C_GREEN" "$C_RESET"
    elif [ "$ok_count" -eq 0 ]; then
        printf "%sCloudflare proxy: none of the tested hosts support websocket upgrade%s\n" "$C_RED" "$C_RESET"
    else
        printf "%sCloudflare proxy: partially works (%s/%s hosts passed websocket upgrade)%s\n" "$C_YELLOW" "$ok_count" "6" "$C_RESET"
    fi
    if [ -n "$ok_hosts" ]; then
        printf "Working hosts:\n"
        printf "  %b\n" "$ok_hosts"
    fi
    pause
}

advanced_menu() {
    while true; do
        show_header
        printf "%sAdvanced%s\n\n" "$C_BOLD" "$C_RESET"
        printf "  1) Show full status\n"
        printf "  2) Toggle verbose\n"
        printf "  3) Restart proxy\n"
        printf "  4) Show quick commands\n"
        printf "  5) Remove binary and runtime files\n"
        printf "  6) Configure SOCKS5 auth\n"
        printf "  7) Configure Telegram DC mapping\n"
        printf "  8) Configure update source\n"
        printf "  9) Toggle Cloudflare proxy\n"
        printf " 10) Toggle Cloudflare first\n"
        printf " 11) Set Cloudflare domain\n"
        printf " 12) Check Cloudflare domain\n"
        printf "  Enter) Back\n\n"
        printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
        read advanced_choice

        case "$advanced_choice" in
            1)
                show_header
                show_status
                pause
                ;;
            2)
                toggle_verbose
                ;;
            3)
                restart_proxy
                ;;
            4)
                show_quick_only
                ;;
            5)
                remove_all
                ;;
            6)
                configure_socks_auth
                ;;
            7)
                configure_dc_ip_mapping
                ;;
            8)
                configure_update_source
                ;;
            9)
                toggle_cf_proxy
                ;;
            10)
                toggle_cf_proxy_first
                ;;
            11)
                configure_cf_domain
                ;;
            12)
                check_cf_domain
                ;;
            *)
                return 0
                ;;
        esac
    done
}

menu() {
    running_now="0"
    if is_running; then
        running_now="1"
    fi

    autostart_now="0"
    if autostart_enabled; then
        autostart_now="1"
    fi

    show_header
    show_current_version
    printf "\n"
    show_telegram_settings
    printf "\n"
    show_menu_summary "$running_now" "$autostart_now"
    printf "\n%sActions%s\n" "$C_BOLD" "$C_RESET"
    printf "  1) Setup / Update\n"
    printf "  2) %s\n" "$(menu_proxy_action_label "$running_now")"
    printf "  3) %s\n" "$(menu_autostart_action_label "$autostart_now")"
    printf "  4) Show Telegram SOCKS5 settings\n"
    printf "  5) Advanced\n"
    printf "  6) Start in background\n"
    printf "  Enter) Exit\n\n"
    printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
    read choice

    case "$choice" in
        1) update_binary ;;
        2)
            if [ "$running_now" = "1" ]; then
                stop_proxy
            else
                start_proxy
            fi
            ;;
        3)
            if [ "$autostart_now" = "1" ]; then
                disable_autostart
            else
                enable_autostart
            fi
            ;;
        4) show_telegram_only ;;
        5) advanced_menu ;;
        6) start_proxy_background ;;
        *) exit 0 ;;
    esac
}
