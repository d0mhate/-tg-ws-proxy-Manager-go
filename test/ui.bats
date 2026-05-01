

#!/usr/bin/env bats

setup() {
    TEST_DIR="$BATS_TEST_TMPDIR"

    export APP_NAME="tg-ws-proxy"
    export COMMAND_MODE="1"
    export VERBOSE="0"

    export C_BOLD=""
    export C_RESET=""
    export C_BLUE=""
    export C_DIM=""
    export C_YELLOW=""
    export C_RED=""
    export C_GREEN=""

    export LISTEN_HOST="127.0.0.1"
    export LISTEN_PORT="1080"
    export PROXY_MODE="socks5"
    export SOCKS_USERNAME=""
    export SOCKS_PASSWORD=""
    export MT_SECRET=""
    export MT_LINK_IP=""
    export DC_IPS=""
    export POOL_SIZE="16"
    export CF_PROXY="0"
    export CF_PROXY_FIRST="0"
    export CF_BALANCE="1"
    export CF_DOMAIN=""
    export CF_BUILTIN_DOMAINS="pclead.co.uk,offshor.co.uk,cakeisalie.co.uk,noskomnadzor.co.uk,lovetrue.co.uk,sorokdva.co.uk,pyatdesyatdva.co.uk,kartoshka.co.uk"

    export BIN_PATH="$TEST_DIR/tg-ws-proxy"
    export SOURCE_BIN="$TEST_DIR/source-bin"
    export INIT_SCRIPT_PATH="$TEST_DIR/tg-ws-proxy.init"
    export DEFAULT_BINARY_NAME="default-bin"

    source "$BATS_TEST_DIRNAME/../lib/ui.sh"
}

# --------------------------
# Stubs used by ui.sh
# --------------------------

lan_ip() {
    printf "%s" "${TEST_LAN_IP:-}"
}

