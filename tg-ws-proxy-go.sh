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
SOURCE_BIN="${SOURCE_BIN:-/tmp/tg-ws-proxy-openwrt}"
INSTALL_DIR="${INSTALL_DIR:-/tmp/tg-ws-proxy-go}"
BIN_PATH="${BIN_PATH:-$INSTALL_DIR/tg-ws-proxy}"
LISTEN_HOST="${LISTEN_HOST:-0.0.0.0}"
LISTEN_PORT="${LISTEN_PORT:-1080}"
VERBOSE="${VERBOSE:-0}"
COMMAND_MODE="0"

if [ "$#" -gt 0 ]; then
    COMMAND_MODE="1"
fi

lan_ip() {
    uci get network.lan.ipaddr 2>/dev/null | cut -d/ -f1
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
    printf "  installed : %s\n" "$BIN_PATH"
    printf "  listen    : %s:%s\n" "$LISTEN_HOST" "$LISTEN_PORT"
    printf "  mode      : terminal logs only\n"
    printf "  verbose   : %s\n" "$verbose_state"
}

install_binary() {
    if [ ! -f "$SOURCE_BIN" ]; then
        show_header
        printf "%sSource binary not found%s\n\n" "$C_RED" "$C_RESET"
        printf "Expected file:\n  %s\n\n" "$SOURCE_BIN"
        printf "Copy your build there first.\n"
        pause
        return 1
    fi

    mkdir -p "$INSTALL_DIR" || return 1
    cp "$SOURCE_BIN" "$BIN_PATH" || return 1
    chmod +x "$BIN_PATH" || return 1

    show_header
    printf "%sBinary installed%s\n\n" "$C_GREEN" "$C_RESET"
    printf "Installed to:\n  %s\n" "$BIN_PATH"
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

    show_header
    printf "%sStarting %s in terminal%s\n\n" "$C_GREEN" "$APP_NAME" "$C_RESET"
    printf "Logs will be printed here.\n"
    printf "Stop with Ctrl+C\n"
    printf "Bind: %s:%s\n\n" "$LISTEN_HOST" "$LISTEN_PORT"
    run_binary
    code="$?"
    printf "\n%s%s exited with code %s%s\n" "$C_YELLOW" "$APP_NAME" "$code" "$C_RESET"
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

    show_header
    printf "%sBinary and runtime files removed%s\n" "$C_GREEN" "$C_RESET"
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
