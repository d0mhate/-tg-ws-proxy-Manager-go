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

confirm_yn() {
    # $1 = prompt text
    # Returns 0 if confirmed (y/Y), 1 otherwise
    # Uses single-keypress if stty is available, falls back to line read
    if [ -t 0 ] && [ -t 2 ] && [ "${TERM:-}" != "dumb" ] && command -v stty >/dev/null 2>&1; then
        restore_stty="$(stty -g 2>/dev/null || true)"
        if [ -n "$restore_stty" ]; then
            printf "%s [y/n]: " "$1" >&2
            stty -echo raw min 1 time 0 2>/dev/null || true
            key_hex="$(dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')"
            stty "$restore_stty" 2>/dev/null || true
            printf "\n" >&2
            case "$key_hex" in
                79|59) return 0 ;;  # y or Y
                *)     return 1 ;;
            esac
        fi
    fi
    printf "%s [y/N]: " "$1" >&2
    IFS= read -r _confirm_input
    case "$_confirm_input" in
        y|Y|yes|YES) return 0 ;;
        *)           return 1 ;;
    esac
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

configure_proxy_mode() {
    show_header
    printf "%sProxy mode%s\n" "$C_BOLD" "$C_RESET"
    printf "  current: %s\n" "$PROXY_MODE"
    printf "\nChoose mode:\n"
    printf "  1) socks5  - standard SOCKS5 (default)\n"
    printf "  2) mtproto - MTProto obfuscated proxy\n"
    printf "\nSelect [1-2] (Enter to keep current): "
    IFS= read -r mode_choice

    case "$mode_choice" in
        "")
            printf "\nNo changes made.\n"
            pause
            return 0
            ;;
        1|socks5)
            PROXY_MODE="socks5"
            ;;
        2|mtproto)
            PROXY_MODE="mtproto"
            ;;
        *)
            printf "\n%sUnknown mode%s\n" "$C_RED" "$C_RESET"
            pause
            return 1
            ;;
    esac

    write_settings_config || return 1
    printf "\n%sProxy mode set to %s%s\n" "$C_GREEN" "$PROXY_MODE" "$C_RESET"
    if [ "$PROXY_MODE" = "mtproto" ] && ! mt_secret_valid 2>/dev/null; then
        printf "\n%sSecret is not configured - set it in Advanced -> MTProto secret%s\n" "$C_YELLOW" "$C_RESET"
    fi
    prompt_restart_proxy_for_updated_settings
    pause
}

draw_mt_secret_picker() {
    _cur="$1"
    printf "Action (use arrows, Enter to confirm):\n" >&2
    for _opt in generate clear enter back; do
        if [ "$_opt" = "$_cur" ]; then
            printf "> %s\n" "$_opt" >&2
        else
            printf "  %s\n" "$_opt" >&2
        fi
    done
}