installed_version() {
    [ "${TEST_INSTALLED_VERSION_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_INSTALLED_VERSION"
}

persistent_installed_version() {
    [ "${TEST_PERSISTENT_VERSION_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_PERSISTENT_VERSION"
}

read_latest_version_cache() {
    [ "${TEST_LATEST_VERSION_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_LATEST_VERSION"
}

version_ge() {
    [ "$1" = "$2" ] && return 0
    case "$1:$2" in
        2.*:1.*|3.*:1.*|3.*:2.*) return 0 ;;
    esac
    return 1
}

selected_preview_branch() {
    [ "${TEST_PREVIEW_BRANCH_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_PREVIEW_BRANCH"
}

selected_release_tag() {
    [ "${TEST_RELEASE_TAG_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_RELEASE_TAG"
}

latest_version_cache_is_fresh() {
    return 0
}

refresh_latest_version_cache() {
    return 0
}

mt_secret_valid() {
    [ "${TEST_MT_SECRET_VALID:-0}" = "1" ]
}

mt_proxy_link() {
    [ "${TEST_MT_PROXY_LINK_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_MT_PROXY_LINK"
}

socks5_proxy_link() {
    [ "${TEST_SOCKS5_PROXY_LINK_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_SOCKS5_PROXY_LINK"
}

cf_builtin_domains() {
    printf "%s" "$CF_BUILTIN_DOMAINS"
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

password_display() {
    if [ -n "$SOCKS_PASSWORD" ]; then
        printf "<set>"
    else
        printf "<empty>"
    fi
}

selected_update_channel() {
    [ "${TEST_UPDATE_CHANNEL_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_UPDATE_CHANNEL"
}

selected_update_ref() {
    [ "${TEST_UPDATE_REF_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_UPDATE_REF"
}

current_launcher_path() {
    [ "${TEST_LAUNCHER_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_LAUNCHER"
}

has_persistent_install() {
    [ "${TEST_HAS_PERSISTENT:-0}" = "1" ]
}

persistent_bin_path() {
    printf "%s" "${TEST_PERSISTENT_BIN:-$TEST_DIR/persist/tg-ws-proxy}"
}

persistent_install_dir() {
    printf "%s" "${TEST_PERSISTENT_DIR:-$TEST_DIR/persist}"
}

is_running() {
    [ "${TEST_IS_RUNNING:-0}" = "1" ]
}

current_pids() {
    printf "111\n222\n"
}

autostart_enabled() {
    [ "${TEST_AUTOSTART_ENABLED:-0}" = "1" ]
}

resolved_binary_name() {
    printf "tg-ws-proxy-openwrt-mipsel_24kc"
}

resolved_release_url() {
    printf "https://example.com/release"
}

is_openwrt() {
    [ "${TEST_IS_OPENWRT:-0}" = "1" ]
}

openwrt_arch() {
    [ "${TEST_OPENWRT_ARCH_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_OPENWRT_ARCH"
}

tmp_available_kb() {
    [ "${TEST_TMP_FREE_SET:-0}" = "1" ] || return 1
    printf "%s" "$TEST_TMP_FREE"
}

pause() {
    printf "[pause]\n"
}

# --------------------------
# DC mapping
# --------------------------

@test "show_dc_ip_mapping_settings shows default" {
    DC_IPS=""

    run show_dc_ip_mapping_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"Telegram DC mapping"* ]]
    [[ "$output" == *"custom : <default>"* ]]
}

@test "show_dc_ip_mapping_settings shows custom mapping" {
    DC_IPS="1:149.154.175.50,2:149.154.167.50"

    run show_dc_ip_mapping_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"custom : 1:149.154.175.50,2:149.154.167.50"* ]]
}

# --------------------------
# telegram_host
# --------------------------

@test "telegram_host uses lan ip for 0.0.0.0" {
    LISTEN_HOST="0.0.0.0"
    TEST_LAN_IP="192.168.1.10"

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = "192.168.1.10" ]
}

@test "telegram_host falls back to localhost when lan ip is empty" {
    LISTEN_HOST="0.0.0.0"
    TEST_LAN_IP=""

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = "127.0.0.1" ]
}

@test "telegram_host treats empty listen host as wildcard" {
    LISTEN_HOST=""
    TEST_LAN_IP="10.0.0.5"

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = "10.0.0.5" ]
}

@test "telegram_host normalizes localhost" {
    LISTEN_HOST="localhost"

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = "127.0.0.1" ]
}

@test "telegram_host keeps custom host" {
    LISTEN_HOST="proxy.example.com"

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = "proxy.example.com" ]
}

# --------------------------
# Header helpers
# --------------------------

@test "_header_new_version returns latest when newer exists" {
    TEST_LATEST_VERSION_SET="1"
    TEST_LATEST_VERSION="2.0.0"

    run _header_new_version "1.0.0"

    [ "$status" -eq 0 ]
    [ "$output" = "2.0.0" ]
}

@test "_header_new_version returns non-zero when installed is empty" {
    TEST_LATEST_VERSION_SET="1"
    TEST_LATEST_VERSION="2.0.0"

    run _header_new_version ""

    [ "$status" -ne 0 ]
    [ "$output" = "" ]
}

@test "_header_new_version returns non-zero when latest is missing" {
    TEST_LATEST_VERSION_SET="0"

    run _header_new_version "1.0.0"

    [ "$status" -ne 0 ]
    [ "$output" = "" ]
}

@test "_header_new_version returns non-zero when installed is up to date" {
    TEST_LATEST_VERSION_SET="1"
    TEST_LATEST_VERSION="1.0.0"

    run _header_new_version "1.0.0"

    [ "$status" -ne 0 ]
    [ "$output" = "" ]
}

@test "_header_track_line prefers preview branch" {
    TEST_PREVIEW_BRANCH_SET="1"
    TEST_PREVIEW_BRANCH="dev"
    TEST_RELEASE_TAG_SET="1"
    TEST_RELEASE_TAG="v1.0.0"

    run _header_track_line

    [ "$status" -eq 0 ]
    [ "$output" = "preview: dev" ]
}

@test "_header_track_line shows pinned release tag" {
    TEST_PREVIEW_BRANCH_SET="0"
    TEST_RELEASE_TAG_SET="1"
    TEST_RELEASE_TAG="v1.0.0"

    run _header_track_line

    [ "$status" -eq 0 ]
    [ "$output" = "pinned: v1.0.0" ]
}

@test "_header_track_line returns empty when no track selected" {
    TEST_PREVIEW_BRANCH_SET="0"
    TEST_RELEASE_TAG_SET="0"

    run _header_track_line

    [ "$status" -eq 0 ]
    [ "$output" = "" ]
}

@test "_header_box_line prints long line without negative padding failure" {
    run _header_box_line "  this line is definitely longer than thirty four chars" ""

    [ "$status" -eq 0 ]
    [[ "$output" == *"this line is definitely longer"* ]]
}

# --------------------------
# Header
# --------------------------

@test "show_header uses installed version and shows update notice" {
    TEST_INSTALLED_VERSION_SET="1"
    TEST_INSTALLED_VERSION="1.0.0"
    TEST_LATEST_VERSION_SET="1"
    TEST_LATEST_VERSION="2.0.0"

    run show_header

    [ "$status" -eq 0 ]
    [[ "$output" == *"tg-ws-proxy Go manager"* ]]
    [[ "$output" == *"1.0.0"* ]]
    [[ "$output" == *"new version: 2.0.0"* ]]
}

@test "show_header falls back to persistent version" {
    TEST_INSTALLED_VERSION_SET="0"
    TEST_PERSISTENT_VERSION_SET="1"
    TEST_PERSISTENT_VERSION="0.9.0"

    run show_header

    [ "$status" -eq 0 ]
    [[ "$output" == *"0.9.0"* ]]
}

@test "show_header shows track line" {
    TEST_PREVIEW_BRANCH_SET="1"
    TEST_PREVIEW_BRANCH="dev"

    run show_header

    [ "$status" -eq 0 ]
    [[ "$output" == *"preview: dev"* ]]
}

# --------------------------
# Telegram settings full
# --------------------------

@test "show_telegram_settings socks5 default state" {
    PROXY_MODE="socks5"
    LISTEN_HOST="127.0.0.1"
    LISTEN_PORT="1080"
    SOCKS_USERNAME=""
    SOCKS_PASSWORD=""
    MT_LINK_IP=""
    DC_IPS=""
    CF_PROXY="0"
    CF_PROXY_FIRST="0"
    CF_DOMAIN=""

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"Telegram SOCKS5"* ]]
    [[ "$output" == *"mode     : socks5"* ]]
    [[ "$output" == *"host     : 127.0.0.1"* ]]
    [[ "$output" == *"username : <empty>"* ]]
    [[ "$output" == *"password : <empty>"* ]]
    [[ "$output" == *"link ip  : <not set>"* ]]
    [[ "$output" == *"dc map   : <default>"* ]]
    [[ "$output" == *"pool size: 16"* ]]
    [[ "$output" == *"cf proxy : off"* ]]
    [[ "$output" == *"cf order : fallback"* ]]
    [[ "$output" == *"cf mode  : balance"* ]]
    [[ "$output" == *"cf domain: built-in"* ]]
}

