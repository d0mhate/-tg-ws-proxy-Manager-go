#!/usr/bin/env bats

setup() {
    SCRIPT="$BATS_TEST_DIRNAME/../lib/platform.sh"

    export DEFAULT_BINARY_NAME="default-bin"
    export OPENWRT_RELEASE_FILE="$BATS_TEST_TMPDIR/openwrt_release"

    uname() {
        case "$1" in
            -s) echo "Linux" ;;
            -m) echo "x86_64" ;;
        esac
    }

    uci() {
        echo "192.168.1.1/24"
    }

    tmp_available_kb() {
        echo "12345"
    }

    resolved_binary_name() {
        echo "resolved-bin"
    }

    source "$SCRIPT"
}

# --------------------------
# is_openwrt
# --------------------------

@test "is_openwrt true" {
    echo "OpenWrt" > "$OPENWRT_RELEASE_FILE"
    run is_openwrt
    [ "$status" -eq 0 ]
}

@test "is_openwrt false" {
    rm -f "$OPENWRT_RELEASE_FILE"
    run is_openwrt
    [ "$status" -ne 0 ]
}

# --------------------------
# openwrt_arch
# --------------------------

@test "openwrt_arch parses value" {
    echo "DISTRIB_ARCH='mipsel_24kc'" > "$OPENWRT_RELEASE_FILE"
    run openwrt_arch
    [ "$output" = "mipsel_24kc" ]
}

# --------------------------
# binary_name_for_arch (FULL)
# --------------------------

@test "binary_name_for_arch all branches" {
    run binary_name_for_arch mipsel_24kc
    [ "$output" = "tg-ws-proxy-openwrt-mipsel_24kc" ]

    run binary_name_for_arch mips_24kc
    [ "$output" = "tg-ws-proxy-openwrt-mips_24kc" ]

    run binary_name_for_arch aarch64
    [ "$output" = "tg-ws-proxy-openwrt-aarch64" ]

    run binary_name_for_arch x86_64
    [ "$output" = "tg-ws-proxy-openwrt-x86_64" ]

    run binary_name_for_arch arm_cortex-a7
    [ "$output" = "tg-ws-proxy-openwrt-armv7" ]

    run binary_name_for_arch arm_cortex-a9
    [ "$output" = "tg-ws-proxy-openwrt-armv7" ]

    run binary_name_for_arch arm_cortex-a15_neon-vfpv4
    [ "$output" = "tg-ws-proxy-openwrt-armv7" ]

    run binary_name_for_arch unknown
    [ "$output" = "default-bin" ]
}

# --------------------------
# generic_binary_name (FULL)
# --------------------------

@test "generic_binary_name linux variants" {
    uname() { [ "$1" = "-s" ] && echo linux || echo x86_64; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-openwrt-x86_64" ]

    uname() { [ "$1" = "-s" ] && echo linux || echo aarch64; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-openwrt-aarch64" ]

    uname() { [ "$1" = "-s" ] && echo linux || echo armv7l; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-openwrt-armv7" ]

    uname() { [ "$1" = "-s" ] && echo linux || echo i686; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-linux-386" ]
}

@test "generic_binary_name darwin" {
    uname() { [ "$1" = "-s" ] && echo darwin || echo arm64; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-darwin-arm64" ]
}

@test "generic_binary_name freebsd" {
    uname() { [ "$1" = "-s" ] && echo freebsd || echo arm64; }
    run generic_binary_name
    [ "$output" = "tg-ws-proxy-freebsd-arm64" ]
}

@test "generic_binary_name fallback" {
    uname() { echo unknown; }
    run generic_binary_name
    [ "$output" = "default-bin" ]
}

# --------------------------
# is_supported_openwrt_arch
# --------------------------

@test "is_supported_openwrt_arch true cases" {
    for arch in mipsel_24kc mips_24kc aarch64 x86_64 arm_cortex-a7; do
        run is_supported_openwrt_arch "$arch"
        [ "$status" -eq 0 ]
    done
}

@test "is_supported_openwrt_arch false case" {
    run is_supported_openwrt_arch unknown
    [ "$status" -ne 0 ]
}

# --------------------------
# lan_ip
# --------------------------

@test "lan_ip parses ip" {
    run lan_ip
    [ "$output" = "192.168.1.1" ]
}

# --------------------------
# show_environment_checks
# --------------------------

@test "show_environment_checks output" {
    echo "OpenWrt" > "$OPENWRT_RELEASE_FILE"
    echo "DISTRIB_ARCH='mipsel_24kc'" >> "$OPENWRT_RELEASE_FILE"

    C_GREEN=""
    C_RESET=""
    C_YELLOW=""

    run show_environment_checks

    [[ "$output" == *"OpenWrt detected"* ]]
    [[ "$output" == *"Arch detected"* ]]
    [[ "$output" == *"Release asset: resolved-bin"* ]]
    [[ "$output" == *"tmp free: 12345 KB"* ]]
}