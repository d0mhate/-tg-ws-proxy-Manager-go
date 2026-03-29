# TG WS Proxy Go

[![CI](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml)
![Codecov](https://codecov.io/github/d0mhate/-tg-ws-proxy-Manager-go/graph/badge.svg)
![Go Version](https://img.shields.io/badge/go-1.22-00ADD8)
![License](https://img.shields.io/badge/license-MIT-green)
![OpenWrt](https://img.shields.io/badge/OpenWrt-mipsel__24kc%20%7C%20mips__24kc%20%7C%20armv7%20%7C%20aarch64%20%7C%20x86__64-blue)

> [!IMPORTANT]
> Данный способ **не гарантирует 100% работу** !!!
> Все действия вы выполняете **на свой страх и риск**
> Автор не несёт ответственности за возможные проблемы в работе роутера, или сети

> [!WARNING]
> Этот вариант сделан для OpenWrt и проверен на `mipsel_24kc`
> Manager script автоматически выбирает release asset для `mipsel_24kc`, `mips_24kc`, `armv7`, `aarch64` и `x86_64`
> На других архитектурах или сборках OpenWrt бинарник может не подойти
> Перед установкой script проверяет архитектуру, свободное место в `/tmp` и доступность release

Лёгкая Go версия `tg-ws-proxy` для OpenWrt без Python runtime и desktop-обвязки.

- это локальный `SOCKS5` прокси для Telegram
- он пытается вести трафик через `TLS + WebSocket`
- если не получается, уходит в обычный `TCP fallback`
- текущий OpenWrt binary весит примерно `5 MB`

Проект появился как более компактная альтернатива [StressOzz/tg-ws-proxy-Manager](https://github.com/StressOzz/tg-ws-proxy-Manager) для маленьких OpenWrt storage.

## Быстрый старт на роутере

Подключитесь по SSH к роутеру и запустите:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://raw.githubusercontent.com/d0mhate/-tg-ws-proxy-Manager-go/main/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh
```

Дальше в меню обычно хватает трёх действий:

1. `Setup / Update`
2. `Start proxy`
3. `Enable autostart`, если нужен запуск после перезагрузки

`Enable autostart` сам:

- создаёт persistent copy, если её ещё нет
- включает `init.d` сервис
- синкает текущие параметры запуска

Если в постоянном хранилище роутера не хватит места, автозагрузка не включится и script напишет причину.

Без меню:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://raw.githubusercontent.com/d0mhate/-tg-ws-proxy-Manager-go/main/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh install && sh /tmp/tg-ws-proxy-go.sh start
```

Во время `start` прокси работает в foreground, логи идут прямо в терминал, остановка через `Ctrl+C`.

Script создаёт короткий launcher `tgm`. Обычно это `/usr/bin/tgm`, если туда нельзя писать, будет fallback в `/tmp/tgm`.

## Настройки Telegram

Если прокси запущен на роутере:

- тип: `SOCKS5`
- host: `IP роутера`
- port: `1080`
- username: пусто
- password: пусто

Если запускаете локально на той же машине:

- тип: `SOCKS5`
- host: `127.0.0.1`
- port: `1080`
- username: пусто
- password: пусто

## Основные команды

```bash
sh tg-ws-proxy-go.sh install
sh tg-ws-proxy-go.sh update
sh tg-ws-proxy-go.sh start
sh tg-ws-proxy-go.sh stop
sh tg-ws-proxy-go.sh restart
sh tg-ws-proxy-go.sh enable-autostart
sh tg-ws-proxy-go.sh disable-autostart
sh tg-ws-proxy-go.sh status
sh tg-ws-proxy-go.sh telegram
sh tg-ws-proxy-go.sh remove
```

Если автозагрузка уже включена, для обновления обычно достаточно:

```bash
tgm update
```

`disable-autostart` выключает автозапуск и удаляет persistent copy, которую script создавал для него.

## Локальный запуск

Сборка:

```bash
go build ./cmd/tg-ws-proxy
```

Запуск:

```bash
./tg-ws-proxy --host 127.0.0.1 --port 1080 --verbose
```

Целевые OpenWrt сборки:

```bash
GOOS=linux GOARCH=mipsle GOMIPS=softfloat
GOOS=linux GOARCH=mips GOMIPS=softfloat
GOOS=linux GOARCH=arm GOARM=7
GOOS=linux GOARCH=arm64
GOOS=linux GOARCH=amd64
```

Проверенная цель:

- `Xiaomi Mi Router 4A Gigabit Edition v2`
- `OpenWrt 24.10.5`
- `ramips/mt7621`
- `mipsel_24kc`

## Тесты

```bash
go test ./...
```

В GitHub Actions запускаются:

- `go test ./...`
- `go build ./cmd/tg-ws-proxy`
- кросс-сборка `linux/mipsle`

## Основа проекта

Сейчас это Go-only версия вокруг минимального proxy core на базе [tg-ws-proxy](https://github.com/Flowseal/tg-ws-proxy) от [Flowseal](https://github.com/Flowseal)

## Лицензия

[MIT License](LICENSE)