@test "show_telegram_settings socks5 with auth link dc and cf domains count" {
    PROXY_MODE="socks5"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD="pass"
    MT_LINK_IP="8.8.8.8"
    TEST_SOCKS5_PROXY_LINK_SET="1"
    TEST_SOCKS5_PROXY_LINK="tg://socks?server=8.8.8.8"
    DC_IPS="1:1.1.1.1"
    CF_PROXY="1"
    CF_PROXY_FIRST="1"
    CF_DOMAIN="a.example.com,b.example.com,c.example.com"

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"username : user"* ]]
    [[ "$output" == *"password : <set>"* ]]
    [[ "$output" == *"link ip  : 8.8.8.8"* ]]
    [[ "$output" == *"tg link  : tg://socks?server=8.8.8.8"* ]]
    [[ "$output" == *"dc map   : 1:1.1.1.1"* ]]
    [[ "$output" == *"cf proxy : on"* ]]
    [[ "$output" == *"cf order : first"* ]]
    [[ "$output" == *"cf mode  : balance"* ]]
    [[ "$output" == *"cf domain: 3 custom domains"* ]]
}

@test "show_telegram_settings socks5 with single cf domain" {
    PROXY_MODE="socks5"
    CF_DOMAIN="cf.example.com"

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"cf domain: cf.example.com"* ]]
    [[ "$output" == *"cf mode  : balance"* ]]
}

@test "show_telegram_settings mtproto with missing secret" {
    PROXY_MODE="mtproto"
    TEST_MT_SECRET_VALID="0"
    MT_LINK_IP=""

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"Telegram MTProto"* ]]
    [[ "$output" == *"mode     : mtproto"* ]]
    [[ "$output" == *"secret   : <not set>"* ]]
    [[ "$output" == *"link ip  : <not set>"* ]]
}

@test "show_telegram_settings mtproto with valid secret and link" {
    PROXY_MODE="mtproto"
    TEST_MT_SECRET_VALID="1"
    MT_SECRET="abcdef"
    MT_LINK_IP="8.8.4.4"
    TEST_MT_PROXY_LINK_SET="1"
    TEST_MT_PROXY_LINK="tg://proxy?server=8.8.4.4"

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"secret   : abcdef"* ]]
    [[ "$output" == *"link ip  : 8.8.4.4"* ]]
    [[ "$output" == *"tg link  : tg://proxy?server=8.8.4.4"* ]]
}

