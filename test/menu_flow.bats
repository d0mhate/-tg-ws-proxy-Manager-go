#!/usr/bin/env bats

setup() {
    export MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR"
    source "$BATS_TEST_DIRNAME/helpers/menu_fixture.sh"
}

@test "advanced_menu routes full status action" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu >/dev/null <<< "1"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/show_status_called" ]
}

@test "advanced_menu routes quick commands action" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu >/dev/null <<< "3"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/show_quick_only_called" ]
}

@test "advanced_menu routes restart proxy action" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu >/dev/null <<< "5"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/restart_proxy_called" ]
}

@test "advanced_menu routes remove binary action" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu >/dev/null <<< "18"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/remove_all_called" ]
}

@test "advanced_menu returns session-close code after full remove" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_REMOVE_ALL_STATUS="20"
        advanced_menu >/dev/null <<< "18"
    '

    [ "$status" -eq 20 ]
    [ -f "$BATS_TEST_TMPDIR/remove_all_called" ]
}

@test "menu runs update when setup selected and confirmed" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        confirm_yn() { return 0; }
        menu >/dev/null <<< "1"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/update_binary_called" ]
}

@test "menu starts proxy in terminal mode by default" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_IS_RUNNING="0"
        printf "2\n" | menu >/dev/null
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/start_proxy_called" ]
    [ ! -f "$BATS_TEST_TMPDIR/start_proxy_bg_called" ]
}

@test "menu starts proxy in background when requested" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_IS_RUNNING="0"
        printf "2\nb\n" | menu >/dev/null
    '

    [ "$status" -eq 0 ]
    [ ! -f "$BATS_TEST_TMPDIR/start_proxy_called" ]
    [ -f "$BATS_TEST_TMPDIR/start_proxy_bg_called" ]
}

@test "menu stops proxy when already running" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_IS_RUNNING="1"
        menu >/dev/null <<< "2"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/stop_proxy_called" ]
}

@test "menu enables autostart when disabled" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_AUTOSTART_ENABLED="0"
        menu >/dev/null <<< "3"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/enable_autostart_called" ]
    [ ! -f "$BATS_TEST_TMPDIR/disable_autostart_called" ]
}

@test "menu disables autostart when enabled" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_AUTOSTART_ENABLED="1"
        menu >/dev/null <<< "3"
    '

    [ "$status" -eq 0 ]
    [ ! -f "$BATS_TEST_TMPDIR/enable_autostart_called" ]
    [ -f "$BATS_TEST_TMPDIR/disable_autostart_called" ]
}

@test "menu opens advanced menu on action 4" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu() { : > "$MENU_FIXTURE_TMPDIR/advanced_menu_called"; }
        menu >/dev/null <<< "4"
    '

    [ "$status" -eq 0 ]
    [ -f "$BATS_TEST_TMPDIR/advanced_menu_called" ]
}

@test "menu exits cleanly after full remove" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        advanced_menu() { return 20; }
        menu >/dev/null <<< "4"
    '

    [ "$status" -eq 0 ]
}
