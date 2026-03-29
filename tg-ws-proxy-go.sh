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
BINARY_NAME="${BINARY_NAME:-tg-ws-proxy-openwrt}"
RELEASE_URL="${RELEASE_URL:-https://github.com/$REPO_OWNER/$REPO_NAME/releases/latest/download/$BINARY_NAME}"
SOURCE_BIN="${SOURCE_BIN:-/tmp/tg-ws-proxy-openwrt}"
INSTALL_DIR="${INSTALL_DIR:-/tmp/tg-ws-proxy-go}"
BIN_PATH="${BIN_PATH:-$INSTALL_DIR/tg-ws-proxy}"
LAUNCHER_PATH="${LAUNCHER_PATH:-/usr/bin/$LAUNCHER_NAME}"
LISTEN_HOST="${LISTEN_HOST:-0.0.0.0}"
LISTEN_PORT="${LISTEN_PORT:-1080}"
VERBOSE="${VERBOSE:-0}"
REQUIRED_TMP_KB="${REQUIRED_TMP_KB:-8192}"
COMMAND_MODE="0"

if [ "$#" -gt 0 ]; then
    COMMAND_MODE="1"
fi

lan_ip() {
    uci get network.lan.ipaddr 2>/dev/null | cut -d/ -f1
}

is_openwrt() {
    [ -f /etc/openwrt_release ] && grep -q "OpenWrt" /etc/openwrt_release 2>/dev/null
}

openwrt_arch() {
    awk -F"'" '/DISTRIB_ARCH/ {print $2}' /etc/openwrt_release 2>/dev/null
}