configure_mt_secret() {
    show_header
    printf "%sMTProto secret%s\n" "$C_BOLD" "$C_RESET"
    if mt_secret_valid 2>/dev/null; then
        printf "  current: %s\n" "$MT_SECRET"
    else
        printf "  current: %s<not set>%s\n" "$C_RED" "$C_RESET"
    fi
    printf "\n"

    new_secret=""
    action=""

    # --- Arrow picker ---
    if can_use_arrow_update_source_picker; then
        restore_stty=""
        if [ -z "$FORCE_ARROW_UPDATE_SOURCE_PICKER" ]; then
            restore_stty="$(stty -g 2>/dev/null || true)"
            if [ -n "$restore_stty" ]; then
                stty -echo -icanon min 1 time 0 2>/dev/null || true
            fi
        else
            restore_stty="forced"
        fi
        if [ -n "$restore_stty" ]; then
            action="generate"
            redraw="0"
            while true; do
                if [ "$redraw" = "1" ]; then
                    printf '\033[5A\033[J' >&2
                fi
                draw_mt_secret_picker "$action"
                redraw="1"
                first_hex="$(read_picker_hex_byte)"
                case "$first_hex" in
                    0a|0d) break ;;
                    1b)
                        second_hex="$(read_picker_hex_byte)"
                        third_hex="$(read_picker_hex_byte)"
                        case "$second_hex$third_hex" in
                            5b41|5b44)
                                case "$action" in
                                    generate) action="back"     ;;
                                    clear)    action="generate" ;;
                                    enter)    action="clear"    ;;
                                    back)     action="enter"    ;;
                                esac
                                ;;
                            5b42|5b43)
                                case "$action" in
                                    generate) action="clear"    ;;
                                    clear)    action="enter"    ;;
                                    enter)    action="back"     ;;
                                    back)     action="generate" ;;
                                esac
                                ;;
                        esac
                        ;;
                esac
            done
            if [ "$restore_stty" != "forced" ]; then
                stty "$restore_stty" 2>/dev/null || true
            fi
            printf "\n" >&2
        fi
    fi

    # --- Numbered fallback ---
    if [ -z "$action" ] && can_use_numbered_update_source_picker; then
        printf "  1) generate - random secret (plain, dd, or ee)\n"
        printf "  2) clear    - remove current secret\n"
        printf "  3) enter    - type hex value\n"
        printf "  4) back     - return without changes\n"
        printf "Select [1-4] (Enter to go back): "
        IFS= read -r sel
        printf "\n"
        case "$sel" in
            1|generate) action="generate" ;;
            2|clear)    action="clear"    ;;
            3|enter)    action="enter"    ;;
            4|back|"")
                printf "No changes made.\n"
                pause
                return 0
                ;;
            *)          action="enter"    ;;
        esac
    fi

    # --- Text fallback (no interactive terminal) ---
    if [ -z "$action" ]; then
        printf "Enter secret or 'gen' to generate.\n"
        printf "Formats: 32 hex (plain) | dd+32 hex (34 total) | ee+32 hex+hostname_hex (34+ total)\n"
        printf "Use 'clear' to remove the secret.\n"
        printf "Secret (empty to keep current): "
        IFS= read -r new_secret
        case "$new_secret" in
            "")
                printf "\nNo changes made.\n"
                pause
                return 0
                ;;
            generate|GENERATE|gen) action="generate" ;;
            clear|CLEAR|Clear)     action="clear"    ;;
            *)                     action="enter"    ;;
        esac
    fi

    # --- Execute action ---
    case "$action" in
        back)
            return 0
            ;;
        generate)
            printf "\nSecret format:\n"
            printf "  1) plain - 32 hex, standard obfuscation\n"
            printf "  2) dd    - 34 hex, padded intermediate (recommended)\n"
            printf "  3) ee    - FakeTLS, needs a hostname\n"
            printf "Select [1-3] (Enter for 2): "
            IFS= read -r _fmt_sel
            _gen_fmt="dd"
            _gen_domain=""
            case "$_fmt_sel" in
                1|plain) _gen_fmt="plain" ;;
                3|ee)
                    _gen_fmt="ee"
                    printf "Hostname for FakeTLS SNI (e.g. google.com): "
                    IFS= read -r _gen_domain
                    if [ -z "$_gen_domain" ]; then
                        printf "\n%sHostname is required for ee format%s\n" "$C_RED" "$C_RESET"
                        pause
                        return 1
                    fi
                    ;;
            esac
            generated="$(generate_mt_secret "$_gen_fmt" "$_gen_domain" 2>/dev/null || true)"
            if [ -z "$generated" ]; then
                printf "\n%sFailed to generate secret (need openssl, hexdump, or od + /dev/urandom)%s\n" "$C_RED" "$C_RESET"
                pause
                return 1
            fi
            new_secret="$generated"
            printf "\nGenerated: %s\n" "$new_secret"
            ;;
        clear)
            MT_SECRET=""
            write_settings_config || return 1
            printf "\n%sSecret cleared%s\n" "$C_GREEN" "$C_RESET"
            prompt_restart_proxy_for_updated_settings
            pause
            return 0
            ;;
        enter)
            if [ -z "$new_secret" ]; then
                printf "Formats: 32 hex (plain) | dd+32 hex | ee+32 hex+hostname_hex\n"
                printf "Secret: "
                IFS= read -r new_secret
            fi
            if [ -z "$new_secret" ]; then
                printf "\nNo changes made.\n"
                pause
                return 0
            fi
            ;;
    esac

    # Validate secret format
    case "$new_secret" in
        *[!0-9a-fA-F]*)
            printf "\n%sSecret must contain only hex characters (0-9, a-f)%s\n" "$C_RED" "$C_RESET"
            pause
            return 1
            ;;
    esac
    _new_secret_len=${#new_secret}
    case "$new_secret" in
        [dD][dD]*)
            if [ "$_new_secret_len" -ne 34 ]; then
                printf "\n%sdd-prefix secret must be exactly 34 hex chars, got %d%s\n" \
                    "$C_RED" "$_new_secret_len" "$C_RESET"
                pause
                return 1
            fi
            ;;
        [eE][eE]*)
            if [ "$_new_secret_len" -lt 34 ]; then
                printf "\n%see-prefix secret must be at least 34 hex chars, got %d%s\n" \
                    "$C_RED" "$_new_secret_len" "$C_RESET"
                pause
                return 1
            fi
            if [ "$(( _new_secret_len % 2 ))" -ne 0 ]; then
                printf "\n%sSecret must have an even number of hex chars%s\n" "$C_RED" "$C_RESET"
                pause
                return 1
            fi
            ;;
        *)
            if [ "$_new_secret_len" -ne 32 ]; then
                printf "\n%sPlain secret must be exactly 32 hex chars (16 bytes), got %d%s\n" \
                    "$C_RED" "$_new_secret_len" "$C_RESET"
                pause
                return 1
            fi
            ;;
    esac

    MT_SECRET="$new_secret"
    write_settings_config || return 1
    printf "\n%sSecret saved%s\n" "$C_GREEN" "$C_RESET"
    if [ -n "$MT_LINK_IP" ]; then
        _saved_link="$(mt_proxy_link 2>/dev/null || true)"
        [ -n "$_saved_link" ] && printf "%s\n" "$_saved_link"
    fi
    prompt_restart_proxy_for_updated_settings
    pause
}

_ip_is_private() {
    case "$1" in
        10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[01].*|127.*|169.254.*) return 0 ;;
        *) return 1 ;;
    esac
}

_fetch_public_ip() {
    _fpi=""
    if command -v curl >/dev/null 2>&1; then
        _fpi="$(curl -sf --max-time 4 https://ifconfig.me 2>/dev/null || true)"
    fi
    if [ -z "$_fpi" ] && command -v wget >/dev/null 2>&1; then
        _fpi="$(wget -q -O - --timeout=4 https://ifconfig.me 2>/dev/null || true)"
    fi
    [ -n "$_fpi" ] && printf "%s" "$_fpi"
}

_detect_local_wan_ip() {
    _dlw_iface="$(ip route show default 2>/dev/null | awk 'NR==1 { for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit} }')"
    if [ -n "$_dlw_iface" ]; then
        _dlw_ip="$(ip -4 addr show dev "$_dlw_iface" 2>/dev/null | awk '/inet / { split($2,a,"/"); print a[1]; exit }')"
        [ -n "$_dlw_ip" ] && printf "%s" "$_dlw_ip" && return 0
    fi
    ip route get 1.1.1.1 2>/dev/null | awk '{ for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit} }'
}

