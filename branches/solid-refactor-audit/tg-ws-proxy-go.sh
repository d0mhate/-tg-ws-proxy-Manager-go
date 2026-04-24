#!/bin/sh

set -u


if [ -t 1 ]; then
    C_RESET="$(printf '\033[0m')"
    C_BOLD="$(printf '\033[1m')"
    C_GREEN="$(printf '\033[1;32m')"
    C_YELLOW="$(printf '\033[1;33m')"
    C_RED="$(printf '\033[1;31m')"
    C_CYAN="$(printf '\033[1;36m')"
    C_BLUE="$(printf '\033[0;34m')"
    C_DIM="$(printf '\033[38;5;244m')"
else
    C_RESET=""
    C_BOLD=""
    C_GREEN=""
    C_YELLOW=""
    C_RED=""
    C_CYAN=""
    C_BLUE=""
    C_DIM=""
fi


APP_NAME="tg-ws-proxy"
LAUNCHER_NAME="${LAUNCHER_NAME:-tgm}"
REPO_OWNER="${REPO_OWNER:-d0mhate}"
REPO_NAME="${REPO_NAME:--tg-ws-proxy-Manager-go}"
DEFAULT_BINARY_NAME="${DEFAULT_BINARY_NAME:-tg-ws-proxy-openwrt}"
BINARY_NAME="${BINARY_NAME:-}"
LISTEN_HOST_FROM_ENV="${LISTEN_HOST+x}"
LISTEN_PORT_FROM_ENV="${LISTEN_PORT+x}"
VERBOSE_FROM_ENV="${VERBOSE+x}"
POOL_SIZE_FROM_ENV="${POOL_SIZE+x}"
SOCKS_USERNAME_FROM_ENV="${SOCKS_USERNAME+x}"
SOCKS_PASSWORD_FROM_ENV="${SOCKS_PASSWORD+x}"
DC_IPS_FROM_ENV="${DC_IPS+x}"
CF_PROXY_FROM_ENV="${CF_PROXY+x}"
CF_PROXY_FIRST_FROM_ENV="${CF_PROXY_FIRST+x}"
CF_DOMAIN_FROM_ENV="${CF_DOMAIN+x}"
PROXY_MODE_FROM_ENV="${PROXY_MODE+x}"
MT_SECRET_FROM_ENV="${MT_SECRET+x}"
MT_LINK_IP_FROM_ENV="${MT_LINK_IP+x}"
MT_UPSTREAM_PROXIES_FROM_ENV="${MT_UPSTREAM_PROXIES+x}"
RUNTIME_BIN_OVERRIDE_FROM_ENV="${RUNTIME_BIN_OVERRIDE+x}"
UPDATE_CHANNEL_FROM_ENV="${UPDATE_CHANNEL+x}"
PREVIEW_BRANCH_FROM_ENV="${PREVIEW_BRANCH+x}"
OPENWRT_RELEASE_FILE="${OPENWRT_RELEASE_FILE:-/etc/openwrt_release}"
RELEASE_DOWNLOAD_BASE_URL="${RELEASE_DOWNLOAD_BASE_URL:-https://github.com/$REPO_OWNER/$REPO_NAME/releases/latest/download}"
RELEASE_URL="${RELEASE_URL:-}"
RELEASE_API_URL="${RELEASE_API_URL:-https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest}"
RELEASES_API_URL="${RELEASES_API_URL:-https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases?per_page=10}"
SCRIPT_RELEASE_BASE_URL="${SCRIPT_RELEASE_BASE_URL:-https://github.com/$REPO_OWNER/$REPO_NAME/releases/download}"
PREVIEW_BRANCH_NAME="${PREVIEW_BRANCH_NAME:-preview}"
PREVIEW_BASE_URL="${PREVIEW_BASE_URL:-https://raw.githubusercontent.com/$REPO_OWNER/$REPO_NAME/$PREVIEW_BRANCH_NAME/branches}"
RELEASE_TAG="${RELEASE_TAG:-}"
MIN_PINNED_RELEASE_TAG="${MIN_PINNED_RELEASE_TAG:-v1.1.29}"
FORCE_ARROW_UPDATE_SOURCE_PICKER="${FORCE_ARROW_UPDATE_SOURCE_PICKER:-}"
FORCE_NUMBERED_UPDATE_SOURCE_PICKER="${FORCE_NUMBERED_UPDATE_SOURCE_PICKER:-}"
SOURCE_BIN="${SOURCE_BIN:-/tmp/tg-ws-proxy-openwrt}"
SOURCE_VERSION_FILE="${SOURCE_VERSION_FILE:-$SOURCE_BIN.version}"
SOURCE_MANAGER_SCRIPT="${SOURCE_MANAGER_SCRIPT:-$SOURCE_BIN.manager}"
INSTALL_DIR="${INSTALL_DIR:-/tmp/tg-ws-proxy-go}"
BIN_PATH="${BIN_PATH:-$INSTALL_DIR/tg-ws-proxy}"
VERSION_FILE="${VERSION_FILE:-$INSTALL_DIR/version}"
PERSIST_STATE_DIR="${PERSIST_STATE_DIR:-/etc/tg-ws-proxy-go}"
PERSIST_PATH_FILE="${PERSIST_PATH_FILE:-$PERSIST_STATE_DIR/install_dir}"
PERSIST_VERSION_FILE="${PERSIST_VERSION_FILE:-$PERSIST_STATE_DIR/version}"
PERSIST_CONFIG_FILE="${PERSIST_CONFIG_FILE:-$PERSIST_STATE_DIR/autostart.conf}"
PERSIST_RELEASE_TAG_FILE="${PERSIST_RELEASE_TAG_FILE:-$PERSIST_STATE_DIR/release_tag}"
PERSIST_UPDATE_CHANNEL_FILE="${PERSIST_UPDATE_CHANNEL_FILE:-$PERSIST_STATE_DIR/update_channel}"
PERSIST_PREVIEW_BRANCH_FILE="${PERSIST_PREVIEW_BRANCH_FILE:-$PERSIST_STATE_DIR/preview_branch}"
LATEST_VERSION_CACHE_FILE="${LATEST_VERSION_CACHE_FILE:-$PERSIST_STATE_DIR/latest_version_cache}"
INIT_SCRIPT_PATH="${INIT_SCRIPT_PATH:-/etc/init.d/tg-ws-proxy-go}"
PERSIST_MANAGER_NAME="${PERSIST_MANAGER_NAME:-tg-ws-proxy-go.sh}"
PERSISTENT_DIR_CANDIDATES="${PERSISTENT_DIR_CANDIDATES:-/root/tg-ws-proxy-go /opt/tg-ws-proxy-go /etc/tg-ws-proxy-go}"
RC_COMMON_PATH="${RC_COMMON_PATH:-/etc/rc.common}"
RC_D_DIR="${RC_D_DIR:-/etc/rc.d}"
PROC_ROOT="${PROC_ROOT:-/proc}"
LAUNCHER_PATH="${LAUNCHER_PATH:-/usr/bin/$LAUNCHER_NAME}"
LISTEN_HOST="${LISTEN_HOST:-0.0.0.0}"
LISTEN_PORT="${LISTEN_PORT:-1080}"
VERBOSE="${VERBOSE:-0}"
POOL_SIZE="${POOL_SIZE:-4}"
SOCKS_USERNAME="${SOCKS_USERNAME:-}"
SOCKS_PASSWORD="${SOCKS_PASSWORD:-}"
DC_IPS="${DC_IPS:-}"
CF_PROXY="${CF_PROXY:-0}"
CF_PROXY_FIRST="${CF_PROXY_FIRST:-0}"
CF_DOMAIN="${CF_DOMAIN:-}"
PROXY_MODE="${PROXY_MODE:-socks5}"
MT_SECRET="${MT_SECRET:-}"
MT_LINK_IP="${MT_LINK_IP:-}"
MT_UPSTREAM_PROXIES="${MT_UPSTREAM_PROXIES:-}"
RUNTIME_BIN_OVERRIDE="${RUNTIME_BIN_OVERRIDE:-}"
UPDATE_CHANNEL="${UPDATE_CHANNEL:-}"
PREVIEW_BRANCH="${PREVIEW_BRANCH:-}"
REQUIRED_TMP_KB="${REQUIRED_TMP_KB:-8192}"
PERSISTENT_SPACE_HEADROOM_KB="${PERSISTENT_SPACE_HEADROOM_KB:-2048}"
PID_FILE="${PID_FILE:-$INSTALL_DIR/pid}"
COMMAND_MODE="0"