tmp_available_kb() {
    df -k /tmp 2>/dev/null | awk 'NR==2 {print $4+0}'
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

is_running() {
    if ! command -v pgrep >/dev/null 2>&1; then
        return 1
    fi
    pgrep -f "$BIN_PATH" >/dev/null 2>&1
}

current_pids() {
    if ! command -v pgrep >/dev/null 2>&1; then
        return 1
    fi
    pgrep -f "$BIN_PATH"
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

show_quick_commands() {
    printf "%sQuick commands%s\n" "$C_BOLD" "$C_RESET"
    printf "  sh %s install\n" "$0"
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
    if command -v wget >/dev/null 2>&1; then
        wget --spider "$RELEASE_URL" >/dev/null 2>&1
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -I -L --fail "$RELEASE_URL" >/dev/null 2>&1
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
        if [ "$arch" = "mipsel_24kc" ]; then
            printf "%sArch detected:%s %s\n" "$C_GREEN" "$C_RESET" "$arch"
        else
            printf "%sWarning:%s detected arch is %s and expected arch is mipsel_24kc\n" "$C_YELLOW" "$C_RESET" "$arch"
        fi
    fi

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

    printf "%sStatus%s\n" "$C_BOLD" "$C_RESET"
    printf "  binary    : %s\n" "$install_state"
    printf "  process   : %s\n" "$run_state"
    printf "  pid       : %s\n" "$pid"
    printf "  source    : %s\n" "$SOURCE_BIN"
    printf "  release   : %s\n" "$RELEASE_URL"
    printf "  installed : %s\n" "$BIN_PATH"
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
    target="$LAUNCHER_PATH"

    if ! mkdir -p "$(dirname "$target")" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
    fi

    if ! {
        printf '#!/bin/sh\n'
        printf 'sh %s "$@"\n' "$0"
    } > "$target" 2>/dev/null; then
        target="/tmp/$LAUNCHER_NAME"
        {
            printf '#!/bin/sh\n'
            printf 'sh %s "$@"\n' "$0"
        } > "$target" || return 1
    fi

    chmod +x "$target" || return 1
    printf "%s" "$target"
}

download_binary() {
    mkdir -p "$(dirname "$SOURCE_BIN")" || return 1

    if command -v wget >/dev/null 2>&1; then
        wget -O "$SOURCE_BIN" "$RELEASE_URL"
        return $?
    fi

    if command -v curl >/dev/null 2>&1; then
        curl -L --fail -o "$SOURCE_BIN" "$RELEASE_URL"
        return $?
    fi

    return 1
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

    if [ ! -f "$SOURCE_BIN" ]; then
        printf "%sLocal binary not found%s\n\n" "$C_YELLOW" "$C_RESET"
        printf "Trying to download from GitHub Release\n"
        printf "%s\n\n" "$RELEASE_URL"
        if ! release_url_reachable; then
            printf "%sRelease URL is not reachable%s\n\n" "$C_RED" "$C_RESET"
            printf "Check GitHub Release visibility or network access\n"
            pause
            return 1
        fi
        if ! download_binary; then
            printf "%sDownload failed%s\n\n" "$C_RED" "$C_RESET"
            printf "You can also place the binary here manually\n"
            printf "  %s\n" "$SOURCE_BIN"
            pause
            return 1
        fi
    fi

    mkdir -p "$INSTALL_DIR" || return 1
    cp "$SOURCE_BIN" "$BIN_PATH" || return 1
    chmod +x "$BIN_PATH" || return 1
    launcher_path="$(install_launcher)" || launcher_path=""

    show_header
    printf "%sBinary installed%s\n\n" "$C_GREEN" "$C_RESET"
    printf "Source:\n  %s\n\n" "$SOURCE_BIN"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
    if [ -n "$launcher_path" ]; then
        printf "\nLauncher:\n  %s\n" "$launcher_path"
    fi
    pause
}

run_binary() {
    if [ "$VERBOSE" = "1" ]; then
        "$BIN_PATH" --host "$LISTEN_HOST" --port "$LISTEN_PORT" --verbose
    else
        "$BIN_PATH" --host "$LISTEN_HOST" --port "$LISTEN_PORT"
    fi
}

start_proxy() {
    if [ ! -x "$BIN_PATH" ]; then
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
    trap 'interrupted="1"; kill -INT "$child_pid" 2>/dev/null' INT
    wait "$child_pid"
    code="$?"
    trap - INT
    printf "\n%s%s exited with code %s%s\n" "$C_YELLOW" "$APP_NAME" "$code" "$C_RESET"
    if [ "$interrupted" = "1" ]; then
        printf "Returned to menu after Ctrl+C\n"
    fi
    pause
}

stop_running() {
    if ! is_running; then
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
    rm -rf "$INSTALL_DIR"
    rm -f "$SOURCE_BIN"
    rm -f "$LAUNCHER_PATH" "/tmp/$LAUNCHER_NAME"

    show_header
    printf "%sBinary launcher and downloaded files removed%s\n" "$C_GREEN" "$C_RESET"
    pause
}

toggle_verbose() {
    if [ "$VERBOSE" = "1" ]; then
        VERBOSE="0"
    else
        VERBOSE="1"
    fi
}

show_help() {
    show_header
    printf "%sUsage%s\n" "$C_BOLD" "$C_RESET"
    printf "  sh %s                start menu mode\n" "$0"
    printf "  sh %s install        install or update binary\n" "$0"
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
    show_header
    show_telegram_settings
    printf "\n"
    show_status
    printf "\n%sActions%s\n" "$C_BOLD" "$C_RESET"
    printf "  1) Install or update binary\n"
    printf "  2) Run proxy in terminal\n"
    printf "  3) Stop proxy\n"
    printf "  4) Restart proxy in terminal\n"
    printf "  5) Toggle verbose\n"
    printf "  6) Show Telegram SOCKS5 settings\n"
    printf "  7) Show quick commands\n"
    printf "  8) Remove binary and runtime files\n"
    printf "  Enter) Exit\n\n"
    printf "%sSelect:%s " "$C_CYAN" "$C_RESET"
    read choice

    case "$choice" in
        1) install_binary ;;
        2) start_proxy ;;
        3) stop_proxy ;;
        4) restart_proxy ;;
        5) toggle_verbose ;;
        6) show_telegram_only ;;
        7) show_quick_only ;;
        8) remove_all ;;
        *) exit 0 ;;
    esac
}

if [ "$COMMAND_MODE" = "1" ]; then
    case "$1" in
        install) install_binary ;;
        start) start_proxy ;;
        stop) stop_proxy ;;
        restart) restart_proxy ;;
        status) show_header; show_status ;;
        quick) show_quick_only ;;
        telegram) show_telegram_only ;;
        remove) remove_all ;;
        help|-h|--help) show_help ;;
        *)
            show_help
            exit 1
            ;;
    esac
    exit 0
fi

while true; do
    menu
done