detect_wan_ip() {
    _dw_local="$(_detect_local_wan_ip 2>/dev/null || true)"
    if [ -n "$_dw_local" ] && ! _ip_is_private "$_dw_local"; then
        printf "%s" "$_dw_local"
        return 0
    fi
    # Double-NAT detected: try to get real public IP from external service
    _fetch_public_ip
}

configure_mt_link_ip() {
    show_header
    printf "%sMTProto public IP (for tg:// link)%s\n" "$C_BOLD" "$C_RESET"
    if [ -n "$MT_LINK_IP" ]; then
        printf "  current: %s\n" "$MT_LINK_IP"
    else
        printf "  current: <not set>\n"
    fi

    _suggested_ip=""
    _double_nat=0
    if [ -z "$MT_LINK_IP" ]; then
        _suggested_ip="$(lan_ip 2>/dev/null || true)"
        if [ -z "$_suggested_ip" ]; then
            _local_wan="$(_detect_local_wan_ip 2>/dev/null || true)"
            if [ -n "$_local_wan" ] && _ip_is_private "$_local_wan"; then
                _double_nat=1
            fi
            _suggested_ip="$(detect_wan_ip 2>/dev/null || true)"
        fi
    fi

    printf "\nEnter the public IP of this server (shown in the tg:// proxy link).\n"
    if [ "$_double_nat" -eq 1 ]; then
        printf "%sDouble NAT detected - your WAN IP is private.%s\n" "$C_YELLOW" "$C_RESET"
    fi
    printf "Use 'clear' to remove.\n"

    new_ip=""

    if [ "$_double_nat" -eq 1 ] && [ -n "$_suggested_ip" ]; then
        printf "\nDetected public IP: %s%s%s\n" "$C_BOLD" "$_suggested_ip" "$C_RESET"
        printf "Is this correct? [Y/n]: "
        IFS= read -r _confirm
        case "$_confirm" in
            ""|[Yy]|[Yy][Ee][Ss])
                new_ip="$_suggested_ip"
                ;;
            *)
                printf "IP: "
                IFS= read -r new_ip
                ;;
        esac
    elif [ -n "$MT_LINK_IP" ]; then
        printf "IP (Enter to keep %s): " "$MT_LINK_IP"
        IFS= read -r new_ip
    elif [ -n "$_suggested_ip" ]; then
        printf "IP (Enter for %s): " "$_suggested_ip"
        IFS= read -r new_ip
    else
        printf "IP: "
        IFS= read -r new_ip
    fi

    case "$new_ip" in
        "")
            if [ -n "$_suggested_ip" ] && [ -z "$MT_LINK_IP" ]; then
                new_ip="$_suggested_ip"
            else
                printf "\nNo changes made.\n"
                pause
                return 0
            fi
            ;;
        clear|CLEAR|Clear)
            MT_LINK_IP=""
            write_settings_config || return 1
            printf "\n%sPublic IP cleared%s\n" "$C_GREEN" "$C_RESET"
            pause
            return 0
            ;;
    esac

    MT_LINK_IP="$new_ip"
    write_settings_config || return 1
    printf "\n%sPublic IP saved%s\n" "$C_GREEN" "$C_RESET"
    _saved_link="$(mt_proxy_link 2>/dev/null || true)"
    [ -n "$_saved_link" ] && printf "%s\n" "$_saved_link"
    pause
}



show_mt_qr() {
    show_header
    printf "%sMTProto QR code%s\n" "$C_BOLD" "$C_RESET"

    link="$(mt_proxy_link 2>/dev/null || true)"
    if [ -z "$link" ]; then
        if ! mt_secret_valid 2>/dev/null; then
            printf "\n%sSecret not set%s - configure it via item 19.\n" "$C_RED" "$C_RESET"
        else
            printf "\n%sPublic IP not set%s - configure it via item 14.\n" "$C_RED" "$C_RESET"
        fi
        pause
        return 1
    fi
    printf "\n  %s\n\n" "$link"

    # Use the built-in qr subcommand from the proxy binary (no external deps)
    _bin="$(runtime_bin_path 2>/dev/null || true)"
    if [ -x "$_bin" ] && "$_bin" qr "$link" 2>/dev/null; then
        pause
        return 0
    fi

    # Fall back to system qrencode if available
    if qrencode --version >/dev/null 2>&1; then
        qrencode -t UTF8 "$link"
        pause
        return 0
    fi

    printf "  %sQR unavailable%s\n" "$C_YELLOW" "$C_RESET"
    printf "  Copy the link above and scan it on another device.\n"
    pause
}

