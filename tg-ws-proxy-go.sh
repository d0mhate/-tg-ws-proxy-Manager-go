#!/bin/sh


set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/lib/colors.sh"
. "$SCRIPT_DIR/lib/globals.sh"
. "$SCRIPT_DIR/lib/utils.sh"
. "$SCRIPT_DIR/lib/platform.sh"
. "$SCRIPT_DIR/lib/config.sh"
. "$SCRIPT_DIR/lib/release.sh"
. "$SCRIPT_DIR/lib/process.sh"
. "$SCRIPT_DIR/lib/autostart.sh"
. "$SCRIPT_DIR/lib/install.sh"


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
    printf "%s+----------------------------------+%s\n" "$C_BLUE" "$C_RESET"
    printf "%s|%s %s%s Go manager%s            %s|%s\n" "$C_BLUE" "$C_RESET" "$C_BOLD" "$APP_NAME" "$C_RESET" "$C_BLUE" "$C_RESET"
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

show_update_source_settings() {
    channel="$(selected_update_channel 2>/dev/null || printf release)"
    ref="$(selected_update_ref 2>/dev/null || printf latest)"
    printf "%sUpdate source%s\n" "$C_BOLD" "$C_RESET"
    printf "  mode     : %s\n" "$channel"
    printf "  ref      : %s\n" "$ref"
}

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
        "" )
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
            tags="$current_tag
$tags"
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
        "" )
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
        "" )
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
    printf "  sh %s enable-autostart\n" "$0"
    printf "  sh %s disable-autostart\n" "$0"
    printf "  sh %s start\n" "$0"
    printf "  sh %s stop\n" "$0"
    printf "  sh %s restart\n" "$0"
    printf "  sh %s status\n" "$0"
    printf "  sh %s quick\n" "$0"
    printf "  sh %s telegram\n" "$0"
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

show_quick_only() {
    show_header
    show_quick_commands
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

menu_proxy_action_label() {
    if [ "$1" = "1" ]; then
        printf "Stop proxy"
    else
        printf "Run proxy in terminal"
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

    printf "%sSummary%s\n" "$C_BOLD" "$C_RESET"
    printf "  proxy     : %s\n" "$proxy_state"
    printf "  autostart : %s\n" "$autostart_state"
    printf "  verbose   : %s\n" "$verbose_state"
    printf "  track     : %s\n" "$(main_menu_track_label)"
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
            *)
                return 0
                ;;
        esac
    done
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

load_saved_settings

if [ "$COMMAND_MODE" = "1" ]; then
    case "$1" in
        disable-autostart|remove|help|-h|--help)
            ;;
        *)
            sync_autostart_config_if_enabled >/dev/null 2>&1 || true
            ;;
    esac

    rc=0
    case "$1" in
        install) install_binary; rc=$? ;;
        update) update_binary; rc=$? ;;
        persist) install_persistent_binary; rc=$? ;;
        enable-autostart) enable_autostart; rc=$? ;;
        disable-autostart) disable_autostart; rc=$? ;;
        start) start_proxy; rc=$? ;;
        start-background|start-bg) start_proxy_background; rc=$? ;;
        stop) stop_proxy; rc=$? ;;
        restart) restart_proxy; rc=$? ;;
        status) show_header; show_status; rc=$? ;;
        quick) show_quick_only; rc=$? ;;
        telegram) show_telegram_only; rc=$? ;;
        remove) remove_all; rc=$? ;;
        help|-h|--help) show_help; rc=$? ;;
        *)
            show_help
            exit 1
            ;;
    esac
    exit "$rc"
fi

while true; do
    menu
done