if [ "$#" -gt 0 ]; then
    COMMAND_MODE="1"
fi


tmp_available_kb() {
    df -k /tmp 2>/dev/null | awk 'NR==2 {print $4+0}'
}

closest_existing_path() {
    path="$1"

    while [ -n "$path" ] && [ "$path" != "/" ] && [ ! -e "$path" ]; do
        path="$(dirname "$path")"
    done

    if [ -z "$path" ]; then
        printf "/"
        return 0
    fi

    printf "%s" "$path"
}

path_available_kb() {
    path="$(closest_existing_path "$1")"
    df -k "$path" 2>/dev/null | awk 'NR==2 {print $4+0}'
}

read_first_line() {
    file="$1"
    [ -f "$file" ] || return 1
    IFS= read -r line < "$file" || return 1
    [ -n "$line" ] || return 1
    printf "%s" "$line"
}

pause() {
    if [ "$COMMAND_MODE" = "1" ]; then
        return 0
    fi
    printf "\nPress Enter to continue..."
    read dummy
}

canonical_path() {
    path="$1"
    readlink -f "$path" 2>/dev/null || printf "%s" "$path"
}

current_script_path() {
    if [ -n "${0:-}" ]; then
        canonical_path "$0"
        return 0
    fi
    return 1
}

password_display() {
    if [ -n "$SOCKS_PASSWORD" ]; then
        printf "<set>"
    else
        printf "<empty>"
    fi
}


lan_ip() {
    uci get network.lan.ipaddr 2>/dev/null | cut -d/ -f1
}

is_openwrt() {
    [ -f "$OPENWRT_RELEASE_FILE" ] && grep -q "OpenWrt" "$OPENWRT_RELEASE_FILE" 2>/dev/null
}

openwrt_arch() {
    awk -F"'" '/DISTRIB_ARCH/ {print $2}' "$OPENWRT_RELEASE_FILE" 2>/dev/null
}

binary_name_for_arch() {
    arch="$1"
    case "$arch" in
        mipsel_24kc)
            printf "tg-ws-proxy-openwrt-mipsel_24kc"
            ;;
        mips_24kc)
            printf "tg-ws-proxy-openwrt-mips_24kc"
            ;;
        aarch64*)
            printf "tg-ws-proxy-openwrt-aarch64"
            ;;
        x86_64)
            printf "tg-ws-proxy-openwrt-x86_64"
            ;;
        arm_cortex-a7|arm_cortex-a9|arm_cortex-a15_neon-vfpv4)
            printf "tg-ws-proxy-openwrt-armv7"
            ;;
        *)
            printf "%s" "$DEFAULT_BINARY_NAME"
            ;;
    esac
}

generic_binary_name() {
    os="$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m 2>/dev/null)"
    case "$os" in
        linux)
            case "$arch" in
                x86_64)          printf "tg-ws-proxy-openwrt-x86_64" ;;
                aarch64|arm64)   printf "tg-ws-proxy-openwrt-aarch64" ;;
                armv8l)          printf "tg-ws-proxy-openwrt-armv7" ;;
                armv7*)          printf "tg-ws-proxy-openwrt-armv7" ;;
                armv6*)          printf "tg-ws-proxy-linux-armv6" ;;
                i386|i686)       printf "tg-ws-proxy-linux-386" ;;
                riscv64)         printf "tg-ws-proxy-linux-riscv64" ;;
                loongarch64)     printf "tg-ws-proxy-linux-loong64" ;;
                mips64el|mipsel) printf "tg-ws-proxy-openwrt-mipsel_24kc" ;;
                mips64|mips)     printf "tg-ws-proxy-openwrt-mips_24kc" ;;
                *)               printf "%s" "$DEFAULT_BINARY_NAME" ;;
            esac
            ;;
        darwin)
            case "$arch" in
                x86_64)          printf "tg-ws-proxy-darwin-amd64" ;;
                arm64)           printf "tg-ws-proxy-darwin-arm64" ;;
                *)               printf "tg-ws-proxy-darwin-universal" ;;
            esac
            ;;
        freebsd)
            case "$arch" in
                aarch64|arm64)   printf "tg-ws-proxy-freebsd-arm64" ;;
                *)               printf "tg-ws-proxy-freebsd-amd64" ;;
            esac
            ;;
        *)
            printf "%s" "$DEFAULT_BINARY_NAME"
            ;;
    esac
}

is_supported_openwrt_arch() {
    arch="$1"
    case "$arch" in
        mipsel_24kc|mips_24kc|aarch64*|x86_64|arm_cortex-a7|arm_cortex-a9|arm_cortex-a15_neon-vfpv4)
            return 0
            ;;
    esac
    return 1
}

show_environment_checks() {
    if is_openwrt; then
        printf "%sOpenWrt detected%s\n" "$C_GREEN" "$C_RESET"
    else
        printf "%sWarning:%s system does not look like OpenWrt\n" "$C_YELLOW" "$C_RESET"
    fi

    arch="$(openwrt_arch)"
    if [ -n "$arch" ]; then
        if is_supported_openwrt_arch "$arch"; then
            printf "%sArch detected:%s %s\n" "$C_GREEN" "$C_RESET" "$arch"
        else
            printf "%sWarning:%s detected arch is %s and there is no dedicated release asset mapping for it yet\n" "$C_YELLOW" "$C_RESET" "$arch"
        fi
    fi

    printf "Release asset: %s\n" "$(resolved_binary_name)"

    free_kb="$(tmp_available_kb)"
    if [ -n "$free_kb" ]; then
        printf "tmp free: %s KB\n" "$free_kb"
    fi
}


read_config_value() {
    key="$1"
    [ -f "$PERSIST_CONFIG_FILE" ] || return 1
    sed -n "s/^${key}='\(.*\)'$/\1/p" "$PERSIST_CONFIG_FILE" 2>/dev/null | head -n 1
}

normalize_dc_ip_list() {
    value="$(printf "%s" "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    [ -n "$value" ] || return 1

    awk -v input="$value" '
        function trim(s) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
            return s
        }
        BEGIN {
            count = split(input, parts, ",")
            if (count < 1) exit 1
            out = ""
            for (i = 1; i <= count; i++) {
                part = trim(parts[i])
                if (part == "") exit 1
                n = split(part, pair, ":")
                if (n != 2) exit 1
                dc = trim(pair[1])
                ip = trim(pair[2])
                if (dc !~ /^[0-9]+$/) exit 1
                octets = split(ip, ipParts, ".")
                if (octets != 4) exit 1
                for (j = 1; j <= 4; j++) {
                    if (ipParts[j] !~ /^[0-9]+$/) exit 1
                    if (ipParts[j] < 0 || ipParts[j] > 255) exit 1
                }
                if (out != "") out = out ", "
                out = out dc ":" ip
            }
            print out
            exit 0
        }
    '
}

