#!/usr/bin/env bats

setup() {
    export MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR"
    source "$BATS_TEST_DIRNAME/helpers/menu_fixture.sh"
}

@test "toggle_verbose flips value and syncs autostart config" {
    VERBOSE="0"

    toggle_verbose

    [ "$VERBOSE" = "1" ]
    [ "$TEST_SYNC_AUTOSTART_CALLED" = "1" ]
}

@test "toggle_cf_proxy flips value and writes config" {
    CF_PROXY="0"

    toggle_cf_proxy

    [ "$CF_PROXY" = "1" ]
    [ -f "$BATS_TEST_TMPDIR/write_settings_called" ]
}

@test "toggle_cf_proxy_first flips value and writes config" {
    CF_PROXY_FIRST="0"

    toggle_cf_proxy_first

    [ "$CF_PROXY_FIRST" = "1" ]
    [ -f "$BATS_TEST_TMPDIR/write_settings_called" ]
}

@test "configure_socks_auth disables auth on empty username" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        SOCKS_USERNAME="user"
        SOCKS_PASSWORD="pass"
        configure_socks_auth 2>&1 <<< ""
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"SOCKS5 auth disabled"* ]]
    [ -f "$BATS_TEST_TMPDIR/write_settings_called" ]
}

@test "configure_socks_auth rejects empty password" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_socks_auth 2>&1 <<< $'"'"'"'"'"'"'"'alice\n'"'"'"'"'"'"'"'
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Password cannot be empty when username is set"* ]]
}

@test "configure_socks_auth saves username and password" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_socks_auth 2>&1 <<< $'"'"'"'"'"'"'"'alice\nsecret'"'"'"'"'"'"'"'
        printf "\nUSER=%s\nPASS=%s\n" "$SOCKS_USERNAME" "$SOCKS_PASSWORD"
    '

    [ "$status" -eq 0 ]
}

@test "configure_listen_port rejects non numeric value" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_listen_port 2>&1 <<< "abc"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Port must be a number"* ]]
}

@test "configure_listen_port rejects out of range value" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_listen_port 2>&1 <<< "70000"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Port must be between 1 and 65535"* ]]
}

@test "configure_listen_port saves valid value" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_listen_port 2>&1 <<< "1443"
        printf "\nPORT=%s\n" "$LISTEN_PORT"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Port saved: 1443"* ]]
    [[ "$output" == *"PORT=1443"* ]]
}

@test "configure_pool_size retries invalid input and saves next valid value" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_pool_size 2>&1 <<< $'"'"'"'"'"'"'"'abc\n9'"'"'"'"'"'"'"'
        printf "\nPOOL=%s\n" "$POOL_SIZE"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Pool size must be a whole number between 0 and 64"* ]]
}

@test "configure_dc_ip_mapping resets to defaults" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        DC_IPS="1:1.1.1.1"
        configure_dc_ip_mapping 2>&1 <<< "default"
        printf "\nDC=%s\n" "$DC_IPS"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Telegram DC mapping reset to defaults"* ]]
    [[ "$output" == *"DC="* ]]
}

@test "configure_dc_ip_mapping rejects invalid mapping" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_dc_ip_mapping 2>&1 <<< "invalid"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Invalid DC mapping. Use format DC:IP, DC:IP"* ]]
}

@test "configure_dc_ip_mapping saves normalized mapping" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_dc_ip_mapping 2>&1 <<< "2:149.154.167.220, 4:149.154.167.220"
        printf "\nDC=%s\nRESTART=%s\n" "$DC_IPS" "$TEST_RESTART_PROMPT_CALLED"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Telegram DC mapping saved"* ]]
    [[ "$output" == *"DC=2:149.154.167.220, 4:149.154.167.220"* ]]
    [[ "$output" == *"RESTART=1"* ]]
}

@test "configure_cf_domain clears current domain" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        CF_DOMAIN="cf.example.com"
        configure_cf_domain 2>&1 <<< "3"
        printf "\nDOMAIN=%s\nRESTART=%s\n" "$CF_DOMAIN" "$TEST_RESTART_PROMPT_CALLED"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Custom Cloudflare domains cleared; using the built-in pool"* ]]
    [[ "$output" == *"DOMAIN="* ]]
    [[ "$output" == *"RESTART=1"* ]]
}

@test "configure_cf_domain saves domain and warns when CF proxy is off" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        CF_PROXY="0"
        configure_cf_domain 2>&1 <<< $'\''1\na.example.com,b.example.com'\''
        printf "\nDOMAIN=%s\n" "$CF_DOMAIN"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"CF proxy is currently off"* ]]
    [[ "$output" == *"Custom Cloudflare domains saved"* ]]
    [[ "$output" == *"domains saved, but CF route is disabled"* ]]
    [[ "$output" == *"DOMAIN=a.example.com,b.example.com"* ]]
}

@test "configure_cf_domain normalizes comma separated domains" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_cf_domain 2>&1 <<< $'\''1\n a.example.com , b.example.com , a.example.com '\''
        printf "\nDOMAIN=%s\n" "$CF_DOMAIN"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Custom Cloudflare domains saved"* ]]
    [[ "$output" == *"DOMAIN=a.example.com,b.example.com"* ]]
}

@test "configure_cf_domain returns on back" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        CF_DOMAIN="kept.example.com"
        configure_cf_domain 2>&1 <<< "4"
        printf "\nDOMAIN=%s\n" "$CF_DOMAIN"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"DOMAIN=kept.example.com"* ]]
}

@test "configure_cf_domain keeps current on default selection" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        CF_DOMAIN="kept.example.com"
        configure_cf_domain 2>&1 <<< ""
        printf "\nDOMAIN=%s\n" "$CF_DOMAIN"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"DOMAIN=kept.example.com"* ]]
}

@test "configure_cf_domain keeps current on explicit selection" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        CF_DOMAIN="kept.example.com"
        configure_cf_domain 2>&1 <<< "2"
        printf "\nDOMAIN=%s\n" "$CF_DOMAIN"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"No changes made."* ]]
    [[ "$output" == *"DOMAIN=kept.example.com"* ]]
}
