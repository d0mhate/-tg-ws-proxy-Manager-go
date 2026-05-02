#!/bin/sh

export COMMAND_MODE="1"
if [ -n "${BATS_TEST_DIRNAME:-}" ]; then
    MENU_LIB_DIR=$(CDPATH= cd -- "$BATS_TEST_DIRNAME/../lib" && pwd)
else
    MENU_FIXTURE_SELF="${BASH_SOURCE:-$0}"
    MENU_FIXTURE_DIR=$(CDPATH= cd -- "$(dirname -- "$MENU_FIXTURE_SELF")" && pwd)
    MENU_LIB_DIR=$(CDPATH= cd -- "$MENU_FIXTURE_DIR/../../lib" && pwd)
fi
export TERM="dumb"
export FORCE_ARROW_UPDATE_SOURCE_PICKER=""
export FORCE_NUMBERED_UPDATE_SOURCE_PICKER=""

export C_BOLD=""
export C_RESET=""
export C_RED=""
export C_GREEN=""
export C_YELLOW=""
export C_DIM=""
export C_CYAN=""

export PROXY_MODE="socks5"
export MT_SECRET=""
export MT_LINK_IP=""
export SOCKS_USERNAME=""
export SOCKS_PASSWORD=""
export LISTEN_PORT="1080"
export POOL_SIZE="4"
export DC_IPS=""
export VERBOSE="0"
export CF_PROXY="0"
export CF_PROXY_FIRST="0"
export CF_BALANCE="1"
export CF_DOMAIN=""
export CF_BUILTIN_DOMAINS_OBF='\160\143\154\145\141\144\056\143\157\056\165\153\054\157\146\146\163\150\157\162\056\143\157\056\165\153\054\143\141\153\145\151\163\141\154\151\145\056\143\157\056\165\153\054\156\157\163\153\157\155\156\141\144\172\157\162\056\143\157\056\165\153\054\154\157\166\145\164\162\165\145\056\143\157\056\165\153\054\163\157\162\157\153\144\166\141\056\143\157\056\165\153\054\160\171\141\164\144\145\163\171\141\164\144\166\141\056\143\157\056\165\153\054\153\141\162\164\157\163\150\153\141\056\143\157\056\165\153'
export UPDATE_CHANNEL="release"
export PREVIEW_BRANCH=""
export PREVIEW_BRANCH_FROM_ENV=""
export RELEASE_TAG=""
export MIN_PINNED_RELEASE_TAG="v1.1.29"
export MT_UPSTREAM_PROXIES=""
export TEST_GENERATED_SECRET="dd00112233445566778899aabbccddeeff"
export TEST_MT_PROXY_LINK=""
export TEST_SOCKS5_PROXY_LINK=""
export TEST_LAN_IP=""
export TEST_SHOW_STATUS_CALLED=""
export TEST_SHOW_TELEGRAM_ONLY_CALLED=""
export TEST_SHOW_QUICK_ONLY_CALLED=""
export TEST_UPDATE_BINARY_CALLED=""
export TEST_STOP_PROXY_CALLED=""
export TEST_START_PROXY_CALLED=""
export TEST_START_PROXY_BG_CALLED=""
export TEST_ENABLE_AUTOSTART_CALLED=""
export TEST_DISABLE_AUTOSTART_CALLED=""
export TEST_ADVANCED_MENU_CALLED=""
export TEST_RESTART_PROXY_CALLED=""
export TEST_REMOVE_ALL_CALLED=""
export TEST_REMOVE_ALL_STATUS="0"
export TEST_RESTART_PROMPT_CALLED=""
export TEST_SYNC_AUTOSTART_CALLED=""

show_header() { :; }
show_update_source_settings() { :; }
show_dc_ip_mapping_settings() { :; }
show_telegram_settings() { :; }
pause() { :; }
prompt_restart_proxy_for_updated_settings() {
    TEST_RESTART_PROMPT_CALLED="1"
    : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/restart_prompt_called"
}
sync_autostart_config_if_enabled() {
    TEST_SYNC_AUTOSTART_CALLED="1"
    : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/sync_autostart_called"
}

normalize_version() {
    case "$1" in
        v[0-9]*)
            printf "%s" "$1"
            return 0
            ;;
    esac
    return 1
}

release_tag_meets_minimum() {
    [ "$1" != "v1.1.28" ]
}

recent_release_tags() {
    printf "%s\n" "v1.2.3" "v1.2.0"
}

write_update_source_state() {
    printf "%s|%s" "$1" "$2" > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/update_state"
}

write_settings_config() {
    printf "ok" > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/write_settings_called"
}

cf_builtin_domains() {
    if [ -n "${CF_BUILTIN_DOMAINS:-}" ]; then
        printf "%s" "$CF_BUILTIN_DOMAINS"
        return 0
    fi
    printf '%b' "${CF_BUILTIN_DOMAINS_OBF:-}"
}

normalize_cf_domain_list() {
    value="$1"
    [ -n "$value" ] || return 1
    awk -v input="$value" '
        function trim(s) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
            return s
        }
        BEGIN {
            count = split(input, parts, ",")
            out = ""
            for (i = 1; i <= count; i++) {
                part = trim(parts[i])
                if (part == "" || seen[part]++) continue
                out = (out == "" ? part : out "," part)
            }
            if (out == "") exit 1
            print out
        }
    '
}

custom_cf_domains() {
    normalize_cf_domain_list "${CF_DOMAIN:-}" 2>/dev/null
}

resolved_cf_domains() {
    custom_domains="$(custom_cf_domains 2>/dev/null || true)"
    if [ -n "$custom_domains" ]; then
        printf "%s" "$custom_domains"
        return 0
    fi
    cf_builtin_domains
}

