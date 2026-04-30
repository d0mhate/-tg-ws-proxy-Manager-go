#!/usr/bin/env bats

setup() {
    export MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR"
    source "$BATS_TEST_DIRNAME/helpers/menu_fixture.sh"
}

@test "choose_update_source_mode_numbered keeps current on enter" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_update_source_mode_numbered preview 2>/dev/null <<< ""
    '

    [ "$status" -eq 0 ]
    [ "$output" = "preview" ]
}

@test "choose_update_source_mode_numbered maps numeric choice" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_update_source_mode_numbered release 2>/dev/null <<< "2"
    '

    [ "$status" -eq 0 ]
    [ "$output" = "preview" ]
}

@test "prompt_manual_release_tag keeps current tag on enter" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        prompt_manual_release_tag v1.2.3 2>/dev/null <<< ""
    '

    [ "$status" -eq 0 ]
    [ "$output" = "v1.2.3" ]
}

@test "prompt_manual_release_tag rejects invalid tag format" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        prompt_manual_release_tag "" 2>&1 <<< "1.2.3"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Release tag must look like v1.1.28"* ]]
}

@test "prompt_manual_release_tag rejects tag below minimum" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        prompt_manual_release_tag "" 2>&1 <<< "v1.1.28"
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Release tag must be v1.1.29 or newer"* ]]
}

@test "choose_release_ref_numbered selects latest as empty ref" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_release_ref_numbered v1.2.0 2>/dev/null <<< "1"
    '

    [ "$status" -eq 0 ]
    [ "$output" = "" ]
}

@test "choose_release_ref_numbered selects tag from menu" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_release_ref_numbered latest 2>/dev/null <<< "2"
    '

    [ "$status" -eq 0 ]
    [ "$output" = "v1.2.3" ]
}

@test "choose_release_ref_numbered supports manual tag entry" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        printf "4\nv1.3.0\n" | choose_release_ref_numbered latest 2>/dev/null
    '

    [ "$status" -eq 0 ]
    [ "$output" = "v1.3.0" ]
}

@test "choose_update_source_mode falls back to plain text prompt" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        can_use_arrow_update_source_picker() { return 1; }
        can_use_numbered_update_source_picker() { return 1; }
        choose_update_source_mode release 2>/dev/null <<< "preview"
    '

    [ "$status" -eq 0 ]
    [ "$output" = "preview" ]
}

@test "configure_update_source saves pinned release tag" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_update_source_mode() { printf "release"; }
        choose_release_ref() { printf "v1.2.3"; }
        configure_update_source
        printf "\nCHANNEL=%s\nTAG=%s\nPREVIEW=%s\n" "$UPDATE_CHANNEL" "$RELEASE_TAG" "$PREVIEW_BRANCH"
        printf "STATE=%s\n" "$(cat "$MENU_FIXTURE_TMPDIR/update_state")"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Update source saved: release v1.2.3"* ]]
    [[ "$output" == *"CHANNEL=release"* ]]
    [[ "$output" == *"TAG=v1.2.3"* ]]
    [[ "$output" == *"STATE=release|v1.2.3"* ]]
}

@test "configure_update_source saves preview branch" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_update_source_mode() { printf "preview"; }
        PREVIEW_BRANCH="preview-main"
        configure_update_source <<< ""
        printf "\nCHANNEL=%s\nTAG=%s\nPREVIEW=%s\n" "$UPDATE_CHANNEL" "$RELEASE_TAG" "$PREVIEW_BRANCH"
        printf "STATE=%s\n" "$(cat "$MENU_FIXTURE_TMPDIR/update_state")"
    '

    [ "$status" -eq 0 ]
    [[ "$output" == *"Update source saved: preview preview-main"* ]]
    [[ "$output" == *"CHANNEL=preview"* ]]
    [[ "$output" == *"PREVIEW=preview-main"* ]]
    [[ "$output" == *"STATE=preview|preview-main"* ]]
}

@test "configure_update_source rejects empty preview branch when none saved" {
    run env MENU_FIXTURE_TMPDIR="$BATS_TEST_TMPDIR" bash -c '
        source ./test/helpers/menu_fixture.sh
        choose_update_source_mode() { printf "preview"; }
        PREVIEW_BRANCH=""
        configure_update_source 2>&1 <<< ""
    '

    [ "$status" -ne 0 ]
    [[ "$output" == *"Preview branch cannot be empty"* ]]
}