write_settings_config() {
    bin_path="${1:-}"

    if [ -z "$bin_path" ]; then
        bin_path="$(read_config_value BIN 2>/dev/null || true)"
    fi

    auth_settings_valid || return 1

    mkdir -p "$PERSIST_STATE_DIR" || return 1
    {
        if [ -n "$bin_path" ]; then
            printf "BIN='%s'\n" "$bin_path"
        fi
        printf "HOST='%s'\n" "$LISTEN_HOST"
        printf "PORT='%s'\n" "$LISTEN_PORT"
        printf "VERBOSE='%s'\n" "$VERBOSE"
        printf "POOL_SIZE='%s'\n" "$POOL_SIZE"
        printf "USERNAME='%s'\n" "$SOCKS_USERNAME"
        printf "PASSWORD='%s'\n" "$SOCKS_PASSWORD"
        printf "DC_IPS='%s'\n" "$DC_IPS"
        printf "CF_PROXY='%s'\n" "$CF_PROXY"
        printf "CF_PROXY_FIRST='%s'\n" "$CF_PROXY_FIRST"
        printf "CF_DOMAIN='%s'\n" "$CF_DOMAIN"
        printf "PROXY_MODE='%s'\n" "$PROXY_MODE"
        printf "MT_SECRET='%s'\n" "$MT_SECRET"
        printf "MT_LINK_IP='%s'\n" "$MT_LINK_IP"
        printf "MT_UPSTREAM_PROXIES='%s'\n" "$MT_UPSTREAM_PROXIES"
    } > "$PERSIST_CONFIG_FILE" || return 1
}

load_saved_settings() {
    [ -f "$PERSIST_CONFIG_FILE" ] || return 0

    if [ -z "$LISTEN_HOST_FROM_ENV" ]; then
        host="$(read_config_value HOST 2>/dev/null || true)"
        [ -n "$host" ] && LISTEN_HOST="$host"
    fi

    if [ -z "$LISTEN_PORT_FROM_ENV" ]; then
        port="$(read_config_value PORT 2>/dev/null || true)"
        [ -n "$port" ] && LISTEN_PORT="$port"
    fi

    if [ -z "$VERBOSE_FROM_ENV" ]; then
        verbose_value="$(read_config_value VERBOSE 2>/dev/null || true)"
        [ -n "$verbose_value" ] && VERBOSE="$verbose_value"
    fi

    if [ -z "$POOL_SIZE_FROM_ENV" ]; then
        pool_size_value="$(read_config_value POOL_SIZE 2>/dev/null || true)"
        [ -n "$pool_size_value" ] && POOL_SIZE="$pool_size_value"
    fi

    if [ -z "$SOCKS_USERNAME_FROM_ENV" ]; then
        SOCKS_USERNAME="$(read_config_value USERNAME 2>/dev/null || true)"
    fi

    if [ -z "$SOCKS_PASSWORD_FROM_ENV" ]; then
        SOCKS_PASSWORD="$(read_config_value PASSWORD 2>/dev/null || true)"
    fi

    if [ -z "$DC_IPS_FROM_ENV" ]; then
        DC_IPS="$(read_config_value DC_IPS 2>/dev/null || true)"
    fi

    if [ -z "$CF_PROXY_FROM_ENV" ]; then
        cf_proxy_value="$(read_config_value CF_PROXY 2>/dev/null || true)"
        [ -n "$cf_proxy_value" ] && CF_PROXY="$cf_proxy_value"
    fi

    if [ -z "$CF_PROXY_FIRST_FROM_ENV" ]; then
        cf_proxy_first_value="$(read_config_value CF_PROXY_FIRST 2>/dev/null || true)"
        [ -n "$cf_proxy_first_value" ] && CF_PROXY_FIRST="$cf_proxy_first_value"
    fi

    if [ -z "$CF_DOMAIN_FROM_ENV" ]; then
        CF_DOMAIN="$(read_config_value CF_DOMAIN 2>/dev/null || true)"
    fi

    if [ -z "$PROXY_MODE_FROM_ENV" ]; then
        proxy_mode_value="$(read_config_value PROXY_MODE 2>/dev/null || true)"
        [ -n "$proxy_mode_value" ] && PROXY_MODE="$proxy_mode_value"
    fi

    if [ -z "$MT_SECRET_FROM_ENV" ]; then
        MT_SECRET="$(read_config_value MT_SECRET 2>/dev/null || true)"
    fi

    if [ -z "$MT_LINK_IP_FROM_ENV" ]; then
        MT_LINK_IP="$(read_config_value MT_LINK_IP 2>/dev/null || true)"
    fi

    if [ -z "$MT_UPSTREAM_PROXIES_FROM_ENV" ]; then
        MT_UPSTREAM_PROXIES="$(read_config_value MT_UPSTREAM_PROXIES 2>/dev/null || true)"
    fi
}

selected_update_channel() {
    case "$UPDATE_CHANNEL" in
        preview)
            printf "preview"
            return 0
            ;;
        ""|release)
            ;;
        *)
            return 1
            ;;
    esac

    if [ "$UPDATE_CHANNEL_FROM_ENV" = x ]; then
        printf "release"
        return 0
    fi

    value="$(read_first_line "$PERSIST_UPDATE_CHANNEL_FILE" 2>/dev/null || true)"
    case "$value" in
        preview)
            printf "preview"
            return 0
            ;;
    esac

    printf "release"
}

selected_preview_branch() {
    if [ "$(selected_update_channel 2>/dev/null || true)" != "preview" ]; then
        return 1
    fi

    selected_preview_branch_value
}

selected_preview_branch_value() {
    if [ "$PREVIEW_BRANCH_FROM_ENV" = x ]; then
        [ -n "$PREVIEW_BRANCH" ] || return 1
        printf "%s" "$PREVIEW_BRANCH"
        return 0
    fi

    value="$(read_first_line "$PERSIST_PREVIEW_BRANCH_FILE" 2>/dev/null || true)"
    [ -n "$value" ] || return 1
    printf "%s" "$value"
}

selected_update_ref() {
    if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
        selected_preview_branch
        return $?
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s" "$tag"
        return 0
    fi

    printf "latest"
}

write_update_source_state() {
    channel="$1"
    ref="$2"

    case "$channel" in
        preview)
            [ -n "$ref" ] || return 1
            mkdir -p "$PERSIST_STATE_DIR" || return 1
            printf "preview\n" > "$PERSIST_UPDATE_CHANNEL_FILE" || return 1
            printf "%s\n" "$ref" > "$PERSIST_PREVIEW_BRANCH_FILE" || return 1
            rm -f "$PERSIST_RELEASE_TAG_FILE"
            return 0
            ;;
        release)
            mkdir -p "$PERSIST_STATE_DIR" || return 1
            printf "release\n" > "$PERSIST_UPDATE_CHANNEL_FILE" || return 1
            rm -f "$PERSIST_PREVIEW_BRANCH_FILE"
            write_release_tag_state "$ref" || return 1
            return 0
            ;;
    esac

    return 1
}

auth_settings_valid() {
    if [ -n "$SOCKS_USERNAME" ] && [ -n "$SOCKS_PASSWORD" ]; then
        return 0
    fi

    if [ -z "$SOCKS_USERNAME" ] && [ -z "$SOCKS_PASSWORD" ]; then
        return 0
    fi

    return 1
}

show_invalid_auth_settings() {
    printf "%sSOCKS5 auth settings are incomplete%s\n\n" "$C_RED" "$C_RESET"
    printf "SOCKS_USERNAME and SOCKS_PASSWORD must be both set or both empty.\n"
}

mt_secret_valid() {
    [ -n "$MT_SECRET" ] || return 1
    case "$MT_SECRET" in
        *[!0-9a-fA-F]*) return 1 ;;
    esac
    _msv_len=${#MT_SECRET}
    case "$MT_SECRET" in
        [dD][dD]*) [ "$_msv_len" -eq 34 ] ;;
        [eE][eE]*) [ "$_msv_len" -ge 34 ] && [ "$(( _msv_len % 2 ))" -eq 0 ] ;;
        *)         [ "$_msv_len" -eq 32 ] ;;
    esac
}

