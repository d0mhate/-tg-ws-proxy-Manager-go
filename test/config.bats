#!/usr/bin/env bats

setup() {
  # создаём временный каталог для файлов состояния и конфигурации
  tmpdir="$(mktemp -d)"
  export PERSIST_CONFIG_FILE="$tmpdir/config"
  export PERSIST_STATE_DIR="$tmpdir/state"
}

teardown() {
  rm -rf "$tmpdir"
}

@test "read_config_value returns key value from config file" {
  # записываем простой конфиг
  printf "FOO='bar'\n" > "$PERSIST_CONFIG_FILE"
  run bash -c '
    source ./lib/config.sh
    value="$(read_config_value FOO)"
    printf "%s" "$value"
  '
  [ "$status" -eq 0 ]
  [ "$output" = "bar" ]
}

@test "normalize_dc_ip_list normalizes a valid DC:IP list" {
  run bash -c '
    source ./lib/config.sh
    result="$(normalize_dc_ip_list "1:127.0.0.1, 2:10.0.0.1")"
    printf "%s" "$result"
  '
  [ "$status" -eq 0 ]
  # пробелы и порядок должны быть приведены в норму
  [ "$output" = "1:127.0.0.1, 2:10.0.0.1" ]
}

@test "normalize_dc_ip_list fails on invalid IP" {
  run bash -c '
    source ./lib/config.sh
    normalize_dc_ip_list "1:999.1.1.1"
  '
  # выходной статус ненулевой при ошибке в IP
  [ "$status" -ne 0 ]
}

@test "mt_secret_valid accepts valid secrets and rejects invalid ones" {
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="00112233445566778899aabbccddeeff"
    mt_secret_valid
  '
  [ "$status" -eq 0 ]  # plain 32 символа
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="dd00112233445566778899aabbccddeeff"
    mt_secret_valid
  '
  [ "$status" -eq 0 ]  # dd‑префикс
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="invalid_secret"
    mt_secret_valid
  '
  [ "$status" -ne 0 ]  # недопустимый формат
}

@test "mt_secret_type detects secret type" {
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="00112233445566778899aabbccddeeff"
    mt_secret_type
  '
  [ "$output" = "plain" ]
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="dd00112233445566778899aabbccddeeff"
    mt_secret_type
  '
  [ "$output" = "dd" ]
  run bash -c '
    source ./lib/config.sh
    MT_SECRET="ee00112233445566778899aabbccddeeff737461636b6f766572666c6f7731"
    mt_secret_type
  '
  # ee‑секрет с декодируемым доменом "stackoverflow1"
  [ "$output" = "ee:stackoverflow1" ]
}

@test "validate_upstream_proxy_entry accepts valid HOST:PORT:SECRET entry" {
  run bash -c '
    source ./lib/config.sh
    validate_upstream_proxy_entry "host.example.com:8080:00112233445566778899aabbccddeeff"
  '
  [ "$status" -eq 0 ]
  run bash -c '
    source ./lib/config.sh
    validate_upstream_proxy_entry "host.example.com:port:secret"
  '
  [ "$status" -ne 0 ]
}

@test "mt_proxy_link builds proxy link for plain and dd secret" {
  run bash -c '
    source ./lib/config.sh
    MT_LINK_IP="1.2.3.4"
    LISTEN_PORT="1080"
    MT_SECRET="00112233445566778899aabbccddeeff"
    echo "$(mt_proxy_link)"
  '
  # plain secret получает dd‑префикс
  [ "$output" = "tg://proxy?server=1.2.3.4&port=1080&secret=dd00112233445566778899aabbccddeeff" ]
  run bash -c '
    source ./lib/config.sh
    MT_LINK_IP="1.2.3.4"
    LISTEN_PORT="1080"
    MT_SECRET="dd00112233445566778899aabbccddeeff"
    echo "$(mt_proxy_link)"
  '
  # dd‑секрет передаётся как есть
  [ "$output" = "tg://proxy?server=1.2.3.4&port=1080&secret=dd00112233445566778899aabbccddeeff" ]
}

@test "socks5_proxy_link includes username and password when set" {
  run bash -c '
    source ./lib/config.sh
    MT_LINK_IP="5.6.7.8"
    LISTEN_PORT="1080"
    SOCKS_USERNAME="user"
    SOCKS_PASSWORD="pass"
    echo "$(socks5_proxy_link)"
  '
  [ "$output" = "tg://socks?server=5.6.7.8&port=1080&user=user&pass=pass" ]
  run bash -c '
    source ./lib/config.sh
    MT_LINK_IP="5.6.7.8"
    LISTEN_PORT="1080"
    SOCKS_USERNAME=""
    SOCKS_PASSWORD=""
    echo "$(socks5_proxy_link)"
  '
  [ "$output" = "tg://socks?server=5.6.7.8&port=1080" ]
}
