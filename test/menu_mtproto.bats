#!/usr/bin/env bats

setup() {
    export MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR"
    source "$BATS_TEST_DIRNAME/helpers/menu_fixture.sh"
}

@test "configure_mt_secret clears current secret" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        MT_SECRET="dd00112233445566778899aabbccddeeff"
        can_use_numbered_update_source_picker() { return 1; }
        can_use_arrow_update_source_picker() { return 1; }
        configure_mt_secret 2>&1 <<< "clear"
        printf "\nSECRET=%s\nRESTART=%s\n" "$MT_SECRET" "$TEST_RESTART_PROMPT_CALLED"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Secret cleared"* ]]
    [[ "$output" == *"SECRET="* ]]
    [[ "$output" == *"RESTART=1"* ]]
}

@test "configure_mt_secret rejects non hex secret" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        can_use_numbered_update_source_picker() { return 1; }
        can_use_arrow_update_source_picker() { return 1; }
        configure_mt_secret 2>&1 <<< "zz-not-hex"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Secret must contain only hex characters"* ]]
}

@test "configure_mt_secret saves plain secret and prints link when ip is set" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        MT_LINK_IP="8.8.8.8"
        can_use_numbered_update_source_picker() { return 1; }
        can_use_arrow_update_source_picker() { return 1; }
        configure_mt_secret 2>&1 <<< "00112233445566778899aabbccddeeff"
        printf "\nSECRET=%s\nRESTART=%s\n" "$MT_SECRET" "$TEST_RESTART_PROMPT_CALLED"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Secret saved"* ]]
    [[ "$output" == *"tg://proxy?server=8.8.8.8&port=1080&secret=00112233445566778899aabbccddeeff"* ]]
    [[ "$output" == *"SECRET=00112233445566778899aabbccddeeff"* ]]
    [[ "$output" == *"RESTART=1"* ]]
}

@test "configure_mt_secret generates ee secret with hostname" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        can_use_numbered_update_source_picker() { return 0; }
        can_use_arrow_update_source_picker() { return 1; }
        configure_mt_secret 2>&1 <<< $'"'"'"'"'"'"'"'1\n3\nexample.com'"'"'"'"'"'"'"'
        printf "\nSECRET=%s\n" "$MT_SECRET"
    '

    [ "$status" -eq 0 ]
}

@test "configure_mt_link_ip saves suggested lan ip on empty input" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_LAN_IP="10.0.0.2"
        configure_mt_link_ip 2>&1 <<< ""
        printf "\nIP=%s\n" "$MT_LINK_IP"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Public IP saved"* ]]
    [[ "$output" == *"IP=10.0.0.2"* ]]
}

@test "configure_mt_link_ip clears current ip" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        MT_LINK_IP="8.8.8.8"
        configure_mt_link_ip 2>&1 <<< "clear"
        printf "\nIP=%s\n" "$MT_LINK_IP"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Public IP cleared"* ]]
    [[ "$output" == *"IP="* ]]
}

@test "configure_mt_link_ip accepts detected public ip on double nat" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        TEST_LAN_IP=""
        _detect_local_wan_ip() { printf "192.168.0.10"; }
        detect_wan_ip() { printf "9.9.9.9"; }
        configure_mt_link_ip 2>&1 <<< ""
        printf "\nIP=%s\n" "$MT_LINK_IP"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Double NAT detected"* ]]
    [[ "$output" == *"Detected public IP: 9.9.9.9"* ]]
    [[ "$output" == *"IP=9.9.9.9"* ]]
}

@test "configure_mt_upstream_proxies adds valid entry" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        configure_mt_upstream_proxies 2>&1 <<< $'"'"'"'"'"'"'"'1\nproxy.example.com:443:dd00112233445566778899aabbccddeeff\n'"'"'"'"'"'"'"'
        printf "\nLIST=%s\n" "$MT_UPSTREAM_PROXIES"
    '

    [ "$status" -eq 0 ]
}

@test "configure_mt_upstream_proxies rejects invalid entry and stays in loop" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        validate_upstream_proxy_entry() { return 1; }
        configure_mt_upstream_proxies 2>&1 <<< $'"'"'"'"'"'"'"'1\nbad-entry\n'"'"'"'"'"'"'"'
        printf "\nLIST=%s\n" "$MT_UPSTREAM_PROXIES"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Invalid entry. Expected HOST:PORT:SECRET"* ]]
    [[ "$output" == *"LIST="* ]]
}

@test "configure_mt_upstream_proxies removes selected entry" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        MT_UPSTREAM_PROXIES="one:443:aa,two:443:bb"
        configure_mt_upstream_proxies 2>&1 <<< $'"'"'"'"'"'"'"'2\n1\n'"'"'"'"'"'"'"'
        printf "\nLIST=%s\n" "$MT_UPSTREAM_PROXIES"
    '

    [ "$status" -eq 0 ]
}

@test "configure_mt_upstream_proxies clears all entries" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        MT_UPSTREAM_PROXIES="one:443:aa"
        configure_mt_upstream_proxies 2>&1 <<< "3"
        printf "\nLIST=%s\n" "$MT_UPSTREAM_PROXIES"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"All upstream proxies cleared"* ]]
    [[ "$output" == *"LIST="* ]]
}