# Prints the format label of MT_SECRET: plain / dd / ee:hostname
mt_secret_type() {
    case "$MT_SECRET" in
        [dD][dD]*) printf "dd" ;;
        [eE][eE]*)
            _mst_host_hex="$(printf '%s' "$MT_SECRET" | cut -c35-)"
            if [ -n "$_mst_host_hex" ]; then
                _mst_host="$(printf "%b" "$(printf "%s" "$_mst_host_hex" | sed 's/../\\x&/g')" 2>/dev/null || true)"
                if [ -n "$_mst_host" ]; then
                    printf "ee:%s" "$_mst_host"
                    return
                fi
            fi
            printf "ee"
            ;;
        *) printf "plain" ;;
    esac
}

# $1 = format: plain (default) | dd | ee
# $2 = hostname (required for ee)
generate_mt_secret() {
    _gsm_fmt="${1:-plain}"
    _gsm_domain="${2:-}"

    _gsm_hex=""
    if command -v openssl >/dev/null 2>&1; then
        _gsm_hex="$(openssl rand -hex 16 2>/dev/null)" || true
    fi
    if [ -z "$_gsm_hex" ] && [ -r /dev/urandom ]; then
        if command -v hexdump >/dev/null 2>&1; then
            _gsm_hex="$(dd if=/dev/urandom bs=16 count=1 2>/dev/null | hexdump -v -e '1/1 "%02x"')" || true
        elif command -v od >/dev/null 2>&1; then
            _gsm_hex="$(dd if=/dev/urandom bs=16 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n')" || true
        fi
    fi
    [ -n "$_gsm_hex" ] || return 1

    _hex_encode() {
        if command -v hexdump >/dev/null 2>&1; then
            printf '%s' "$1" | hexdump -v -e '1/1 "%02x"'
        else
            printf '%s' "$1" | od -An -tx1 | tr -d ' \n'
        fi
    }

    case "$_gsm_fmt" in
        dd) printf "dd%s" "$_gsm_hex" ;;
        ee)
            [ -n "$_gsm_domain" ] || return 1
            _gsm_dhex="$(_hex_encode "$_gsm_domain")"
            [ -n "$_gsm_dhex" ] || return 1
            printf "ee%s%s" "$_gsm_hex" "$_gsm_dhex"
            ;;
        *) printf "%s" "$_gsm_hex" ;;
    esac
}

# Returns 0 if HOST:PORT:SECRET entry is valid, 1 otherwise.
validate_upstream_proxy_entry() {
    _vup_entry="$1"
    _vup_host="$(printf "%s" "$_vup_entry" | cut -d: -f1)"
    _vup_port="$(printf "%s" "$_vup_entry" | cut -d: -f2)"
    _vup_secret="$(printf "%s" "$_vup_entry" | cut -d: -f3-)"

    [ -n "$_vup_host" ]   || return 1
    [ -n "$_vup_port" ]   || return 1
    [ -n "$_vup_secret" ] || return 1

    case "$_vup_port" in *[!0-9]*) return 1 ;; esac
    [ "$_vup_port" -ge 1 ] && [ "$_vup_port" -le 65535 ] || return 1

    case "$_vup_secret" in *[!0-9a-fA-F]*) return 1 ;; esac
    _vup_slen=${#_vup_secret}
    case "$_vup_secret" in
        [dD][dD]*) [ "$_vup_slen" -eq 34 ]  || return 1 ;;
        [eE][eE]*) [ "$_vup_slen" -ge 34 ]  || return 1 ;;
        *)         [ "$_vup_slen" -eq 32 ]  || return 1 ;;
    esac
    return 0
}

mt_proxy_link() {
    [ -n "$MT_LINK_IP" ] || return 1
    mt_secret_valid 2>/dev/null || return 1
    # Plain secrets get dd-prefix in the link so Telegram uses padded intermediate mode.
    _mpl_secret="$MT_SECRET"
    case "$MT_SECRET" in
        [dD][dD]*|[eE][eE]*) : ;;
        *) _mpl_secret="dd${MT_SECRET}" ;;
    esac
    printf "tg://proxy?server=%s&port=%s&secret=%s" "$MT_LINK_IP" "$LISTEN_PORT" "$_mpl_secret"
}

socks5_proxy_link() {
    [ -n "$MT_LINK_IP" ] || return 1
    if [ -n "$SOCKS_USERNAME" ] && [ -n "$SOCKS_PASSWORD" ]; then
        printf "tg://socks?server=%s&port=%s&user=%s&pass=%s" "$MT_LINK_IP" "$LISTEN_PORT" "$SOCKS_USERNAME" "$SOCKS_PASSWORD"
    else
        printf "tg://socks?server=%s&port=%s" "$MT_LINK_IP" "$LISTEN_PORT"
    fi
}

persistent_install_dir() {
    value="$(read_first_line "$PERSIST_PATH_FILE" 2>/dev/null || true)"
    [ -n "$value" ] || return 1
    printf "%s" "$value"
}

persistent_bin_path() {
    dir="$(persistent_install_dir 2>/dev/null || true)"
    [ -n "$dir" ] || return 1
    printf "%s/tg-ws-proxy" "$dir"
}

persistent_manager_path() {
    dir="$(persistent_install_dir 2>/dev/null || true)"
    [ -n "$dir" ] || return 1
    printf "%s/%s" "$dir" "$PERSIST_MANAGER_NAME"
}

has_persistent_install() {
    bin="$(persistent_bin_path 2>/dev/null || true)"
    [ -n "$bin" ] || return 1
    [ -x "$bin" ]
}

write_persistent_state() {
    install_dir="$1"
    version="$2"

    mkdir -p "$PERSIST_STATE_DIR" || return 1
    printf "%s\n" "$install_dir" > "$PERSIST_PATH_FILE" || return 1
    if [ -n "$version" ]; then
        printf "%s\n" "$version" > "$PERSIST_VERSION_FILE" || return 1
    else
        rm -f "$PERSIST_VERSION_FILE"
    fi
}

write_autostart_config() {
    bin_path="$1"
    write_settings_config "$bin_path"
}

sync_autostart_config_if_enabled() {
    if ! autostart_enabled; then
        return 0
    fi

    bin_path="$(persistent_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        return 0
    fi

    write_autostart_config "$bin_path"
}


resolved_binary_name() {
    if [ -n "$BINARY_NAME" ]; then
        printf "%s" "$BINARY_NAME"
        return 0
    fi

    if is_openwrt; then
        arch="$(openwrt_arch)"
        if [ -n "$arch" ]; then
            binary_name_for_arch "$arch"
            return 0
        fi
    fi

    generic_binary_name
}

resolved_release_url() {
    if [ -n "$RELEASE_URL" ]; then
        printf "%s" "$RELEASE_URL"
        return 0
    fi

    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s/%s/%s" "$PREVIEW_BASE_URL" "$preview_branch" "$(resolved_binary_name)"
        return 0
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s/%s/%s" "$SCRIPT_RELEASE_BASE_URL" "$tag" "$(resolved_binary_name)"
        return 0
    fi

    printf "%s/%s" "$RELEASE_DOWNLOAD_BASE_URL" "$(resolved_binary_name)"
}

source_binary_size_kb() {
    if [ ! -f "$SOURCE_BIN" ]; then
        return 1
    fi
    bytes="$(wc -c < "$SOURCE_BIN" 2>/dev/null | tr -d ' ')"
    [ -n "$bytes" ] || return 1
    printf "%s" $(( (bytes + 1023) / 1024 ))
}

required_persistent_kb() {
    size_kb="$(source_binary_size_kb 2>/dev/null || true)"
    if [ -z "$size_kb" ]; then
        printf "%s" "$REQUIRED_TMP_KB"
        return 0
    fi

    need_kb=$((size_kb + PERSISTENT_SPACE_HEADROOM_KB))
    if [ "$need_kb" -lt "$REQUIRED_TMP_KB" ]; then
        need_kb="$REQUIRED_TMP_KB"
    fi
    printf "%s" "$need_kb"
}

