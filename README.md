# TG WS Proxy Go

[![CI](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml)
![Go Version](https://img.shields.io/badge/go-1.22-00ADD8)
![License](https://img.shields.io/badge/license-MIT-green)
![OpenWrt](https://img.shields.io/badge/OpenWrt-ramips%2Fmt7621-blue)

> [!IMPORTANT]
> Данный способ **не гарантирует 100% работу** !!!
> Все действия вы выполняете **на свой страх и риск**
> Автор не несёт ответственности за возможные проблемы в работе роутера, или сети

Это Go версия `tg-ws-proxy` без Python GUI и без desktop обвязки

Проект делался под OpenWrt роутер чтобы убрать тяжёлый Python runtime и оставить только нужное proxy ядро

Если коротко то это локальный `SOCKS5` прокси для Telegram который пытается гнать трафик через `TLS + WebSocket` и при неудаче уходит в обычный `TCP`

## Что умеет

- поднимает локальный `SOCKS5` прокси
- распознаёт Telegram трафик
- достаёт `DC` из `MTProto init`
- пытается перевести Telegram трафик в `TLS + WebSocket`
- если не вышло идёт в прямой `TCP fallback`
- умеет `WS pool`
- умеет `cooldown` и `blacklist`
- пишет runtime stats

## Текущее состояние

Сейчас проект уже рабочий

- есть сборка под `OpenWrt`
- есть `GitHub Actions` для тестов и сборки

Целевой роутер под который всё и делалось

- `Xiaomi Mi Router 4A Gigabit Edition v2`
- `OpenWrt 24.10.5`
- `ramips/mt7621`
- `mipsel_24kc`

Целевая сборка такая

```bash
GOOS=linux GOARCH=mipsle GOMIPS=softfloat
```

## Как это работает

```text
Telegram client -> SOCKS5 -> tg-ws-proxy -> WSS or TCP -> Telegram DC
```

Если запускать бинарник руками локально то он по умолчанию слушает `127.0.0.1:1080`

Если запускать через `tg-ws-proxy-go.sh` на роутере то дефолт уже `0.0.0.0:1080`

## Быстрый старт на роутере

Подключитесь по SSH к роутеру и выполните команду

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://raw.githubusercontent.com/d0mhate/-tg-ws-proxy-Manager-go/main/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh
```

Дальше в меню просто выберите `Install or update binary`

Если хотите без меню то так

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://raw.githubusercontent.com/d0mhate/-tg-ws-proxy-Manager-go/main/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh install && sh /tmp/tg-ws-proxy-go.sh start
```

Во время `start` прокси работает в foreground

Логи идут прямо в терминал

Остановить можно через `Ctrl+C`

`install` сначала ищет локальный binary в `/tmp/tg-ws-proxy-openwrt`

Если файла нет то script сам пробует скачать готовый OpenWrt binary из `latest release`

## Быстрый старт локально

Если хотите сначала проверить всё на своей машине то соберите бинарник

```bash
go build ./cmd/tg-ws-proxy
```

Потом запустите

```bash
./tg-ws-proxy --host 127.0.0.1 --port 1080 --verbose
```

В Telegram укажите

- тип `SOCKS5`
- host `127.0.0.1`
- port `1080`
- username пусто
- password пусто

Это только для локальной проверки на той же машине где запущен прокси

Если прокси крутится на роутере то в Telegram нужно указывать уже IP роутера в локальной сети

- тип `SOCKS5`
- host `IP роутера`
- port `1080`
- username пусто
- password пусто

## Что делает tg-ws-proxy-go.sh

`tg-ws-proxy-go.sh` это простой manager script для роутера

По умолчанию он поднимает прокси на `0.0.0.0:1080`

Он

- берёт локальный binary из `/tmp/tg-ws-proxy-openwrt`
- если локального файла нет, скачивает binary из GitHub Release
- копирует его в `/tmp/tg-ws-proxy-go/tg-ws-proxy`
- умеет `install`
- умеет `start`
- умеет `stop`
- умеет `restart`
- умеет показывать настройки для Telegram

Основные команды такие

```bash
sh tg-ws-proxy-go.sh install
```

```bash
sh tg-ws-proxy-go.sh start
```

```bash
sh tg-ws-proxy-go.sh stop
```

```bash
sh tg-ws-proxy-go.sh restart
```

```bash
sh tg-ws-proxy-go.sh status
```

```bash
sh tg-ws-proxy-go.sh quick
```

```bash
sh tg-ws-proxy-go.sh telegram
```

```bash
sh tg-ws-proxy-go.sh remove
```

## Флаги CLI

| flag | default | meaning |
|---|---|---|
| `--host` | `127.0.0.1` | где слушать `SOCKS5` |
| `--port` | `1080` | порт `SOCKS5` |
| `--verbose` | `false` | подробные логи |
| `--buf-kb` | `256` | socket buffer в KB |
| `--pool-size` | `1` | idle `WS` connections на bucket |
| `--dial-timeout` | `10s` | timeout TCP dial |
| `--init-timeout` | `15s` | timeout чтения `MTProto init` |
| `--dc-ip` | built-in defaults | override target IP for DC |

Пример ручного запуска

```bash
./tg-ws-proxy --host 0.0.0.0 --port 1080 --verbose
```

## Тесты

Локально запускаются так

```bash
go test ./...
```

На GitHub уже есть CI

- `go test ./...`
- `go build ./cmd/tg-ws-proxy`
- кросс сборка `linux/mipsle`

## Откуда это взялось

Раньше это был Python desktop проект

Сейчас репозиторий это Go only версия вокруг минимального proxy core для роутера на базе [tg-ws-proxy](https://github.com/Flowseal/tg-ws-proxy) от [Flowseal](https://github.com/Flowseal)

## Лицензия

[MIT License](LICENSE)