# --------------------------
# Current version
# --------------------------

@test "show_current_version shows installed version" {
    TEST_INSTALLED_VERSION_SET="1"
    TEST_INSTALLED_VERSION="1.2.3"

    run show_current_version

    [ "$status" -eq 0 ]
    [[ "$output" == *"Binary version"* ]]
    [[ "$output" == *"1.2.3"* ]]
}

@test "show_current_version falls back to persistent version" {
    TEST_INSTALLED_VERSION_SET="0"
    TEST_PERSISTENT_VERSION_SET="1"
    TEST_PERSISTENT_VERSION="0.8.0"

    run show_current_version

    [ "$status" -eq 0 ]
    [[ "$output" == *"0.8.0"* ]]
}

@test "show_current_version shows dash when version is missing" {
    TEST_INSTALLED_VERSION_SET="0"
    TEST_PERSISTENT_VERSION_SET="0"

    run show_current_version

    [ "$status" -eq 0 ]
    [[ "$output" == *"  -"* ]]
}

# --------------------------
# Telegram compact
# --------------------------

@test "show_telegram_settings_compact socks5 no auth" {
    PROXY_MODE="socks5"
    DC_IPS=""
    POOL_SIZE="8"
    CF_PROXY="0"
    CF_PROXY_FIRST="0"
    CF_DOMAIN=""

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"SOCKS5  127.0.0.1:1080  no auth  dc:default  pool:8"* ]]
    [[ "$output" == *"CF      off / fallback / balance / domain:8 built-in"* ]]
}

@test "show_telegram_settings_compact socks5 with username and password" {
    PROXY_MODE="socks5"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD="pass"
    MT_LINK_IP="8.8.8.8"
    TEST_SOCKS5_PROXY_LINK_SET="1"
    TEST_SOCKS5_PROXY_LINK="tg://socks?server=8.8.8.8"
    DC_IPS="1:1.1.1.1"
    CF_PROXY="1"
    CF_PROXY_FIRST="1"
    CF_DOMAIN="a.example.com,b.example.com"

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"SOCKS5  127.0.0.1:1080  user:user/<set>  dc:custom  pool:16"* ]]
    [[ "$output" == *"tg://socks?server=8.8.8.8"* ]]
    [[ "$output" == *"CF      on / first / balance / domain:2 custom"* ]]
}

@test "show_telegram_settings_compact socks5 with username only" {
    PROXY_MODE="socks5"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD=""

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"user:user"* ]]
}

@test "show_telegram_settings_compact mtproto missing secret" {
    PROXY_MODE="mtproto"
    TEST_MT_SECRET_VALID="0"
    MT_LINK_IP=""

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"MTProto"* ]]
    [[ "$output" == *"127.0.0.1:1080"* ]]
    [[ "$output" == *"secret:missing"* ]]
    [[ "$output" == *"ip:none"* ]]
    [[ "$output" == *"dc:default"* ]]
    [[ "$output" == *"pool:16"* ]]
}

@test "show_telegram_settings_compact mtproto valid secret with link" {
    PROXY_MODE="mtproto"
    TEST_MT_SECRET_VALID="1"
    MT_SECRET="abcdef"
    MT_LINK_IP="8.8.4.4"

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"MTProto  127.0.0.1:1080  secret:set  ip:8.8.4.4  dc:default  pool:16"* ]]
    [[ "$output" == *"tg://proxy?server=8.8.4.4&port=1080&secret=abcdef"* ]]
}

@test "show_telegram_settings_compact single cf domain" {
    CF_DOMAIN="cf.example.com"

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"domain:cf.example.com"* ]]
}

# --------------------------
# Update source / track labels
# --------------------------

@test "show_update_source_settings defaults to release latest" {
    run show_update_source_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"Update source"* ]]
    [[ "$output" == *"mode     : release"* ]]
    [[ "$output" == *"ref      : latest"* ]]
}

@test "show_update_source_settings shows selected values" {
    TEST_UPDATE_CHANNEL_SET="1"
    TEST_UPDATE_CHANNEL="preview"
    TEST_UPDATE_REF_SET="1"
    TEST_UPDATE_REF="dev"

    run show_update_source_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"mode     : preview"* ]]
    [[ "$output" == *"ref      : dev"* ]]
}