normalize_version() {
    value="$1"
    case "$value" in
        v[0-9]*)
            printf "%s" "$value"
            return 0
            ;;
    esac
    return 1
}

version_ge() {
    left="$(normalize_version "$1" 2>/dev/null || true)"
    right="$(normalize_version "$2" 2>/dev/null || true)"
    [ -n "$left" ] || return 1
    [ -n "$right" ] || return 1

    awk -v left="${left#v}" -v right="${right#v}" '
        BEGIN {
            split(left, l, ".")
            split(right, r, ".")
            max = length(l) > length(r) ? length(l) : length(r)
            for (i = 1; i <= max; i++) {
                lv = (i in l) ? l[i] + 0 : 0
                rv = (i in r) ? r[i] + 0 : 0
                if (lv > rv) exit 0
                if (lv < rv) exit 1
            }
            exit 0
        }
    '
}

release_tag_meets_minimum() {
    version_ge "$1" "$MIN_PINNED_RELEASE_TAG"
}

installed_version() {
    value="$(read_first_line "$VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

cached_source_version() {
    value="$(read_first_line "$SOURCE_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

persistent_release_tag() {
    value="$(read_first_line "$PERSIST_RELEASE_TAG_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

release_tag_requests_latest() {
    case "$RELEASE_TAG" in
        latest|default|none)
            return 0
            ;;
    esac
    return 1
}

selected_release_tag() {
    if release_tag_requests_latest; then
        return 1
    fi

    value="$(normalize_version "$RELEASE_TAG" 2>/dev/null || true)"
    if [ -n "$value" ]; then
        printf "%s" "$value"
        return 0
    fi

    persistent_release_tag
}

resolved_release_ref() {
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s" "$preview_branch"
        return 0
    fi

    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "%s" "$tag"
        return 0
    fi

    latest_release_tag
}

write_release_tag_state() {
    value="$(normalize_version "$1" 2>/dev/null || true)"
    if [ -z "$value" ]; then
        rm -f "$PERSIST_RELEASE_TAG_FILE"
        return 0
    fi

    mkdir -p "$(dirname "$PERSIST_RELEASE_TAG_FILE")" || return 1
    printf "%s\n" "$value" > "$PERSIST_RELEASE_TAG_FILE" || return 1
}

persistent_installed_version() {
    value="$(read_first_line "$PERSIST_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

latest_release_tag() {
    _lrt_tag=""
    case "$RELEASE_API_URL" in
        file://*)
            local_path="${RELEASE_API_URL#file://}"
            _lrt_tag="$(tr -d '\n' < "$local_path" 2>/dev/null | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            ;;
        *)
            if command -v wget >/dev/null 2>&1; then
                _lrt_tag="$(wget -qO - "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            elif command -v curl >/dev/null 2>&1; then
                _lrt_tag="$(curl -fsSL "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | sed -n '1p')"
            else
                return 1
            fi
            ;;
    esac
    if [ -z "$_lrt_tag" ]; then
        return 1
    fi
    printf "%s" "$_lrt_tag"
}

recent_release_tags() {
    max_items="${1:-10}"

    case "$RELEASES_API_URL" in
        file://*)
            local_path="${RELEASES_API_URL#file://}"
            tr -d '\n' < "$local_path" 2>/dev/null | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
                normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
                [ -n "$normalized_tag" ] || continue
                release_tag_meets_minimum "$normalized_tag" || continue
                printf "%s\n" "$normalized_tag"
            done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
            return 0
            ;;
    esac

    if command -v wget >/dev/null 2>&1; then
        wget -qO - "$RELEASES_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
            normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
            [ -n "$normalized_tag" ] || continue
            release_tag_meets_minimum "$normalized_tag" || continue
            printf "%s\n" "$normalized_tag"
        done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$RELEASES_API_URL" 2>/dev/null | tr -d '\n' | sed 's/\"tag_name\"/\
\"tag_name\"/g' | sed -n 's/.*\"tag_name\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | while IFS= read -r raw_tag; do
            normalized_tag="$(normalize_version "$raw_tag" 2>/dev/null || true)"
            [ -n "$normalized_tag" ] || continue
            release_tag_meets_minimum "$normalized_tag" || continue
            printf "%s\n" "$normalized_tag"
        done | awk '!seen[$0]++' | sed -n "1,${max_items}p"
        return 0
    fi

    return 1
}

release_url_reachable() {
    url="$(resolved_release_url)"
    if command -v wget >/dev/null 2>&1; then
        wget --spider "$url" >/dev/null 2>&1
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -I -L --fail "$url" >/dev/null 2>&1
        return $?
    fi

    return 1
}

script_release_url() {
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview_branch" ]; then
        printf "%s/%s/%s" "$PREVIEW_BASE_URL" "$preview_branch" "$PERSIST_MANAGER_NAME"
        return 0
    fi

    ref="$1"
    printf "%s/%s/%s" "$SCRIPT_RELEASE_BASE_URL" "$ref" "$PERSIST_MANAGER_NAME"
}

download_binary() {
    mkdir -p "$(dirname "$SOURCE_BIN")" || return 1
    url="$(resolved_release_url)"

    if command -v wget >/dev/null 2>&1; then
        wget -O "$SOURCE_BIN" "$url"
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$SOURCE_BIN" "$url"
        return $?
    fi

    return 1
}

download_manager_script() {
    ref="$1"
    url="$(script_release_url "$ref")"

    mkdir -p "$(dirname "$SOURCE_MANAGER_SCRIPT")" || return 1

    if command -v wget >/dev/null 2>&1; then
        wget -O "$SOURCE_MANAGER_SCRIPT" "$url" >/dev/null 2>&1 || return 1
        chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$SOURCE_MANAGER_SCRIPT" "$url" >/dev/null 2>&1 || return 1
        chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
        return 0
    fi

    return 1
}

write_source_version_file() {
    version="$1"
    [ -n "$version" ] || return 0
    printf "%s\n" "$version" > "$SOURCE_VERSION_FILE" || return 1
}

read_latest_version_cache() {
    value="$(read_first_line "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

latest_version_cache_is_fresh() {
    [ -f "$LATEST_VERSION_CACHE_FILE" ] || return 1
    ts="$(sed -n '2p' "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true)"
    [ -n "$ts" ] || return 1
    now="$(date +%s 2>/dev/null || printf "0")"
    age=$((now - ts))
    [ "$age" -lt 3600 ]
}

refresh_latest_version_cache() {
    tag="$(latest_release_tag 2>/dev/null || true)"
    [ -n "$tag" ] || return 0
    mkdir -p "$(dirname "$LATEST_VERSION_CACHE_FILE")" 2>/dev/null || return 0
    ts="$(date +%s 2>/dev/null || printf "0")"
    printf "%s\n%s\n" "$tag" "$ts" > "$LATEST_VERSION_CACHE_FILE" 2>/dev/null || true
}


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
        printf '%s\n' '    CF_DOMAIN="${CF_DOMAIN:-}"'
        printf '%s\n' '    PROXY_MODE="${PROXY_MODE:-socks5}"'
        printf '%s\n' '    MT_SECRET="${MT_SECRET:-}"'
        printf '%s\n' '    MT_LINK_IP="${MT_LINK_IP:-}"'
        printf '%s\n' '    MT_UPSTREAM_PROXIES="${MT_UPSTREAM_PROXIES:-}"'
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
        printf '%s\n' '    if [ "$CF_PROXY" = "1" ] && [ -n "$CF_DOMAIN" ]; then'
        printf '%s\n' '        set -- "$@" --cf-proxy --cf-domain "$CF_DOMAIN"'
        printf '%s\n' '        if [ "$CF_PROXY_FIRST" = "1" ]; then'
        printf '%s\n' '            set -- "$@" --cf-proxy-first'
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
        if ! check_tmp_space; then
            free_kb="$(tmp_available_kb)"
            printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
            printf "Required: %s KB\n" "$REQUIRED_TMP_KB"
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


select_persistent_dir() {
    required_kb="$1"

    for dir in $PERSISTENT_DIR_CANDIDATES; do
        free_kb="$(path_available_kb "$dir")"
        [ -n "$free_kb" ] || continue
        if [ "$free_kb" -ge "$required_kb" ]; then
            printf "%s" "$dir"
            return 0
        fi
    done

    return 1
}

current_launcher_path() {
    if [ -f "$LAUNCHER_PATH" ]; then
        printf "%s" "$LAUNCHER_PATH"
        return 0
    fi

    if [ -f "/tmp/$LAUNCHER_NAME" ]; then
        printf "%s" "/tmp/$LAUNCHER_NAME"
        return 0
    fi

    return 1
}

install_launcher() {
    script_target="$1"
    target="$LAUNCHER_PATH"

    if ! mkdir -p "$(dirname "$target")" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
    fi

    if ! {
        printf '#!/bin/sh\n'
        printf 'sh %s "$@"\n' "$script_target"
    } > "$target" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
        {
            printf '#!/bin/sh\n'
            printf 'sh %s "$@"\n' "$script_target"
        } > "$target" || return 1
    fi

    chmod +x "$target" || return 1
    printf "%s" "$target"
}

copy_manager_bundle() {
    src_script="$1"
    dest_script="$2"
    fallback_script="${3:-}"

    mkdir -p "$(dirname "$dest_script")" || return 1
    cp "$src_script" "$dest_script" || return 1
    chmod +x "$dest_script" || return 1

    src_lib_dir="$(dirname "$src_script")/lib"
    if [ ! -d "$src_lib_dir" ] && [ -n "$fallback_script" ]; then
        fallback_lib_dir="$(dirname "$fallback_script")/lib"
        if [ -d "$fallback_lib_dir" ]; then
            src_lib_dir="$fallback_lib_dir"
        fi
    fi

    if [ -d "$src_lib_dir" ]; then
        dest_lib_dir="$(dirname "$dest_script")/lib"
        rm -rf "$dest_lib_dir"
        cp -R "$src_lib_dir" "$dest_lib_dir" || return 1
    fi
}

copy_current_manager_script() {
    current_script="$(current_script_path 2>/dev/null || true)"
    [ -n "$current_script" ] || current_script="$0"
    copy_manager_bundle "$current_script" "$SOURCE_MANAGER_SCRIPT"
}

refresh_current_manager_script_from_source() {
    current_script="$(current_script_path 2>/dev/null || true)"
    [ -n "$current_script" ] || return 0
    [ -f "$SOURCE_MANAGER_SCRIPT" ] || return 0

    source_script="$(canonical_path "$SOURCE_MANAGER_SCRIPT")"
    [ "$current_script" = "$source_script" ] && return 0

    tmp_manager="$(canonical_path "$INSTALL_DIR/$PERSIST_MANAGER_NAME")"
    [ "$current_script" = "$tmp_manager" ] && return 0

    [ -w "$current_script" ] || return 0

    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$current_script" "$current_script"
}

ensure_source_manager_current() {
    ref="$1"
    strict="${2:-0}"

    if [ -n "$ref" ] && download_manager_script "$ref"; then
        return 0
    fi

    if [ "$strict" = "1" ] && [ -n "$ref" ]; then
        return 1
    fi

    if [ -x "$SOURCE_MANAGER_SCRIPT" ]; then
        return 0
    fi

    copy_current_manager_script
}

install_from_source() {
    mkdir -p "$INSTALL_DIR" || return 1
    cp "$SOURCE_BIN" "$BIN_PATH" || return 1
    chmod +x "$BIN_PATH" || return 1
    current_script="$(current_script_path 2>/dev/null || true)"
    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$INSTALL_DIR/$PERSIST_MANAGER_NAME" "$current_script" || return 1

    version="$(cached_source_version 2>/dev/null || true)"
    if [ -n "$version" ]; then
        printf "%s\n" "$version" > "$VERSION_FILE" || return 1
    else
        rm -f "$VERSION_FILE"
    fi

    if has_persistent_install; then
        launcher_path="$(current_launcher_path 2>/dev/null || true)"
    else
        launcher_path="$(install_launcher "$INSTALL_DIR/$PERSIST_MANAGER_NAME")" || return 1
    fi
    printf "%s" "$launcher_path"
}

install_persistent_from_source() {
    install_dir="$1"

    mkdir -p "$install_dir" || return 1
    cp "$SOURCE_BIN" "$install_dir/tg-ws-proxy" || return 1
    chmod +x "$install_dir/tg-ws-proxy" || return 1
    current_script="$(current_script_path 2>/dev/null || true)"
    copy_manager_bundle "$SOURCE_MANAGER_SCRIPT" "$install_dir/$PERSIST_MANAGER_NAME" "$current_script" || return 1

    version="$(cached_source_version 2>/dev/null || true)"
    write_persistent_state "$install_dir" "$version" || return 1
    launcher_path="$(install_launcher "$install_dir/$PERSIST_MANAGER_NAME")" || return 1
    printf "%s" "$launcher_path"
}

ensure_source_binary_current() {
    desired_ref="$(resolved_release_ref 2>/dev/null || true)"
    preview_branch="$(selected_preview_branch 2>/dev/null || true)"

    if [ -n "$desired_ref" ]; then
        cached_tag="$(cached_source_version 2>/dev/null || true)"
        need_download="0"

        if [ ! -f "$SOURCE_BIN" ]; then
            printf "%sLocal binary not found%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ -n "$preview_branch" ]; then
            need_download="1"
        elif [ -z "$cached_tag" ]; then
            printf "%sLocal binary version is unknown%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ "$cached_tag" != "$desired_ref" ]; then
            printf "%sLocal binary is outdated%s\n\n" "$C_YELLOW" "$C_RESET"
            printf "Cached version: %s\n" "$cached_tag"
            if [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
                printf "Selected version: %s\n\n" "$desired_ref"
            else
                printf "Latest version: %s\n\n" "$desired_ref"
            fi
            need_download="1"
        fi

        if [ "$need_download" = "1" ]; then
            release_url="$(resolved_release_url)"
            printf "Trying to download from GitHub Release\n"
            printf "%s\n\n" "$release_url"
            if ! release_url_reachable; then
                if [ -f "$SOURCE_BIN" ]; then
                    printf "%sRelease URL is not reachable%s\n\n" "$C_YELLOW" "$C_RESET"
                    printf "Using local cached binary\n"
                else
                    printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
                    printf "Check GitHub Release visibility or network access\n"
                    return 1
                fi
            else
                rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
                if ! download_binary; then
                    printf "%sDownload failed%s\n\n" "$C_RED" "$C_RESET"
                    printf "You can also place the binary here manually\n"
                    printf "  %s\n" "$SOURCE_BIN"
                    return 1
                fi
                write_source_version_file "$desired_ref" || return 1
            fi
        fi
    elif [ ! -f "$SOURCE_BIN" ]; then
        printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
        printf "GitHub API rate limit may be exceeded (60 req/hour per IP)\n"
        printf "Use Advanced > Configure update source to set version manually\n"
        printf "Or check GitHub API access or network access\n"
        return 1
    fi
}

check_tmp_space() {
    free_kb="$(tmp_available_kb)"
    [ -n "$free_kb" ] || return 0
    [ "$free_kb" -ge "$REQUIRED_TMP_KB" ]
}

install_binary() {
    show_header
    show_environment_checks
    printf "\n"

    if ! check_tmp_space; then
        free_kb="$(tmp_available_kb)"
        printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
        printf "Required: %s KB\n" "$REQUIRED_TMP_KB"
        printf "Available: %s KB\n" "${free_kb:-unknown}"
        pause
        return 1
    fi

    if ! ensure_source_binary_current; then
        pause
        return 1
    fi
    release_ref="$(resolved_release_ref 2>/dev/null || true)"
    if ! ensure_source_manager_current "$release_ref"; then
        pause
        return 1
    fi

    launcher_path="$(install_from_source)" || return 1
    write_update_source_state "$(selected_update_channel 2>/dev/null || printf release)" "$(selected_update_ref 2>/dev/null || printf latest)" || return 1

    show_header
    printf "%sBinary installed%s\n\n" "$C_GREEN" "$C_RESET"
    printf "Source:\n  %s\n\n" "$SOURCE_BIN"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    version="$(installed_version 2>/dev/null || true)"
    if [ -n "$version" ]; then
        printf "\nVersion:\n  %s\n" "$version"
    fi
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    pause
}

install_persistent_binary() {
    if ! ensure_source_manager_current "$(resolved_release_ref 2>/dev/null || true)"; then
        return 1
    fi

    need_kb="$(required_persistent_kb)"
    target_dir="$(select_persistent_dir "$need_kb" 2>/dev/null || true)"
    if [ -z "$target_dir" ]; then
        return 1
    fi

    install_persistent_from_source "$target_dir"
}

show_persistent_install_failure() {
    need_kb="$(required_persistent_kb)"
    printf "%sNo suitable persistent path found%s\n\n" "$C_RED" "$C_RESET"
    printf "Need about: %s KB in persistent storage\n" "$need_kb"
    for candidate in $PERSISTENT_DIR_CANDIDATES; do
        printf "  %s : %s KB free\n" "$candidate" "$(path_available_kb "$candidate" 2>/dev/null || printf unknown)"
    done
}

update_binary() {
    show_header
    show_environment_checks
    printf "\n"

    if ! check_tmp_space; then
        free_kb="$(tmp_available_kb)"
        printf "%sNot enough free space in /tmp%s\n\n" "$C_RED" "$C_RESET"
        printf "Required: %s KB\n" "$REQUIRED_TMP_KB"
        printf "Available: %s KB\n" "${free_kb:-unknown}"
        pause
        return 1
    fi

    release_ref="$(resolved_release_ref 2>/dev/null || true)"
    if [ -z "$release_ref" ]; then
        if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
            printf "%sPreview branch is not configured%s\n\n" "$C_RED" "$C_RESET"
            printf "Set a preview branch first in Advanced -> Configure update source\n"
        else
            printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
            printf "GitHub API rate limit may be exceeded (60 req/hour per IP)\n"
            printf "Use Advanced > Configure update source to set version manually\n"
            printf "Or check GitHub API access or network access\n"
        fi
        pause
        return 1
    fi

    current_tag="$(installed_version 2>/dev/null || true)"
    if [ -z "$current_tag" ]; then
        current_tag="$(persistent_installed_version 2>/dev/null || true)"
    fi
    if [ "$(selected_update_channel 2>/dev/null || true)" != "preview" ] && [ -n "$current_tag" ] && [ "$current_tag" = "$release_ref" ] && [ -x "$BIN_PATH" ] && ! has_persistent_install; then
        if [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
            printf "%sAlready on the selected version%s\n\n" "$C_GREEN" "$C_RESET"
        else
            printf "%sAlready on the latest version%s\n\n" "$C_GREEN" "$C_RESET"
        fi
        printf "Current version: %s\n" "$current_tag"
        pause
        return 0
    fi

    printf "Current version: %s\n" "${current_tag:-unknown}"
    if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
        printf "Preview branch: %s\n\n" "$release_ref"
    elif [ -n "$(selected_release_tag 2>/dev/null || true)" ]; then
        printf "Selected version: %s\n\n" "$release_ref"
    else
        printf "Latest version: %s\n\n" "$release_ref"
    fi

    if ! release_url_reachable; then
        printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
        if [ "$(selected_update_channel 2>/dev/null || true)" = "preview" ]; then
            printf "Check preview branch artifacts or network access\n"
        else
            printf "Check GitHub Release visibility or network access\n"
        fi
        pause
        return 1
    fi

    rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
    if ! download_binary; then
        printf "%sDownload failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    write_source_version_file "$release_ref" || return 1
    if ! ensure_source_manager_current "$release_ref" "1"; then
        printf "%sManager script update failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    refresh_current_manager_script_from_source || true
    launcher_path="$(install_from_source)" || return 1
    write_update_source_state "$(selected_update_channel 2>/dev/null || printf release)" "$(selected_update_ref 2>/dev/null || printf latest)" || return 1
    if has_persistent_install; then
        persist_dir="$(persistent_install_dir 2>/dev/null || true)"
        if [ -n "$persist_dir" ]; then
            launcher_path="$(install_persistent_from_source "$persist_dir")" || return 1
        fi
    fi

    show_header
    printf "%sUpdated to %s%s\n\n" "$C_GREEN" "$release_ref" "$C_RESET"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    if [ "$COMMAND_MODE" = "0" ]; then
        printf "\n%sRestarting menu...%s\n" "$C_GREEN" "$C_RESET"
        sleep 1
        exec "$0" 2>/dev/null || {
            printf "%sPlease restart the menu manually.%s\n" "$C_YELLOW" "$C_RESET"
            pause
        }
    else
        pause
    fi
}

remove_all() {
    stop_running >/dev/null 2>&1 || true
    if [ -f "$INIT_SCRIPT_PATH" ]; then
        "$INIT_SCRIPT_PATH" disable >/dev/null 2>&1 || true
        "$INIT_SCRIPT_PATH" stop >/dev/null 2>&1 || true
    fi

    persist_dir="$(persistent_install_dir 2>/dev/null || true)"
    rm -rf "$INSTALL_DIR"
    if [ -n "$persist_dir" ]; then
        rm -rf "$persist_dir"
    fi
    rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
    rm -f "$SOURCE_MANAGER_SCRIPT"
    rm -f "$PID_FILE"
    rm -rf "$PERSIST_STATE_DIR"
    rm -f "$INIT_SCRIPT_PATH"
    rm -f "$LAUNCHER_PATH" "/tmp/$LAUNCHER_NAME"

    show_header
    printf "%sBinary launcher autostart and downloaded files removed%s\n" "$C_GREEN" "$C_RESET"
    pause
}


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

_header_new_version() {
    installed="$1"
    [ -n "$installed" ] || return 1
    latest="$(read_latest_version_cache 2>/dev/null || true)"
    [ -n "$latest" ] || return 1
    version_ge "$installed" "$latest" && return 1
    printf "%s" "$latest"
}

_header_track_line() {
    preview="$(selected_preview_branch 2>/dev/null || true)"
    if [ -n "$preview" ]; then
        printf "preview: %s" "$preview"
        return 0
    fi
    tag="$(selected_release_tag 2>/dev/null || true)"
    if [ -n "$tag" ]; then
        printf "pinned: %s" "$tag"
        return 0
    fi
}

_header_box_line() {
    inner="$1"
    inner_len=${#inner}
    rpad=$((34 - inner_len))
    [ "$rpad" -lt 0 ] && rpad=0
    printf "%s|%s%s%s%s%${rpad}s%s|%s\n" \
        "$C_BLUE" "$C_RESET" "$2" "$inner" "$C_RESET" "" "$C_BLUE" "$C_RESET"
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
    track="$(_header_track_line 2>/dev/null || true)"
    if [ -n "$track" ]; then
        _header_box_line "  $track" "$C_DIM"
    fi
    new_version="$(_header_new_version "$version" 2>/dev/null || true)"
    if [ -n "$new_version" ]; then
        _header_box_line "  * new version: $new_version" "$C_YELLOW"
    fi
    printf "%s+----------------------------------+%s\n\n" "$C_BLUE" "$C_RESET"
    if [ "$COMMAND_MODE" = "0" ] && ! latest_version_cache_is_fresh 2>/dev/null; then
        { refresh_latest_version_cache >/dev/null 2>&1; } &
    fi
}

show_telegram_settings() {
    if [ "$PROXY_MODE" = "mtproto" ]; then
        printf "%sTelegram MTProto%s\n" "$C_BOLD" "$C_RESET"
        printf "  mode     : mtproto\n"
        printf "  host     : %s\n" "$(telegram_host)"
        printf "  port     : %s\n" "$LISTEN_PORT"
        if mt_secret_valid 2>/dev/null; then
            printf "  secret   : %s\n" "$MT_SECRET"
        else
            printf "  secret   : %s<not set>%s\n" "$C_RED" "$C_RESET"
        fi
        if [ -n "$MT_LINK_IP" ]; then
            printf "  link ip  : %s\n" "$MT_LINK_IP"
            link="$(mt_proxy_link 2>/dev/null || true)"
            if [ -n "$link" ]; then
                printf "  tg link  : %s\n" "$link"
            fi
        else
            printf "  link ip  : <not set>\n"
        fi
    else
        printf "%sTelegram SOCKS5%s\n" "$C_BOLD" "$C_RESET"
        printf "  mode     : socks5\n"
        printf "  host     : %s\n" "$(telegram_host)"
        printf "  port     : %s\n" "$LISTEN_PORT"
        if [ -n "$SOCKS_USERNAME" ]; then
            printf "  username : %s\n" "$SOCKS_USERNAME"
        else
            printf "  username : <empty>\n"
        fi
        printf "  password : %s\n" "$(password_display)"
        if [ -n "$MT_LINK_IP" ]; then
            printf "  link ip  : %s\n" "$MT_LINK_IP"
            link="$(socks5_proxy_link 2>/dev/null || true)"
            if [ -n "$link" ]; then
                printf "  tg link  : %s\n" "$link"
            fi
        else
            printf "  link ip  : <not set>\n"
        fi
    fi
    if [ -n "$DC_IPS" ]; then
        printf "  dc map   : %s\n" "$DC_IPS"
    else
        printf "  dc map   : <default>\n"
    fi
    printf "  pool size: %s\n" "$POOL_SIZE"
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
    if [ -n "$DC_IPS" ]; then
        dc_part="dc:custom"
    else
        dc_part="dc:default"
    fi
    pool_part="pool:$POOL_SIZE"

    if [ "$PROXY_MODE" = "mtproto" ]; then
        if mt_secret_valid 2>/dev/null; then
            secret_part="secret:set"
        else
            secret_part="${C_RED}secret:missing${C_RESET}"
        fi
        if [ -n "$MT_LINK_IP" ]; then
            ip_part="ip:$MT_LINK_IP"
        else
            ip_part="${C_DIM}ip:none${C_RESET}"
        fi
        printf "  MTProto %s:%s  %s  %s  %s  %s\n" "$host" "$LISTEN_PORT" "$secret_part" "$ip_part" "$dc_part" "$pool_part"
        if [ -n "$MT_LINK_IP" ] && mt_secret_valid 2>/dev/null; then
            printf "  tg://proxy?server=%s&port=%s&secret=%s\n" "$MT_LINK_IP" "$LISTEN_PORT" "$MT_SECRET"
        fi
    else
        if [ -n "$SOCKS_USERNAME" ]; then
            if [ -n "$SOCKS_PASSWORD" ]; then
                auth_part="user:$SOCKS_USERNAME/<set>"
            else
                auth_part="user:$SOCKS_USERNAME"
            fi
        else
            auth_part="no auth"
        fi
        printf "  SOCKS5  %s:%s  %s  %s  %s\n" "$host" "$LISTEN_PORT" "$auth_part" "$dc_part" "$pool_part"
        if [ -n "$MT_LINK_IP" ]; then
            link="$(socks5_proxy_link 2>/dev/null || true)"
            if [ -n "$link" ]; then
                printf "  %s\n" "$link"
            fi
        fi
    fi

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
    _cf_interrupted=0
    trap '_cf_interrupted=1' INT
    for prefix in kws1 kws2 kws3 kws4 kws5 kws203; do
        [ "$_cf_interrupted" = "0" ] || break
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
    trap - INT

    if [ "$_cf_interrupted" = "1" ]; then
        printf "\nCancelled.\n"
        pause
        return 0
    fi

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
                printf "  %d. %s\n" "$_up_count" "$_up_e"
            done
            IFS="$_up_old_ifs"
        fi
        if [ "$_up_count" -eq 0 ]; then
            printf "  (none)\n"
        fi

        printf "\n  1) Add proxy\n"
        if [ "$_up_count" -gt 0 ]; then
            printf "  2) Remove proxy\n"
            printf "  3) Clear all\n"
        fi
        printf "  Enter) Back\n\n"
        printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
        IFS= read -r _up_choice

        case "$_up_choice" in
            1|add)
                printf "\nEnter HOST:PORT:SECRET\n"
                printf "Example: proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f\n"
                printf "Entry: "
                IFS= read -r _up_new
                if [ -z "$_up_new" ]; then
                    continue
                fi
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
            2|remove)
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
            3|clear)
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
        printf "  8) Set domain\n"
        printf "  9) Check domain\n"
        printf "\n  Settings\n"
        printf " 10) SOCKS5 auth\n"
        printf " 11) DC mapping\n"
        printf " 12) Port (%s%s%s)\n" "$C_GREEN" "$LISTEN_PORT" "$C_RESET"
        printf " 13) Pool size (%s%s%s)\n" "$C_GREEN" "$POOL_SIZE" "$C_RESET"
        if [ -n "$MT_LINK_IP" ]; then
            printf " 14) Public IP (%s%s%s)\n" "$C_GREEN" "$MT_LINK_IP" "$C_RESET"
        else
            printf " 14) Public IP (%snot set%s)\n" "$C_DIM" "$C_RESET"
        fi
        printf " 15) Show QR code\n"
        printf " 16) Update source\n"
        printf " 17) Remove binary\n"
        printf "\n  MTProto\n"
        if [ "$PROXY_MODE" = "mtproto" ]; then
            printf " 18) Mode (%smtproto%s)\n" "$C_GREEN" "$C_RESET"
        else
            printf " 18) Mode (%ssocks5%s)\n" "$C_DIM" "$C_RESET"
        fi
        if mt_secret_valid 2>/dev/null; then
            _sec_type="$(mt_secret_type 2>/dev/null || printf "set")"
            printf " 19) Secret (%s%s%s)\n" "$C_GREEN" "$_sec_type" "$C_RESET"
        else
            printf " 19) Secret (%snot set%s)\n" "$C_RED" "$C_RESET"
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
            printf " 20) Upstream proxies (%s%d set%s)\n" "$C_GREEN" "$_adv_up_count" "$C_RESET"
        else
            printf " 20) Upstream proxies (%snone%s)\n" "$C_DIM" "$C_RESET"
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
                configure_cf_domain
                ;;
            9)
                check_cf_domain
                ;;
            10)
                configure_socks_auth
                ;;
            11)
                configure_dc_ip_mapping
                ;;
            12)
                configure_listen_port
                ;;
            13)
                configure_pool_size
                ;;
            14)
                configure_mt_link_ip
                ;;
            15)
                if [ "$PROXY_MODE" = "mtproto" ]; then
                    show_mt_qr
                else
                    show_socks5_qr
                fi
                ;;
            16)
                configure_update_source
                ;;
            17)
                remove_all
                ;;
            18)
                configure_proxy_mode
                ;;
            19)
                configure_mt_secret
                ;;
            20)
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
