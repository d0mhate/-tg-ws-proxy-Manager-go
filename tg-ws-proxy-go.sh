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
OPENWRT_RELEASE_FILE="${OPENWRT_RELEASE_FILE:-/etc/openwrt_release}"
RELEASE_DOWNLOAD_BASE_URL="${RELEASE_DOWNLOAD_BASE_URL:-https://github.com/$REPO_OWNER/$REPO_NAME/releases/latest/download}"
RELEASE_URL="${RELEASE_URL:-}"
RELEASE_API_URL="${RELEASE_API_URL:-https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest}"
SCRIPT_RELEASE_BASE_URL="${SCRIPT_RELEASE_BASE_URL:-https://github.com/$REPO_OWNER/$REPO_NAME/releases/download}"
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
REQUIRED_TMP_KB="${REQUIRED_TMP_KB:-8192}"
PERSISTENT_SPACE_HEADROOM_KB="${PERSISTENT_SPACE_HEADROOM_KB:-2048}"
PID_FILE="${PID_FILE:-$INSTALL_DIR/pid}"
COMMAND_MODE="0"

if [ "$#" -gt 0 ]; then
    COMMAND_MODE="1"
fi

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

is_supported_openwrt_arch() {
    arch="$1"
    case "$arch" in
        mipsel_24kc|mips_24kc|aarch64*|x86_64|arm_cortex-a7|arm_cortex-a9|arm_cortex-a15_neon-vfpv4)
            return 0
            ;;
    esac
    return 1
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

    printf "%s" "$DEFAULT_BINARY_NAME"
}

resolved_release_url() {
    if [ -n "$RELEASE_URL" ]; then
        printf "%s" "$RELEASE_URL"
        return 0
    fi

    printf "%s/%s" "$RELEASE_DOWNLOAD_BASE_URL" "$(resolved_binary_name)"
}

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