@test "main_menu_track_label defaults to release latest" {
    run main_menu_track_label

    [ "$status" -eq 0 ]
    [ "$output" = "release/latest" ]
}

@test "main_menu_track_label shows preview ref" {
    TEST_UPDATE_CHANNEL_SET="1"
    TEST_UPDATE_CHANNEL="preview"
    TEST_UPDATE_REF_SET="1"
    TEST_UPDATE_REF="dev"

    run main_menu_track_label

    [ "$status" -eq 0 ]
    [ "$output" = "preview/dev" ]
}

@test "main_menu_track_label shows release ref" {
    TEST_UPDATE_CHANNEL_SET="1"
    TEST_UPDATE_CHANNEL="release"
    TEST_UPDATE_REF_SET="1"
    TEST_UPDATE_REF="v1.2.3"

    run main_menu_track_label

    [ "$status" -eq 0 ]
    [ "$output" = "release/v1.2.3" ]
}

# --------------------------
# Quick commands
# --------------------------

@test "show_quick_commands prints commands without launcher" {
    TEST_LAUNCHER_SET="0"

    run show_quick_commands

    [ "$status" -eq 0 ]
    [[ "$output" == *"Quick commands"* ]]
    [[ "$output" == *"sh "*" install"* ]]
    [[ "$output" == *"sh "*" help"* ]]
}

@test "show_quick_commands prints launcher when available" {
    TEST_LAUNCHER_SET="1"
    TEST_LAUNCHER="/usr/bin/tgm"

    run show_quick_commands

    [ "$status" -eq 0 ]
    [[ "$output" == *"/usr/bin/tgm"* ]]
}

# --------------------------
# Status
# --------------------------

@test "show_status shows missing install stopped and not openwrt" {
    TEST_HAS_PERSISTENT="0"
    TEST_IS_RUNNING="0"
    TEST_AUTOSTART_ENABLED="0"
    TEST_IS_OPENWRT="0"
    TEST_OPENWRT_ARCH_SET="0"
    TEST_TMP_FREE_SET="0"

    run show_status

    [ "$status" -eq 0 ]
    [[ "$output" == *"Status"* ]]
    [[ "$output" == *"tmp bin   : not installed"* ]]
    [[ "$output" == *"persist   : not installed"* ]]
    [[ "$output" == *"process   : stopped"* ]]
    [[ "$output" == *"pid       : -"* ]]
    [[ "$output" == *"autostart : not configured"* ]]
    [[ "$output" == *"launcher  : -"* ]]
    [[ "$output" == *"system    : not detected as OpenWrt"* ]]
    [[ "$output" == *"arch      : -"* ]]
    [[ "$output" == *"tmp free  : - KB"* ]]
}

@test "show_status shows installed running persistent openwrt state" {
    touch "$BIN_PATH"
    chmod +x "$BIN_PATH"
    TEST_HAS_PERSISTENT="1"
    TEST_PERSISTENT_VERSION_SET="1"
    TEST_PERSISTENT_VERSION="1.0.0"
    TEST_IS_RUNNING="1"
    TEST_AUTOSTART_ENABLED="1"
    TEST_LAUNCHER_SET="1"
    TEST_LAUNCHER="/usr/bin/tgm"
    TEST_IS_OPENWRT="1"
    TEST_OPENWRT_ARCH_SET="1"
    TEST_OPENWRT_ARCH="mipsel_24kc"
    TEST_TMP_FREE_SET="1"
    TEST_TMP_FREE="12345"
    VERBOSE="1"

    run show_status

    [ "$status" -eq 0 ]
    [[ "$output" == *"tmp bin   : installed"* ]]
    [[ "$output" == *"persist   : installed"* ]]
    [[ "$output" == *"process   : running"* ]]
    [[ "$output" == *"pid       : 111 222"* ]]
    [[ "$output" == *"bin ver   : 1.0.0"* ]]
    [[ "$output" == *"autostart : enabled"* ]]
    [[ "$output" == *"launcher  : /usr/bin/tgm"* ]]
    [[ "$output" == *"verbose   : on"* ]]
    [[ "$output" == *"system    : OpenWrt"* ]]
    [[ "$output" == *"arch      : mipsel_24kc"* ]]
    [[ "$output" == *"tmp free  : 12345 KB"* ]]
}

@test "show_status shows autostart installed but disabled" {
    touch "$INIT_SCRIPT_PATH"
    TEST_AUTOSTART_ENABLED="0"

    run show_status

    [ "$status" -eq 0 ]
    [[ "$output" == *"autostart : installed but disabled"* ]]
}