show_socks5_qr() {
    show_header
    printf "%sSOCKS5 QR code%s\n" "$C_BOLD" "$C_RESET"

    link="$(socks5_proxy_link 2>/dev/null || true)"
    if [ -z "$link" ]; then
        printf "\n%sPublic IP not set%s\n" "$C_RED" "$C_RESET"
        printf "Set it via Settings - item 14.\n"
        pause
        return 1
    fi
    printf "\n  %s\n\n" "$link"

    _bin="$(runtime_bin_path 2>/dev/null || true)"
    if [ -x "$_bin" ] && "$_bin" qr "$link" 2>/dev/null; then
        pause
        return 0
    fi

    if qrencode --version >/dev/null 2>&1; then
        qrencode -t UTF8 "$link"
        pause
        return 0
    fi

    printf "  %sQR unavailable%s\n" "$C_YELLOW" "$C_RESET"
    printf "  Copy the link above and scan it on another device.\n"
    pause
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

configure_listen_port() {
    show_header
    printf "%sListen port%s\n" "$C_BOLD" "$C_RESET"
    printf "  current: %s\n" "$LISTEN_PORT"
    printf "\nPort the proxy will listen on (1-65535).\n"
    printf "Port (Enter to keep %s): " "$LISTEN_PORT"
    IFS= read -r new_port

    case "$new_port" in
        "")
            printf "\nNo changes made.\n"
            pause
            return 0
            ;;
    esac

    case "$new_port" in
        *[!0-9]*)
            printf "\n%sPort must be a number%s\n" "$C_RED" "$C_RESET"
            pause
            return 1
            ;;
    esac
    if [ "$new_port" -lt 1 ] || [ "$new_port" -gt 65535 ] 2>/dev/null; then
        printf "\n%sPort must be between 1 and 65535%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    LISTEN_PORT="$new_port"
    write_settings_config || return 1
    printf "\n%sPort saved: %s%s\n" "$C_GREEN" "$LISTEN_PORT" "$C_RESET"
    prompt_restart_proxy_for_updated_settings
    pause
}

configure_pool_size() {
    while true; do
        show_header
        printf "%sWebSocket pool size%s\n\n" "$C_BOLD" "$C_RESET"
        printf "Current value: %s\n\n" "$POOL_SIZE"
        printf "Enter a number between 0 and 64.\n"
        printf "Use 0 to disable pre-opened pooled connections.\n\n"
        printf "New pool size: "
        IFS= read -r new_pool_size

        if [ -z "$new_pool_size" ]; then
            return 0
        fi

        case "$new_pool_size" in
            *[!0-9]*)
                printf "\n%sPool size must be a whole number between 0 and 64%s\n" "$C_RED" "$C_RESET"
                pause
                continue
                ;;
        esac

        if [ "$new_pool_size" -gt 64 ]; then
            printf "\n%sPool size must be between 0 and 64%s\n" "$C_RED" "$C_RESET"
            pause
            continue
        fi

        POOL_SIZE="$new_pool_size"
        write_settings_config || {
            pause
            continue
        }
        printf "\n%sPool size saved: %s%s\n" "$C_GREEN" "$POOL_SIZE" "$C_RESET"
        prompt_restart_proxy_for_updated_settings
        pause
        return 0
    done
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

choose_preview_branch_numbered() {
    _cpbn_current="${1:-}"
    _cpbn_branches="$(list_preview_branches 20 2>/dev/null || true)"

    if [ -n "$_cpbn_current" ] && ! printf "%s\n" "$_cpbn_branches" | grep -Fx "$_cpbn_current" >/dev/null 2>&1; then
        if [ -n "$_cpbn_branches" ]; then
            _cpbn_branches="$(printf '%s\n%s' "$_cpbn_current" "$_cpbn_branches")"
        else
            _cpbn_branches="$_cpbn_current"
        fi
    fi

    if [ -z "$_cpbn_branches" ]; then
        if [ -n "$_cpbn_current" ]; then
            printf "Preview branch (Enter to keep %s): " "$_cpbn_current" >&2
        else
            printf "Preview branch (for example: preview-channel): " >&2
        fi
        IFS= read -r _cpbn_typed
        if [ -z "$_cpbn_typed" ]; then
            if [ -n "$_cpbn_current" ]; then
                printf "%s" "$_cpbn_current"
                return 0
            fi
            printf "\n%sPreview branch cannot be empty%s\n" "$C_RED" "$C_RESET" >&2
            return 1
        fi
        printf "%s" "$_cpbn_typed"
        return 0
    fi

    printf "Preview branch:\n" >&2
    _cpbn_count=0
    _cpbn_old_ifs="$IFS"
    IFS='
'
    for _cpbn_b in $_cpbn_branches; do
        [ -n "$_cpbn_b" ] || continue
        _cpbn_count=$((_cpbn_count + 1))
        printf "  %s) %s\n" "$_cpbn_count" "$_cpbn_b" >&2
    done
    IFS="$_cpbn_old_ifs"

    _cpbn_manual=$((_cpbn_count + 1))
    printf "  %s) enter branch manually\n" "$_cpbn_manual" >&2
    if [ -n "$_cpbn_current" ]; then
        printf "Select branch [1-%s] (Enter for %s): " "$_cpbn_manual" "$_cpbn_current" >&2
    else
        printf "Select branch [1-%s]: " "$_cpbn_manual" >&2
    fi
    IFS= read -r _cpbn_sel

    case "$_cpbn_sel" in
        "")
            if [ -n "$_cpbn_current" ]; then
                printf "%s" "$_cpbn_current"
                return 0
            fi
            printf "\n%sPreview branch cannot be empty%s\n" "$C_RED" "$C_RESET" >&2
            return 1
            ;;
        "$_cpbn_manual"|m|M|manual)
            if [ -n "$_cpbn_current" ]; then
                printf "Preview branch (Enter to keep %s): " "$_cpbn_current" >&2
            else
                printf "Preview branch: " >&2
            fi
            IFS= read -r _cpbn_typed
            if [ -z "$_cpbn_typed" ]; then
                if [ -n "$_cpbn_current" ]; then
                    printf "%s" "$_cpbn_current"
                    return 0
                fi
                printf "\n%sPreview branch cannot be empty%s\n" "$C_RED" "$C_RESET" >&2
                return 1
            fi
            printf "%s" "$_cpbn_typed"
            return 0
            ;;
        *[!0-9]*)
            printf "%s" "$_cpbn_sel"
            return 0
            ;;
    esac

    _cpbn_chosen="$(printf "%s\n" "$_cpbn_branches" | sed -n "${_cpbn_sel}p" 2>/dev/null || true)"
    if [ -z "$_cpbn_chosen" ]; then
        printf "\n%sUnknown branch selection%s\n" "$C_RED" "$C_RESET" >&2
        return 1
    fi
    printf "%s" "$_cpbn_chosen"
    return 0
}

