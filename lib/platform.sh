#!/bin/sh
# platform.sh

lan_ip() {
    uci get network.lan.ipaddr 2>/dev/null | cut -d/ -f1
}

is_openwrt() {
    [ -f "$OPENWRT_RELEASE_FILE" ] || return 1

    if grep -q "OpenWrt" "$OPENWRT_RELEASE_FILE" 2>/dev/null; then
        return 0
    fi

    grep -q "^DISTRIB_ARCH='" "$OPENWRT_RELEASE_FILE" 2>/dev/null
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
        arm_cortex-a7*|arm_cortex-a9*|arm_cortex-a15*)
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
        mipsel_24kc|mips_24kc|aarch64*|x86_64|arm_cortex-a7*|arm_cortex-a9*|arm_cortex-a15*)
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

    need_kb="$(required_tmp_runtime_install_kb 2>/dev/null || true)"
    if [ -n "$need_kb" ]; then
        printf "tmp need: %s KB\n" "$need_kb"
    fi

    free_kb="$(tmp_available_kb)"
    if [ -n "$free_kb" ]; then
        printf "tmp free: %s KB\n" "$free_kb"
    fi
}