# --------------------------
# One-line labels and summaries
# --------------------------

@test "menu_proxy_action_label returns stop for running" {
    run menu_proxy_action_label 1

    [ "$status" -eq 0 ]
    [ "$output" = "Stop proxy" ]
}

@test "menu_proxy_action_label returns start for stopped" {
    run menu_proxy_action_label 0

    [ "$status" -eq 0 ]
    [ "$output" = "Start proxy" ]
}

@test "menu_autostart_action_label returns disable for enabled" {
    run menu_autostart_action_label 1

    [ "$status" -eq 0 ]
    [ "$output" = "Disable autostart" ]
}

@test "menu_autostart_action_label returns enable for disabled" {
    run menu_autostart_action_label 0

    [ "$status" -eq 0 ]
    [ "$output" = "Enable autostart" ]
}

@test "show_menu_summary shows running enabled verbose and track" {
    VERBOSE="1"
    TEST_UPDATE_CHANNEL_SET="1"
    TEST_UPDATE_CHANNEL="preview"
    TEST_UPDATE_REF_SET="1"
    TEST_UPDATE_REF="dev"

    run show_menu_summary 1 1

    [ "$status" -eq 0 ]
    [[ "$output" == *"proxy: running"* ]]
    [[ "$output" == *"autostart: enabled"* ]]
    [[ "$output" == *"verbose: on"* ]]
    [[ "$output" == *"track: preview/dev"* ]]
}

@test "show_menu_summary shows stopped disabled verbose off" {
    VERBOSE="0"

    run show_menu_summary 0 0

    [ "$status" -eq 0 ]
    [[ "$output" == *"proxy: stopped"* ]]
    [[ "$output" == *"autostart: disabled"* ]]
    [[ "$output" == *"verbose: off"* ]]
    [[ "$output" == *"track: release/latest"* ]]
}

# --------------------------
# Pages with pause
# --------------------------

@test "show_telegram_only renders header settings and pauses" {
    run show_telegram_only

    [ "$status" -eq 0 ]
    [[ "$output" == *"tg-ws-proxy Go manager"* ]]
    [[ "$output" == *"Telegram SOCKS5"* ]]
    [[ "$output" == *"Logs are printed directly in the terminal"* ]]
    [[ "$output" == *"[pause]"* ]]
}

@test "show_quick_only renders header commands and pauses" {
    run show_quick_only

    [ "$status" -eq 0 ]
    [[ "$output" == *"tg-ws-proxy Go manager"* ]]
    [[ "$output" == *"Quick commands"* ]]
    [[ "$output" == *"[pause]"* ]]
}

@test "show_help renders usage and pauses" {
    run show_help

    [ "$status" -eq 0 ]
    [[ "$output" == *"Usage"* ]]
    [[ "$output" == *"start menu mode"* ]]
    [[ "$output" == *"install or update binary"* ]]
    [[ "$output" == *"show this help"* ]]
    [[ "$output" == *"[pause]"* ]]
}

# --------------------------
# Extra robustness / edge cases
# --------------------------

@test "show_header prints frame lines" {
    TEST_INSTALLED_VERSION_SET="1"
    TEST_INSTALLED_VERSION="1.0.0"

    run show_header

    [ "$status" -eq 0 ]
    [[ "$output" == *"+----------------------------------+"* ]]
}

@test "show_telegram_settings handles dirty CF_DOMAIN commas" {
    PROXY_MODE="socks5"
    CF_DOMAIN=",,"

    run show_telegram_settings

    [ "$status" -eq 0 ]
    # Should not crash and should still print cf domain line
    [[ "$output" == *"cf domain:"* ]]
}

@test "show_telegram_settings_compact handles long CF_DOMAIN" {
    PROXY_MODE="socks5"
    CF_DOMAIN="$(printf 'a%.0s' {1..200})"

    run show_telegram_settings_compact

    [ "$status" -eq 0 ]
    [[ "$output" == *"domain:"* ]]
}

@test "show_telegram_settings handles very long DC_IPS" {
    PROXY_MODE="socks5"
    DC_IPS="$(printf '1:1.1.1.1,'%.0s {1..50})"

    run show_telegram_settings

    [ "$status" -eq 0 ]
    [[ "$output" == *"dc map"* ]]
}

@test "telegram_host handles unusual host value" {
    LISTEN_HOST=":::"

    run telegram_host

    [ "$status" -eq 0 ]
    [ "$output" = ":::" ]
}