choose_preview_branch() {
    _cpb_current="${1:-}"

    if can_use_numbered_update_source_picker; then
        choose_preview_branch_numbered "$_cpb_current"
        return $?
    fi

    if [ -n "$_cpb_current" ]; then
        printf "Preview branch (Enter to keep %s): " "$_cpb_current" >&2
    else
        printf "Preview branch (for example: preview-channel): " >&2
    fi
    IFS= read -r _cpb_typed
    if [ -z "$_cpb_typed" ]; then
        if [ -n "$_cpb_current" ]; then
            printf "%s" "$_cpb_current"
            return 0
        fi
        printf "\n%sPreview branch cannot be empty%s\n" "$C_RED" "$C_RESET" >&2
        return 1
    fi
    printf "%s" "$_cpb_typed"
    return 0
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
            new_ref="$(choose_preview_branch "$current_preview_branch")" || {
                pause
                return 1
            }

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

toggle_cf_balance() {
    if [ "$CF_BALANCE" = "1" ]; then
        CF_BALANCE="0"
    else
        CF_BALANCE="1"
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
    if [ "$CF_PROXY" != "1" ]; then
        printf "%sWarning:%s CF proxy is currently off. Saving a domain does not enable CF routing.\n" "$C_YELLOW" "$C_RESET"
    fi
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
    if [ "$CF_PROXY" != "1" ]; then
        printf "%sWarning:%s domain saved, but CF route is disabled until you turn on CF proxy.\n" "$C_YELLOW" "$C_RESET"
    fi
    prompt_restart_proxy_for_updated_settings
    pause
}

check_cf_endpoint() {
    host="$1"
    CHECK_CF_ENDPOINT_STATUS=""

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
        CHECK_CF_ENDPOINT_STATUS="no tool"
        return 1
    fi

    status_line="$(printf "%s\n" "$output" | sed -n '/^HTTP\//{p;q;}')"
    if [ -n "$status_line" ]; then
        case "$status_line" in
            *"101 Switching Protocols"*)
                CHECK_CF_ENDPOINT_STATUS="tls/ws ok"
                return 0
                ;;
            *)
                CHECK_CF_ENDPOINT_STATUS="tls/ws fail"
                return 1
                ;;
        esac
    fi

    if printf "%s" "$output" | grep -E "CONNECTED|Verification: OK|SSL handshake has read|depth=" >/dev/null 2>&1; then
        CHECK_CF_ENDPOINT_STATUS="tls/no http"
        return 1
    fi

    CHECK_CF_ENDPOINT_STATUS="conn err"
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

    printf "\nChecking Cloudflare websocket endpoints\n\n"
    ok_count=0
    total_hosts=0
    _cf_interrupted=0
    _cf_domains=""
    _cf_old_ifs="$IFS"
    IFS=','
    for _cf_domain_raw in $check_domain; do
        _cf_domain_trimmed="$(printf "%s" "$_cf_domain_raw" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        [ -n "$_cf_domain_trimmed" ] || continue
        if [ -z "$_cf_domains" ]; then
            _cf_domains="$_cf_domain_trimmed"
        else
            _cf_domains="$_cf_domains
$_cf_domain_trimmed"
        fi
    done
    IFS="$_cf_old_ifs"

    if [ -z "$_cf_domains" ]; then
        printf "%sNo valid Cloudflare domains provided%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi

    _cf_col_w=6
    _cf_domain_count=0
    _cf_old_ifs="$IFS"
    IFS='