resolved_cf_domain_source() {
    if [ -n "$(custom_cf_domains 2>/dev/null || true)" ]; then
        printf "custom"
    else
        printf "builtin"
    fi
}

normalize_dc_ip_list() {
    case "$1" in
        "2:149.154.167.220, 4:149.154.167.220")
            printf "%s" "$1"
            ;;
        *)
            return 1
            ;;
    esac
}

mt_secret_valid() {
    [ -n "$MT_SECRET" ] || return 1
    case "$MT_SECRET" in
        *[!0-9a-fA-F]*)
            return 1
            ;;
        [dD][dD]*)
            [ "${#MT_SECRET}" -eq 34 ]
            ;;
        [eE][eE]*)
            [ "${#MT_SECRET}" -ge 34 ] && [ $(( ${#MT_SECRET} % 2 )) -eq 0 ]
            ;;
        *)
            [ "${#MT_SECRET}" -eq 32 ]
            ;;
    esac
}

mt_secret_type() {
    case "$MT_SECRET" in
        [dD][dD]*)
            printf "dd"
            ;;
        [eE][eE]*)
            printf "ee"
            ;;
        *)
            printf "plain"
            ;;
    esac
}

generate_mt_secret() {
    case "${1:-dd}" in
        plain)
            printf "00112233445566778899aabbccddeeff"
            ;;
        ee)
            [ -n "${2:-}" ] || return 1
            printf "ee00112233445566778899aabbccddeeff6578616d706c652e636f6d"
            ;;
        *)
            printf "%s" "$TEST_GENERATED_SECRET"
            ;;
    esac
}

mt_proxy_link() {
    if [ -n "$TEST_MT_PROXY_LINK" ]; then
        printf "%s" "$TEST_MT_PROXY_LINK"
        return 0
    fi
    if [ -n "$MT_LINK_IP" ] && mt_secret_valid 2>/dev/null; then
        printf "tg://proxy?server=%s&port=%s&secret=%s" "$MT_LINK_IP" "$LISTEN_PORT" "$MT_SECRET"
        return 0
    fi
    return 1
}

socks5_proxy_link() {
    if [ -n "$TEST_SOCKS5_PROXY_LINK" ]; then
        printf "%s" "$TEST_SOCKS5_PROXY_LINK"
        return 0
    fi
    [ -n "$MT_LINK_IP" ] || return 1
    printf "tg://socks?server=%s&port=%s" "$MT_LINK_IP" "$LISTEN_PORT"
    if [ -n "$SOCKS_USERNAME" ] && [ -n "$SOCKS_PASSWORD" ]; then
        printf "&user=%s&pass=%s" "$SOCKS_USERNAME" "$SOCKS_PASSWORD"
    fi
}

lan_ip() {
    printf "%s" "$TEST_LAN_IP"
}

selected_update_channel() {
    printf "%s" "$UPDATE_CHANNEL"
}

selected_update_ref() {
    if [ "$UPDATE_CHANNEL" = "preview" ] && [ -n "$PREVIEW_BRANCH" ]; then
        printf "%s" "$PREVIEW_BRANCH"
        return 0
    fi
    if [ -n "$RELEASE_TAG" ]; then
        printf "%s" "$RELEASE_TAG"
        return 0
    fi
    printf "latest"
}

selected_preview_branch_value() {
    [ -n "$PREVIEW_BRANCH" ] || return 1
    printf "%s" "$PREVIEW_BRANCH"
}

validate_upstream_proxy_entry() {
    case "$1" in
        *:*:*)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

upstream_secret_kind() {
    case "$1" in
        [eE][eE]*) printf "ee-faketls" ;;
        [dD][dD]*) printf "dd-intermediate" ;;
        *) printf "plain" ;;
    esac
}

show_telegram_settings_compact() { :; }
show_menu_summary() { :; }
menu_proxy_action_label() {
    [ "$1" = "1" ] && printf "Stop proxy" || printf "Start proxy"
}
menu_autostart_action_label() {
    [ "$1" = "1" ] && printf "Disable autostart" || printf "Enable autostart"
}
confirm_yn() { return 0; }
is_running() {
    [ "${TEST_IS_RUNNING:-0}" = "1" ]
}
autostart_enabled() {
    [ "${TEST_AUTOSTART_ENABLED:-0}" = "1" ]
}
update_binary() { TEST_UPDATE_BINARY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/update_binary_called"; }
stop_proxy() { TEST_STOP_PROXY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/stop_proxy_called"; }
start_proxy() { TEST_START_PROXY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/start_proxy_called"; }
start_proxy_background() { TEST_START_PROXY_BG_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/start_proxy_bg_called"; }
enable_autostart() { TEST_ENABLE_AUTOSTART_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/enable_autostart_called"; }
disable_autostart() { TEST_DISABLE_AUTOSTART_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/disable_autostart_called"; }
advanced_menu() { TEST_ADVANCED_MENU_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/advanced_menu_called"; }
show_status() { TEST_SHOW_STATUS_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/show_status_called"; }
show_telegram_only() { TEST_SHOW_TELEGRAM_ONLY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/show_telegram_only_called"; }
show_quick_only() { TEST_SHOW_QUICK_ONLY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/show_quick_only_called"; }
restart_proxy() { TEST_RESTART_PROXY_CALLED="1"; : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/restart_proxy_called"; }
remove_all() {
    TEST_REMOVE_ALL_CALLED="1"
    : > "${MENU_FIXTURE_TMPDIR:-$BATS_TEST_TMPDIR}/remove_all_called"
    return "${TEST_REMOVE_ALL_STATUS:-0}"
}

. "$MENU_LIB_DIR/menu.sh"
