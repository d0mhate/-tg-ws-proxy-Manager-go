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
. "$SCRIPT_DIR/lib/ui.sh"
. "$SCRIPT_DIR/lib/menu.sh"

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