'
    for _cf_domain_line in $_cf_domains; do
        [ -n "$_cf_domain_line" ] || continue
        _cf_len=${#_cf_domain_line}
        [ "$_cf_len" -gt "$_cf_col_w" ] && _cf_col_w="$_cf_len"
        _cf_domain_count=$((_cf_domain_count + 1))
    done
    IFS="$_cf_old_ifs"
    _cf_total_endpoints=$((_cf_domain_count * 6))

    printf "Domains:\n"
    _cf_old_ifs="$IFS"
    IFS='
'
    for _cf_domain_line in $_cf_domains; do
        [ -n "$_cf_domain_line" ] || continue
        printf "  %s\n" "$_cf_domain_line"
    done
    printf "\n"
    printf "Testing %d endpoints in parallel..." "$_cf_total_endpoints"
    printf "\n\n"

    _cf_old_ifs="$IFS"
    IFS='
'
    {
        for _cf_domain_name in $_cf_domains; do
            [ -n "$_cf_domain_name" ] || continue
            for prefix in kws1 kws2 kws3 kws4 kws5 kws203; do
                (
                    if check_cf_endpoint "$prefix.$_cf_domain_name"; then
                        _cf_result="ok"
                    else
                        _cf_result="fail"
                    fi
                    printf '%s|%s|%s|%s\n' "$_cf_domain_name" "$prefix" "$_cf_result" "$CHECK_CF_ENDPOINT_STATUS"
                ) &
            done
        done
        wait
    } | awk -F'|' \
        -v colw="$_cf_col_w" \
        -v cgreen="$C_GREEN" \
        -v cyellow="$C_YELLOW" \
        -v cred="$C_RED" \
        -v creset="$C_RESET" \
        -v total_hosts="$_cf_total_endpoints" '
BEGIN {
    split("kws1 kws2 kws3 kws4 kws5 kws203", prefixes, " ")
    cellw = 12
    domain_count = 0
    ok_count = 0
}
{
    key = $1 SUBSEP $2
    result[key] = $3
    status[key] = $4
    if (!seen[$1]++) {
        order[++domain_count] = $1
    }
    if ($3 == "ok") {
        row_ok[$1]++
        ok_count++
    }
}
END {
    sep = ""
    for (i = 0; i < colw; i++) {
        sep = sep "-"
    }

    printf "%-*s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s | %-9s\n", \
        colw, "domain", "kws1", "kws2", "kws3", "kws4", "kws5", "kws203", "summary"
    printf "%s-|-%s-|-%s-|-%s-|-%s-|-%s-|-%s-|-%s\n", \
        sep, "------------", "------------", "------------", \
        "------------", "------------", "------------", "---------"

    domains_alive = 0
    domains_dead = 0

    for (i = 1; i <= domain_count; i++) {
        d = order[i]
        row = sprintf("%-*s ", colw, d)
        row_pass = 0
        for (j = 1; j <= 6; j++) {
            p = prefixes[j]
            key = d SUBSEP p
            st = status[key]
            if (st == "") {
                st = "no data"
            }
            if (result[key] == "ok") {
                color = cgreen
                row_pass++
            } else if (st == "tls/ws fail") {
                color = cyellow
            } else {
                color = cred
            }
            row = row sprintf("| %s%-12s%s ", color, st, creset)
        }
        row = row sprintf("| %d/6 ok", row_pass)
        print row
        if (row_pass > 0) {
            domains_alive++
        } else {
            domains_dead++
        }
    }

    print ""
    if (domain_count > 1) {
        if (domains_dead == 0) {
            printf "%sDomains: all %d alive%s\n", cgreen, domain_count, creset
        } else if (domains_alive == 0) {
            printf "%sDomains: all %d dead%s\n", cred, domain_count, creset
        } else {
            printf "%sDomains: %d/%d alive, %d dead%s\n", cyellow, domains_alive, domain_count, domains_dead, creset
        }
    }
    if (ok_count == total_hosts) {
        printf "%sCloudflare proxy: all tested hosts support websocket upgrade%s\n", cgreen, creset
    } else if (ok_count == 0) {
        printf "%sCloudflare proxy: none of the tested hosts support websocket upgrade%s\n", cred, creset
    } else {
        printf "%sCloudflare proxy: partially works (%d/%d hosts passed websocket upgrade)%s\n", cyellow, ok_count, total_hosts, creset
    }
}'
    IFS="$_cf_old_ifs"
    pause
}

check_mt_upstream_proxies() {
    show_header
    printf "%sTest MTProto upstream proxies%s\n\n" "$C_BOLD" "$C_RESET"
    _cmu_bin="$(runtime_bin_path 2>/dev/null || true)"
    if [ ! -x "$_cmu_bin" ]; then
        printf "%sProxy binary not found - cannot run test.%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    _cmu_ok=0
    _cmu_total=0
    _cmu_interrupted=0
    trap '_cmu_interrupted=1' INT
    _cmu_old_ifs="$IFS"
    IFS=','
    for _cmu_e in $MT_UPSTREAM_PROXIES; do
        _cmu_e="$(printf "%s" "$_cmu_e" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
        [ -n "$_cmu_e" ] || continue
        [ "$_cmu_interrupted" = "0" ] || break
        _cmu_host="$(printf "%s" "$_cmu_e" | cut -d: -f1)"
        _cmu_port="$(printf "%s" "$_cmu_e" | cut -d: -f2)"
        _cmu_label="${_cmu_host}:${_cmu_port}"
        _cmu_total=$((_cmu_total + 1))
        printf "  %-28s checking...\r" "$_cmu_label"
        _cmu_line="$("$_cmu_bin" probe-upstream "${_cmu_host}:${_cmu_port}" 2>/dev/null || true)"
        _cmu_status="$(printf "%s" "$_cmu_line" | cut -d' ' -f2)"
        _cmu_detail="$(printf "%s" "$_cmu_line" | cut -d' ' -f3)"
        case "$_cmu_status" in
            ok)
                printf "  %-28s %stcp ok | %s%s\n" "$_cmu_label" "$C_GREEN" "$_cmu_detail" "$C_RESET"
                _cmu_ok=$((_cmu_ok + 1))
                ;;
            fail)
                printf "  %-28s %sfailed | %s%s\n" "$_cmu_label" "$C_RED" "$_cmu_detail" "$C_RESET"
                ;;
            *)
                printf "  %-28s %sunknown error%s\n" "$_cmu_label" "$C_RED" "$C_RESET"
                ;;
        esac
    done
    IFS="$_cmu_old_ifs"
    trap - INT
    if [ "$_cmu_interrupted" = "1" ]; then
        printf "\nCancelled.\n"
    else
        printf "\n%d of %d reachable.\n" "$_cmu_ok" "$_cmu_total"
    fi
    pause
}