read_first_line() {
    file="$1"
    [ -f "$file" ] || return 1
    IFS= read -r line < "$file" || return 1
    [ -n "$line" ] || return 1
    printf "%s" "$line"
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

read_config_value() {
    key="$1"
    [ -f "$PERSIST_CONFIG_FILE" ] || return 1
    sed -n "s/^${key}='\(.*\)'$/\1/p" "$PERSIST_CONFIG_FILE" 2>/dev/null | head -n 1
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
}

installed_version() {
    value="$(read_first_line "$VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

cached_source_version() {
    value="$(read_first_line "$SOURCE_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

persistent_installed_version() {
    value="$(read_first_line "$PERSIST_VERSION_FILE" 2>/dev/null || true)"
    normalize_version "$value"
}

latest_release_tag() {
    if command -v wget >/dev/null 2>&1; then
        wget -qO - "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
        return 0
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$RELEASE_API_URL" 2>/dev/null | tr -d '\n' | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
        return 0
    fi

    return 1
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

runtime_bin_path() {
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

autostart_enabled() {
    [ -f "$INIT_SCRIPT_PATH" ] || return 1
    ls "$RC_D_DIR"/*"$(basename "$INIT_SCRIPT_PATH")" >/dev/null 2>&1 || return 1
    bin_path="$(persistent_bin_path 2>/dev/null || true)"
    [ -n "$bin_path" ] || return 1
    [ -x "$bin_path" ] || return 1
    [ -r "$PERSIST_CONFIG_FILE" ]
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

    if command -v ps >/dev/null 2>&1; then
        ps -p "$pid" -o command= 2>/dev/null | grep -F -- "$path" >/dev/null 2>&1
        return $?
    fi

    kill -0 "$pid" 2>/dev/null
}

is_running() {
    if ! command -v pgrep >/dev/null 2>&1; then
        pid="$(read_first_line "$PID_FILE" 2>/dev/null || true)"
        [ -n "$pid" ] || return 1
        runtime_path="$(runtime_bin_path 2>/dev/null || true)"
        [ -n "$runtime_path" ] || return 1
        pid_matches_binary "$pid" "$runtime_path"
        return $?
    fi
    current_pids >/dev/null 2>&1
}

current_pids() {
    all_pids=""
    for path in "$BIN_PATH" "$(persistent_bin_path 2>/dev/null || true)"; do
        [ -n "$path" ] || continue
        pid_from_file="$(read_first_line "$PID_FILE" 2>/dev/null || true)"
        if [ -n "$pid_from_file" ] && pid_matches_binary "$pid_from_file" "$path"; then
            all_pids="$all_pids
$pid_from_file"
            continue
        fi

        if ! command -v pgrep >/dev/null 2>&1; then
            continue
        fi

        pids="$(pgrep -f "$path" 2>/dev/null || true)"
        [ -n "$pids" ] || continue
        for pid in $pids; do
            pid_matches_binary "$pid" "$path" || continue
            all_pids="$all_pids
$pid"
        done
    done

    [ -n "$all_pids" ] || return 1
    printf "%s\n" "$all_pids" | awk 'NF && !seen[$0]++'
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
    printf "  username : <empty>\n"
    printf "  password : <empty>\n"
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

check_tmp_space() {
    free_kb="$(tmp_available_kb)"
    [ -n "$free_kb" ] || return 0
    [ "$free_kb" -ge "$REQUIRED_TMP_KB" ]
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

script_release_url() {
    ref="$1"
    printf "%s/%s/%s" "$SCRIPT_RELEASE_BASE_URL" "$ref" "$PERSIST_MANAGER_NAME"
}

copy_current_manager_script() {
    mkdir -p "$(dirname "$SOURCE_MANAGER_SCRIPT")" || return 1
    cp "$0" "$SOURCE_MANAGER_SCRIPT" || return 1
    chmod +x "$SOURCE_MANAGER_SCRIPT" || return 1
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

refresh_current_manager_script_from_source() {
    current_script="$(current_script_path 2>/dev/null || true)"
    [ -n "$current_script" ] || return 0
    [ -f "$SOURCE_MANAGER_SCRIPT" ] || return 0

    source_script="$(canonical_path "$SOURCE_MANAGER_SCRIPT")"
    [ "$current_script" = "$source_script" ] && return 0

    tmp_manager="$(canonical_path "$INSTALL_DIR/$PERSIST_MANAGER_NAME")"
    [ "$current_script" = "$tmp_manager" ] && return 0

    [ -w "$current_script" ] || return 0

    cp "$SOURCE_MANAGER_SCRIPT" "$current_script" || return 1
    chmod +x "$current_script" || return 1
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

write_source_version_file() {
    version="$1"
    [ -n "$version" ] || return 0
    printf "%s\n" "$version" > "$SOURCE_VERSION_FILE" || return 1
}

install_from_source() {
    mkdir -p "$INSTALL_DIR" || return 1
    cp "$SOURCE_BIN" "$BIN_PATH" || return 1
    chmod +x "$BIN_PATH" || return 1
    cp "$SOURCE_MANAGER_SCRIPT" "$INSTALL_DIR/$PERSIST_MANAGER_NAME" || return 1
    chmod +x "$INSTALL_DIR/$PERSIST_MANAGER_NAME" || return 1

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

install_persistent_from_source() {
    install_dir="$1"

    mkdir -p "$install_dir" || return 1
    cp "$SOURCE_BIN" "$install_dir/tg-ws-proxy" || return 1
    chmod +x "$install_dir/tg-ws-proxy" || return 1
    cp "$SOURCE_MANAGER_SCRIPT" "$install_dir/$PERSIST_MANAGER_NAME" || return 1
    chmod +x "$install_dir/$PERSIST_MANAGER_NAME" || return 1

    version="$(cached_source_version 2>/dev/null || true)"
    write_persistent_state "$install_dir" "$version" || return 1
    launcher_path="$(install_launcher "$install_dir/$PERSIST_MANAGER_NAME")" || return 1
    printf "%s" "$launcher_path"
}

write_autostart_config() {
    bin_path="$1"

    mkdir -p "$PERSIST_STATE_DIR" || return 1
    {
        printf "BIN='%s'\n" "$bin_path"
        printf "HOST='%s'\n" "$LISTEN_HOST"
        printf "PORT='%s'\n" "$LISTEN_PORT"
        printf "VERBOSE='%s'\n" "$VERBOSE"
    } > "$PERSIST_CONFIG_FILE" || return 1
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
        printf '%s\n' '    procd_open_instance'
        printf '%s\n' '    if [ "${VERBOSE:-0}" = "1" ]; then'
        printf '%s\n' '        procd_set_param command "$BIN" --host "$HOST" --port "$PORT" --verbose'
        printf '%s\n' '    else'
        printf '%s\n' '        procd_set_param command "$BIN" --host "$HOST" --port "$PORT"'
        printf '%s\n' '    fi'
        printf '%s\n' '    procd_set_param respawn'
        printf '%s\n' '    procd_set_param stdout 1'
        printf '%s\n' '    procd_set_param stderr 1'
        printf '%s\n' '    procd_close_instance'
        printf '%s\n' '}'
    } > "$INIT_SCRIPT_PATH" || return 1

    chmod +x "$INIT_SCRIPT_PATH" || return 1
}

ensure_source_binary_current() {
    latest_tag="$(latest_release_tag 2>/dev/null || true)"

    if [ -n "$latest_tag" ]; then
        cached_tag="$(cached_source_version 2>/dev/null || true)"
        need_download="0"

        if [ ! -f "$SOURCE_BIN" ]; then
            printf "%sLocal binary not found%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ -z "$cached_tag" ]; then
            printf "%sLocal binary version is unknown%s\n\n" "$C_YELLOW" "$C_RESET"
            need_download="1"
        elif [ "$cached_tag" != "$latest_tag" ]; then
            printf "%sLocal binary is outdated%s\n\n" "$C_YELLOW" "$C_RESET"
            printf "Cached version: %s\n" "$cached_tag"
            printf "Latest version: %s\n\n" "$latest_tag"
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
                write_source_version_file "$latest_tag" || return 1
            fi
        fi
    elif [ ! -f "$SOURCE_BIN" ]; then
        printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
        printf "Check GitHub API access or network access\n"
        return 1
    fi
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
    if ! ensure_source_manager_current "$(cached_source_version 2>/dev/null || true)"; then
        pause
        return 1
    fi

    launcher_path="$(install_from_source)" || return 1

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
    if ! ensure_source_manager_current "$(cached_source_version 2>/dev/null || true)"; then
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

    latest_tag="$(latest_release_tag)"
    if [ -z "$latest_tag" ]; then
        printf "%sCould not detect latest release version%s\n\n" "$C_RED" "$C_RESET"
        printf "Check GitHub API access or network access\n"
        pause
        return 1
    fi

    current_tag="$(installed_version 2>/dev/null || true)"
    if [ -z "$current_tag" ]; then
        current_tag="$(persistent_installed_version 2>/dev/null || true)"
    fi
    if [ -n "$current_tag" ] && [ "$current_tag" = "$latest_tag" ] && [ -x "$BIN_PATH" ] && ! has_persistent_install; then
        printf "%sAlready on the latest version%s\n\n" "$C_GREEN" "$C_RESET"
        printf "Current version: %s\n" "$current_tag"
        pause
        return 0
    fi

    printf "Current version: %s\n" "${current_tag:-unknown}"
    printf "Latest version: %s\n\n" "$latest_tag"

    if ! release_url_reachable; then
        printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
        printf "Check GitHub Release visibility or network access\n"
        pause
        return 1
    fi

    rm -f "$SOURCE_BIN" "$SOURCE_VERSION_FILE"
    if ! download_binary; then
        printf "%sDownload failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    write_source_version_file "$latest_tag" || return 1
    if ! ensure_source_manager_current "$latest_tag" "1"; then
        printf "%sManager script update failed%s\n" "$C_RED" "$C_RESET"
        pause
        return 1
    fi
    refresh_current_manager_script_from_source || true
    launcher_path="$(install_from_source)" || return 1
    if has_persistent_install; then
        persist_dir="$(persistent_install_dir 2>/dev/null || true)"
        if [ -n "$persist_dir" ]; then
            launcher_path="$(install_persistent_from_source "$persist_dir")" || return 1
        fi
    fi

    show_header
    printf "%sUpdated to %s%s\n\n" "$C_GREEN" "$latest_tag" "$C_RESET"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    pause
}

run_binary() {
    bin_path="$(runtime_bin_path 2>/dev/null || true)"
    [ -n "$bin_path" ] || return 1

    if [ "$VERBOSE" = "1" ]; then
        "$bin_path" --host "$LISTEN_HOST" --port "$LISTEN_PORT" --verbose
    else
        "$bin_path" --host "$LISTEN_HOST" --port "$LISTEN_PORT"
    fi
}

start_proxy() {
    bin_path="$(runtime_bin_path 2>/dev/null || true)"
    if [ -z "$bin_path" ] || [ ! -x "$bin_path" ]; then
        show_header
        printf "%s%s binary is not installed%s\n" "$C_RED" "$APP_NAME" "$C_RESET"
        pause
        return 1
    fi

    if is_running; then
        show_header
        printf "%s%s is already running%s\n\n" "$C_YELLOW" "$APP_NAME" "$C_RESET"
        printf "Stop it first from this or another shell.\n"
        pause
        return 0
    fi

    if port_in_use; then
        show_header
        printf "%sPort %s is already busy%s\n\n" "$C_RED" "$LISTEN_PORT" "$C_RESET"
        printf "Free the port first or change LISTEN_PORT\n"
        pause
        return 1
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

enable_autostart() {
    show_header
    started_now="0"
    start_note=""

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
    rm -f "$INIT_SCRIPT_PATH"

    if [ -x "$BIN_PATH" ]; then
        install_launcher "$0" >/dev/null 2>&1 || true
    else
        rm -f "$LAUNCHER_PATH" "/tmp/$LAUNCHER_NAME"
    fi

    printf "%sAutostart disabled and persistent copy removed%s\n" "$C_GREEN" "$C_RESET"
    pause
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

    printf "%sSummary%s\n" "$C_BOLD" "$C_RESET"
    printf "  proxy     : %s\n" "$proxy_state"
    printf "  autostart : %s\n" "$autostart_state"
    printf "  verbose   : %s\n" "$verbose_state"
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
    printf "  sh %s update         update from latest release\n" "$0"
    printf "  sh %s enable-autostart   enable OpenWrt autostart\n" "$0"
    printf "  sh %s disable-autostart  disable OpenWrt autostart\n" "$0"
    printf "  sh %s start          run proxy in terminal\n" "$0"
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
