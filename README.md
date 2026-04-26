# TG WS Proxy Go

[![CI](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/d0mhate/-tg-ws-proxy-Manager-go)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/d0mhate/-tg-ws-proxy-Manager-go/total)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/d0mhate/-tg-ws-proxy-Manager-go)](https://goreportcard.com/report/github.com/d0mhate/-tg-ws-proxy-Manager-go)
![Go Version](https://img.shields.io/badge/go-1.26-00ADD8)
![License](https://img.shields.io/badge/license-MIT-green)
![CGO](https://img.shields.io/badge/CGO__enabled-0-brightgreen)
![Protocol](https://img.shields.io/badge/protocol-SOCKS5%20%7C%20MTProto-blueviolet)

Компактный Go-порт `tg-ws-proxy` для Telegram. Основной сценарий - запуск на
OpenWrt-роутере через меню-скрипт без Python runtime и desktop-обвязки.

Прокси принимает подключения от Telegram как `SOCKS5` или `MTProto` и ведет
трафик до Telegram DC через WebSocket/TLS. Если основной маршрут недоступен,
можно включить Cloudflare route или внешний MTProto proxy как запасной путь.

```text
Telegram app / Telegram Desktop
  -> SOCKS5 или MTProto
  -> tg-ws-proxy
  -> WebSocket + TLS -> Telegram DC
       -> Cloudflare route, если включен
       -> внешний MTProto proxy, если задан
       -> прямой TCP fallback
```

> [!IMPORTANT]
> Используйте на свой страх и риск. Проект не гарантирует 100% работу и не
> берет на себя ответственность за состояние роутера или сети.

| | Python manager | This port |
|---|---|---|
| Runtime | нужен Python | один статический бинарник |
| Размер | зависит от Python-пакетов | OpenWrt binary около `5 MB` |
| OpenWrt | больше места и зависимостей | скачать и запустить |
| CGO | не важно | `CGO_ENABLED=0` |
| Управление | script/menu | script/menu + прямой CLI |

## Quick Start

### OpenWrt через меню

Подключитесь к роутеру по SSH и запустите:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest/download/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh
```

Обычно достаточно трех действий:

1. `Setup / Update` -> подтвердить `y`
2. `Start proxy` -> выбрать `t` для запуска в терминале или `b` для фона
3. `Enable autostart`, если нужен запуск после перезагрузки

Скрипт создает короткую команду `tgm`. Через нее можно снова открыть меню или
выполнить команды вроде `tgm stop`, `tgm status`, `tgm remove`.

### OpenWrt без меню

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest/download/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh install && sh /tmp/tg-ws-proxy-go.sh start
```

Запуск в фоне:

```bash
sh /tmp/tg-ws-proxy-go.sh start-background
```

### Pre-built binaries

Готовые бинарники лежат в [Releases](../../releases). Меню-скрипт сам выбирает
подходящий asset по архитектуре роутера.

![OpenWrt](https://img.shields.io/badge/OpenWrt-mipsel__24kc%20%7C%20mips__24kc%20%7C%20armv7%20%7C%20aarch64%20%7C%20x86__64-blue)
![Linux](https://img.shields.io/badge/Linux-x86__64%20%7C%20aarch64%20%7C%20armv7%20%7C%20armv6%20%7C%20386%20%7C%20riscv64%20%7C%20loong64-orange)
![macOS](https://img.shields.io/badge/macOS-amd64%20%7C%20arm64-lightgrey)
![Windows](https://img.shields.io/badge/Windows-amd64%20%7C%20arm64-0078D6)
![FreeBSD](https://img.shields.io/badge/FreeBSD-amd64%20%7C%20arm64-red)

<details>
<summary>Поддерживаемые release-архитектуры</summary>

- OpenWrt: `mipsel_24kc`, `mips_24kc`, `armv7`, `armv8l`, `aarch64`, `x86_64`
- Linux: `x86_64`, `aarch64`, `armv7`, `armv6`, `386`, `riscv64`, `loong64`
- Additional targets: `mips64`, `mips64el`
- macOS: `amd64`, `arm64`
- Windows: `amd64`, `arm64`
- FreeBSD: `amd64`, `arm64`

</details>

Проверенная цель:

- `Xiaomi Mi Router 4A Gigabit Edition v2`
- `OpenWrt 24.10.5`
- `ramips/mt7621`
- `mipsel_24kc`

## Telegram Setup

### SOCKS5

По умолчанию прокси запускается как SOCKS5 на порту `1080`.

Если прокси запущен на роутере, в Telegram укажите:

| Field | Value |
|---|---|
| Type | `SOCKS5` |
| Host | IP роутера |
| Port | `1080` |
| Username | пусто, если auth не включена |
| Password | пусто, если auth не включена |

Если прокси запущен на той же машине, используйте host `127.0.0.1`.

SOCKS5 auth включается в меню: `Advanced -> SOCKS5 auth`.

### MTProto

MTProto включается через меню:

1. `Advanced`
2. `18) Mode` -> `mtproto`
3. `19) Secret` -> сгенерировать secret или вставить свой
4. `14) Public IP` -> указать публичный IP сервера

После этого меню покажет готовую ссылку `tg://proxy?...`. Ее можно открыть в
Telegram или вывести QR-код через `15) Show QR code`.

## Usage

Прямой запуск бинарника без меню:

```bash
./tg-ws-proxy [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--mode <socks5\|mtproto>` | `socks5` | Режим работы прокси |
| `--host <IP>` | `127.0.0.1` | Адрес, на котором слушает сервер |
| `--port <PORT>` | `1080` | Порт, на котором слушает сервер |
| `--pprof-addr <ADDR>` | пусто | Включить `pprof` HTTP endpoint, например `127.0.0.1:6060` |
| `--username <NAME>` | пусто | Логин для SOCKS5 auth |
| `--password <PASS>` | пусто | Пароль для SOCKS5 auth |
| `--verbose` | `off` | Подробные логи |
| `--buf-kb <KB>` | `256` | Размер буфера сокета |
| `--pool-size <N>` | `4` | Размер пула заранее открытых WebSocket соединений |
| `--pool-max-age <DURATION>` | `55s` | Максимальный возраст idle WebSocket в пуле |
| `--pool-refill-delay <DURATION>` | `250ms` | Пауза между созданием новых WebSocket соединений |
| `--dial-timeout <DURATION>` | `10s` | Таймаут подключения |
| `--init-timeout <DURATION>` | `15s` | Таймаут первого ответа клиента |
| `--dc-ip <DC:IP>` | DC2 + DC4 | IP для конкретного Telegram DC, можно повторять |
| `--cf-proxy` | `off` | Включить Cloudflare route |
| `--cf-proxy-first` | `off` | Сначала пробовать Cloudflare, потом direct route |
| `--cf-balance` | `off` | Балансировать трафик между несколькими Cloudflare доменами с сохранением fallback-порядка |
| `--cf-domain <DOMAIN[,DOMAIN2]>` | пусто | Домен или список доменов для Cloudflare route |
| `--secret <HEX>` | пусто | MTProto secret, нужен для `--mode mtproto` |
| `--link-ip <IP>` | пусто | IP, который попадет в `tg://proxy` ссылку |
| `--mtproto-proxy <HOST:PORT:SECRET>` | пусто | Внешний MTProto proxy как fallback, можно повторять |

### Examples

```bash
# SOCKS5 только для этой машины
./tg-ws-proxy --mode socks5 --host 127.0.0.1 --port 1080

# SOCKS5 для устройств в локальной сети
./tg-ws-proxy --mode socks5 --host 0.0.0.0 --port 1080

# SOCKS5 с логином и паролем
./tg-ws-proxy --mode socks5 --host 0.0.0.0 --port 1080 --username alice --password secret

# SOCKS5 с Cloudflare route
./tg-ws-proxy --mode socks5 --cf-proxy --cf-domain yourdomain.com

# Включить pprof на localhost:6060
./tg-ws-proxy --mode socks5 --pprof-addr 127.0.0.1:6060

# MTProto
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 --link-ip 1.2.3.4

# MTProto с внешним MTProto proxy как fallback
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 \
  --mtproto-proxy proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f
```

## Menu Script

Основные команды:

```bash
sh tg-ws-proxy-go.sh install
sh tg-ws-proxy-go.sh update
sh tg-ws-proxy-go.sh start
sh tg-ws-proxy-go.sh start-background
sh tg-ws-proxy-go.sh stop
sh tg-ws-proxy-go.sh restart
sh tg-ws-proxy-go.sh enable-autostart
sh tg-ws-proxy-go.sh disable-autostart
sh tg-ws-proxy-go.sh status
sh tg-ws-proxy-go.sh telegram
sh tg-ws-proxy-go.sh quick
sh tg-ws-proxy-go.sh remove
sh tg-ws-proxy-go.sh help
```

Если установлена короткая команда `tgm`, можно использовать ее:

```bash
tgm status
tgm stop
tgm remove
```

### Menu map

Главное меню:

- `1) Setup / Update` - скачать или обновить бинарник
- `2) Start proxy` / `2) Stop proxy` - запустить или остановить прокси
- `3) Enable autostart` / `3) Disable autostart` - включить или выключить автозапуск
- `4) Advanced` - расширенные настройки

Advanced:

- `Full status` - полный статус установки
- `Proxy settings` - текущие настройки прокси
- `Quick commands` - готовые команды для скрипта
- `Toggle verbose` - подробные логи
- `Restart proxy` - перезапуск с текущими настройками
- `Toggle proxy` / `Toggle order` - Cloudflare route и порядок попыток
- `Set domain` / `Check domain` - домен Cloudflare и проверка `kws1..kws5`, `kws203`
- `SOCKS5 auth` - логин и пароль
- `DC mapping` - ручная карта `DC:IP`
- `Port` / `Pool size` / `Public IP` - порт, пул соединений, IP для ссылки
- `Show QR code` - QR-код для подключения
- `Update source` - `release/latest`, конкретный тег или preview-ветка
- `Remove binary` - удаление установки
- `Mode` / `Secret` / `Upstream proxies` - MTProto-режим

### Как переключиться на тестовую ветку

Если нужно получать не обычные релизы, а тестовые обновления из ветки:

1. Откройте меню `tgm`
2. Перейдите в `Advanced`
3. Выберите `Update source`
4. Выберите режим `preview`
5. Укажите имя тестовой ветки, например `dev` или другую ветку, которую вам дали
6. После сохранения вернитесь в главное меню и запустите `Setup / Update`

После этого менеджер будет обновляться из выбранной preview-ветки, а не из обычных release-тегов.

Если нужно вернуться обратно на стабильные релизы:

1. Снова откройте `Advanced -> Update source`
2. Выберите режим `release`
3. Оставьте `latest` или укажите конкретный тег
4. Запустите `Setup / Update`

Если у вас уже включен автозапуск, источник обновлений тоже будет сохранён.

## Cloudflare Proxy

Cloudflare route нужен, если прямой путь до Telegram нестабилен.
Для работы нужен свой домен с поддержкой WebSocket.

Полная инструкция по настройке домена: [CF-proxy](https://github.com/Flowseal/tg-ws-proxy/blob/main/docs/CfProxy.md)

В меню:

1. `Advanced`
2. `6) Toggle proxy` -> включить Cloudflare route
3. `8) Set domain` -> указать домен или список доменов через запятую
4. `9) Check domain` -> проверить доступность `kws1..kws5` и `kws203`

Порядок маршрутов:

| Order | Meaning |
|---|---|
| `fallback` | сначала direct route, потом Cloudflare |
| `first` | сначала Cloudflare, потом direct route |

Через переменные окружения:

```bash
CF_PROXY=1 CF_DOMAIN=yourdomain.com tgm start
CF_PROXY=1 CF_PROXY_FIRST=1 CF_DOMAIN=yourdomain.com tgm start
```

Прямой запуск бинарника:

```bash
./tg-ws-proxy --cf-proxy --cf-domain yourdomain.com
./tg-ws-proxy --cf-proxy --cf-proxy-first --cf-domain yourdomain.com
```

`--cf-domain` сам по себе не включает Cloudflare route. Нужен `--cf-proxy`.

## MTProto Proxy

MTProto mode позволяет подключать Telegram напрямую как к MTProto-прокси, без
SOCKS5.

В меню:

1. `Advanced`
2. `18) Mode` -> `mtproto`
3. `19) Secret` -> сгенерировать secret или вставить свой
4. `14) Public IP` -> указать IP для ссылки подключения

После этого прокси покажет `tg://proxy?...` ссылку. Ее можно открыть в Telegram
или вывести QR-код через `15) Show QR code`.

Через переменные окружения:

```bash
PROXY_MODE=mtproto MT_SECRET=dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 MT_LINK_IP=1.2.3.4 tgm start
```

Прямой запуск бинарника:

```bash
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 --link-ip 1.2.3.4
```

`--link-ip` влияет только на ссылку подключения. На маршрутизацию он не влияет.

### Upstream MTProto proxy

Если WebSocket до Telegram недоступен и нет своего Cloudflare-домена, можно
указать внешний MTProto proxy как запасной маршрут.

```bash
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 \
  --mtproto-proxy proxy1.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f \
  --mtproto-proxy proxy2.example.com:443:dda0b1c2d3e4f5061728394a5b6c7d8e9f
```

SECRET в `--mtproto-proxy` - это secret внешнего прокси, а не вашего сервера.

## OpenWrt Builds

Целевые OpenWrt сборки:

```bash
GOOS=linux GOARCH=mipsle GOMIPS=softfloat
GOOS=linux GOARCH=mips GOMIPS=softfloat
GOOS=linux GOARCH=arm GOARM=7
GOOS=linux GOARCH=arm64
GOOS=linux GOARCH=amd64
```

Скрипт перед установкой проверяет архитектуру, свободное место в `/tmp` и
доступность релиза.

## Removal

```bash
tgm remove
```

Или через меню: `Advanced -> Remove binary`.

Команда останавливает прокси, отключает автозапуск и удаляет:

- бинарник и файл версии из `/tmp`
- постоянную копию для автозапуска
- команду `tgm`
- `init.d` сервис
- скрипт управления
- PID-файл и директорию состояния

## License

[MIT License](LICENSE)

---

Данный репозиторий создан исключительно как пример и для ознакомительных целей.
Автор не несет ответственности за использование проекта, его настройку, запуск и
возможные последствия. Вся ответственность за такие действия лежит на пользователе.