configure_mt_upstream_proxies() {
    while true; do
        show_header
        printf "%sMTProto upstream proxies%s\n" "$C_BOLD" "$C_RESET"
        printf "\nUsed as fallback when WebSocket to Telegram is unavailable.\n"
        printf "Format: HOST:PORT:SECRET\n\n"

        _up_count=0
        if [ -n "$MT_UPSTREAM_PROXIES" ]; then
            _up_old_ifs="$IFS"
            IFS=','
            for _up_e in $MT_UPSTREAM_PROXIES; do
                _up_e="$(printf "%s" "$_up_e" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
                [ -n "$_up_e" ] || continue
                _up_count=$((_up_count + 1))
                _up_host="$(printf "%s" "$_up_e" | cut -d: -f1)"
                _up_port="$(printf "%s" "$_up_e" | cut -d: -f2)"
                _up_sec="$(printf "%s" "$_up_e" | cut -d: -f3-)"
                printf "  %d. %s:%s  [%s]\n" "$_up_count" "$_up_host" "$_up_port" "$(upstream_secret_kind "$_up_sec")"
            done
            IFS="$_up_old_ifs"
        fi
        if [ "$_up_count" -eq 0 ]; then
            printf "  (none)\n"
        fi

        printf "\n  1) Add proxy\n"
        if [ "$_up_count" -gt 0 ]; then
            printf "  2) Test\n"
            printf "  3) Remove proxy\n"
            printf "  4) Clear all\n"
        fi
        printf "  Enter) Back\n\n"
        printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
        IFS= read -r _up_choice

        case "$_up_choice" in
            1|add)
                printf "\nEnter HOST:PORT:SECRET\n"
                printf "  %sexample:%s proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f\n" "$C_DIM" "$C_RESET"
                printf "\n%sTip:%s you can also paste a block directly from a proxy channel:\n" "$C_BOLD" "$C_RESET"
                printf "  %sServer: proxy.example.com\n" "$C_DIM"
                printf "  Port: 443\n"
                printf "  Secret: ddf0e1d2c3b4a5968778695a4b3c2d1e0f%s\n" "$C_RESET"
                printf "\nEntry: "
                IFS= read -r _up_new
                if [ -z "$_up_new" ]; then
                    continue
                fi
                case "$_up_new" in
                    [Ss]erver:*)
                        _up_srv="$(printf "%s" "$_up_new" | sed 's/^[^:]*:[[:space:]]*//')"
                        _up_prt=""
                        _up_sec=""
                        IFS= read -r _up_l2
                        case "$_up_l2" in
                            [Pp]ort:*)
                                _up_prt="$(printf "%s" "$_up_l2" | sed 's/^[^:]*:[[:space:]]*//')" ;;
                        esac
                        IFS= read -r _up_l3
                        case "$_up_l3" in
                            [Ss]ecret:*|[Kk]ey:*)
                                _up_sec="$(printf "%s" "$_up_l3" | sed 's/^[^:]*:[[:space:]]*//')" ;;
                        esac
                        if [ -n "$_up_srv" ] && [ -n "$_up_prt" ] && [ -n "$_up_sec" ]; then
                            _up_new="${_up_srv}:${_up_prt}:${_up_sec}"
                        fi
                        ;;
                esac
                _up_new="$(normalize_upstream_proxy_entry "$_up_new")"
                if ! validate_upstream_proxy_entry "$_up_new" 2>/dev/null; then
                    printf "\n%sInvalid entry. Expected HOST:PORT:SECRET\n" "$C_RED"
                    printf "SECRET: 32 hex (plain) | 34 hex dd-prefix | 34+ hex ee-prefix%s\n" "$C_RESET"
                    pause
                    continue
                fi
                if [ -z "$MT_UPSTREAM_PROXIES" ]; then
                    MT_UPSTREAM_PROXIES="$_up_new"
                else
                    MT_UPSTREAM_PROXIES="$MT_UPSTREAM_PROXIES,$_up_new"
                fi
                write_settings_config || { pause; continue; }
                printf "\n%sProxy added%s\n" "$C_GREEN" "$C_RESET"
                prompt_restart_proxy_for_updated_settings
                pause
                ;;
            2|test)
                [ "$_up_count" -gt 0 ] || continue
                check_mt_upstream_proxies
                ;;
            3|remove)
                [ "$_up_count" -gt 0 ] || continue
                if [ "$_up_count" -eq 1 ]; then
                    MT_UPSTREAM_PROXIES=""
                    write_settings_config || { pause; continue; }
                    printf "\n%sProxy removed%s\n" "$C_GREEN" "$C_RESET"
                    prompt_restart_proxy_for_updated_settings
                    pause
                    continue
                fi
                printf "\nRemove which? [1-%d]: " "$_up_count"
                IFS= read -r _up_rm
                case "$_up_rm" in *[!0-9]*|"") continue ;; esac
                [ "$_up_rm" -ge 1 ] && [ "$_up_rm" -le "$_up_count" ] || continue
                _up_i=0
                _up_new_list=""
                _up_old_ifs="$IFS"
                IFS=','
                for _up_e in $MT_UPSTREAM_PROXIES; do
                    _up_e="$(printf "%s" "$_up_e" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
                    [ -n "$_up_e" ] || continue
                    _up_i=$((_up_i + 1))
                    [ "$_up_i" -eq "$_up_rm" ] && continue
                    if [ -z "$_up_new_list" ]; then
                        _up_new_list="$_up_e"
                    else
                        _up_new_list="$_up_new_list,$_up_e"
                    fi
                done
                IFS="$_up_old_ifs"
                MT_UPSTREAM_PROXIES="$_up_new_list"
                write_settings_config || { pause; continue; }
                printf "\n%sProxy removed%s\n" "$C_GREEN" "$C_RESET"
                prompt_restart_proxy_for_updated_settings
                pause
                ;;
            4|clear)
                [ "$_up_count" -gt 0 ] || continue
                MT_UPSTREAM_PROXIES=""
                write_settings_config || { pause; continue; }
                printf "\n%sAll upstream proxies cleared%s\n" "$C_GREEN" "$C_RESET"
                prompt_restart_proxy_for_updated_settings
                pause
                ;;
            *) return 0 ;;
        esac
    done
}

advanced_menu() {
    while true; do
        show_header
        printf "%sAdvanced%s\n" "$C_BOLD" "$C_RESET"
        printf "\n  Info\n"
        printf "  1) Full status\n"
        printf "  2) Proxy settings\n"
        printf "  3) Quick commands\n"
        printf "\n  Proxy\n"
        if [ "$VERBOSE" = "1" ]; then
            printf "  4) Toggle verbose (%son%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf "  4) Toggle verbose (%soff%s)\n" "$C_DIM" "$C_RESET"
        fi
        printf "  5) Restart proxy\n"
        printf "\n  Cloudflare\n"
        if [ "$CF_PROXY" = "1" ]; then
            printf "  6) Toggle proxy (%son%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf "  6) Toggle proxy (%soff%s)\n" "$C_DIM" "$C_RESET"
        fi
        if [ "$CF_PROXY_FIRST" = "1" ]; then
            printf "  7) Toggle order (%sfirst%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf "  7) Toggle order (%sfallback%s)\n" "$C_DIM" "$C_RESET"
        fi
        if [ "$CF_BALANCE" = "1" ]; then
            printf "  8) Toggle balance (%son%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf "  8) Toggle balance (%soff%s)\n" "$C_DIM" "$C_RESET"
        fi
        printf "  9) Set domain\n"
        printf " 10) Check domain\n"
        printf "\n  Settings\n"
        printf " 11) SOCKS5 auth\n"
        printf " 12) DC mapping\n"
        printf " 13) Port (%s%s%s)\n" "$C_GREEN" "$LISTEN_PORT" "$C_RESET"
        printf " 14) Pool size (%s%s%s)\n" "$C_GREEN" "$POOL_SIZE" "$C_RESET"
        if [ -n "$MT_LINK_IP" ]; then
            printf " 15) Public IP (%s%s%s)\n" "$C_GREEN" "$MT_LINK_IP" "$C_RESET"
        else
            printf " 15) Public IP (%snot set%s)\n" "$C_DIM" "$C_RESET"
        fi
        printf " 16) Show QR code\n"
        printf " 17) Update source\n"
        printf " 18) Remove binary\n"
        printf "\n  MTProto\n"
        if [ "$PROXY_MODE" = "mtproto" ]; then
            printf " 19) Mode (%smtproto%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf " 19) Mode (%ssocks5%s)\n" "$C_DIM" "$C_RESET"
        fi
        if mt_secret_valid 2>/dev/null; then
            _sec_type="$(mt_secret_type 2>/dev/null || printf "set")"
            printf " 20) Secret (%s%s%s)\n" "$C_GREEN" "$_sec_type" "$C_RESET"
        else
            printf " 20) Secret (%snot set%s)\n" "$C_RED" "$C_RESET"
        fi
        _adv_up_count=0
        if [ -n "$MT_UPSTREAM_PROXIES" ]; then
            _adv_old_ifs="$IFS"; IFS=','
            for _adv_e in $MT_UPSTREAM_PROXIES; do
                [ -n "$_adv_e" ] && _adv_up_count=$((_adv_up_count + 1))
            done
            IFS="$_adv_old_ifs"
        fi
        if [ "$_adv_up_count" -gt 0 ]; then
            printf " 21) Upstream proxies (%s%d set%s)\n" "$C_GREEN" "$_adv_up_count" "$C_RESET"
        else
            printf " 21) Upstream proxies (%snone%s)\n" "$C_DIM" "$C_RESET"
        fi
        printf "\n  Enter) Back\n\n"
        printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
        read advanced_choice

        case "$advanced_choice" in
            1)
                show_header
                show_status
                pause
                ;;
            2)
                show_telegram_only
                ;;
            3)
                show_quick_only
                ;;
            4)
                toggle_verbose
                ;;
            5)
                restart_proxy
                ;;
            6)
                toggle_cf_proxy
                ;;
            7)
                toggle_cf_proxy_first
                ;;
            8)
                toggle_cf_balance
                ;;
            9)
                configure_cf_domain
                ;;
            10)
                check_cf_domain
                ;;
            11)
                configure_socks_auth
                ;;
            12)
                configure_dc_ip_mapping
                ;;
            13)
                configure_listen_port
                ;;
            14)
                configure_pool_size
                ;;
            15)
                configure_mt_link_ip
                ;;
            16)
                if [ "$PROXY_MODE" = "mtproto" ]; then
                    show_mt_qr
                else
                    show_socks5_qr
                fi
                ;;
            17)
                configure_update_source
                ;;
            18)
                remove_all
                ;;
            19)
                configure_proxy_mode
                ;;
            20)
                configure_mt_secret
                ;;
            21)
                configure_mt_upstream_proxies
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
    show_telegram_settings_compact
    printf "\n"
    show_menu_summary "$running_now" "$autostart_now"
    printf "\n%sActions%s\n" "$C_BOLD" "$C_RESET"
    printf "  1) Setup / Update\n"
    printf "  2) %s\n" "$(menu_proxy_action_label "$running_now")"
    printf "  3) %s\n" "$(menu_autostart_action_label "$autostart_now")"
    printf "  4) Advanced\n"
    printf "  Enter) Exit\n\n"
    printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
    read choice

    case "$choice" in
        1)
            if confirm_yn "  Install / update binary?"; then
                update_binary
            fi
            ;;
        2)
            if [ "$running_now" = "1" ]; then
                stop_proxy
            else
                printf "  (t)erminal or (b)ackground [t]: "
                IFS= read -r start_mode
                case "$start_mode" in
                    b|B|bg|background)
                        start_proxy_background
                        ;;
                    *)
                        start_proxy
                        ;;
                esac
            fi
            ;;
        3)
            if [ "$autostart_now" = "1" ]; then
                disable_autostart
            else
                enable_autostart
            fi
            ;;
        4) advanced_menu ;;
        *) exit 0 ;;
    esac
}
