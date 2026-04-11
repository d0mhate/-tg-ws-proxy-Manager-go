# TG WS Proxy Go

[![CI](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml)
![Go Version](https://img.shields.io/badge/go-1.22-00ADD8)
![License](https://img.shields.io/badge/license-MIT-green)
![OpenWrt](https://img.shields.io/badge/OpenWrt-mipsel__24kc%20%7C%20mips__24kc%20%7C%20armv7%20%7C%20aarch64%20%7C%20x86__64-blue)

> [!IMPORTANT]
> - Данный способ **не гарантирует 100% работу** !!!
> - Все действия вы выполняете **на свой страх и риск**
> - Автор не несёт ответственности за возможные проблемы в работе роутера, или сети

> [!WARNING]
> - Этот вариант сделан для OpenWrt и проверен на `mipsel_24kc`
> - Manager script автоматически выбирает release asset для `mipsel_24kc`, `mips_24kc`, `armv7`, `aarch64` и `x86_64`
> - На других архитектурах или сборках OpenWrt бинарник может не подойти
> - Перед установкой script проверяет архитектуру, свободное место в `/tmp` и доступность release

Лёгкая Go версия `tg-ws-proxy` для OpenWrt без Python runtime и desktop-обвязки.

- это локальный `SOCKS5` прокси для Telegram
- он пытается вести трафик через `TLS + WebSocket`
- если не получается, уходит в обычный `TCP fallback`
- текущий OpenWrt binary весит примерно `5 MB`

Проект появился как более компактная альтернатива [StressOzz/tg-ws-proxy-Manager](https://github.com/StressOzz/tg-ws-proxy-Manager) для маленьких OpenWrt storage.

## Содержание

- [Быстрый старт на роутере](#быстрый-старт-на-роутере)
- [Выбор релизной версии](#выбор-релизной-версии)
- [Настройки Telegram](#настройки-telegram)
- [Telegram DC Mapping](#telegram-dc-mapping)
- [Основные команды](#основные-команды)
- [Удаление](#удаление)
- [Локальный запуск](#локальный-запуск)
- [Тесты](#тесты)

## Быстрый старт на роутере

Подключитесь по SSH к роутеру и запустите:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest/download/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh
```

Дальше в меню обычно хватает трёх действий:

1. `Setup / Update`
2. запустить прокси через пункт `Run proxy in terminal`
3. включить автозапуск через пункт `Enable autostart`, если нужен запуск после перезагрузки

`Enable autostart` сам:

- создаёт persistent copy, если её ещё нет
- включает `init.d` сервис
- сразу пытается его запустить
- синкает текущие параметры запуска

Если в постоянном хранилище роутера не хватит места, автозагрузка не включится и script напишет причину.

Если persistent storage для автозапуска не хватает, можно просто запустить прокси в фоне:

4. `Start in background`

Если нужен `SOCKS5` логин/пароль:

5. `Advanced`
6. `Configure SOCKS5 auth`

Без меню:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest/download/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh install && sh /tmp/tg-ws-proxy-go.sh start
```

Во время `start` прокси работает в foreground, логи идут прямо в терминал, остановка через `Ctrl+C`.

Для запуска в фоне без логов в текущей SSH-сессии:

```bash
sh /tmp/tg-ws-proxy-go.sh start-background
```

После `6) Start in background` прокси запускается в фоне.

Чтобы остановить его потом:

- снова открыть `tgm` и выбрать `2) Stop proxy`
- или командой `tgm stop`

Script создаёт короткий launcher `tgm`. Обычно это `/usr/bin/tgm`, если туда нельзя писать, будет fallback в `/tmp/tgm`.

## Выбор релизной версии

По умолчанию manager обновляется на `latest release`.

Если нужно зафиксироваться на конкретном стабильном теге:

1. `Advanced`
2. `Configure update source`
3. выбрать `release`
4. выбрать `latest` или один из доступных тегов
5. вернуться в главное меню и проверить строку `track`
6. выполнить `Setup / Update`

В меню доступны только release tags, начиная с `v1.1.29`.

После выбора конкретного тега в главном меню строка `track` будет выглядеть так:

- `release/latest`
- `release/v1.1.29`

## Настройки Telegram

Если прокси запущен на роутере:

- тип: `SOCKS5`
- host: `IP роутера`
- port: `1080`
- username: пусто, если auth не включена
- password: пусто, если auth не включена

Если запускаете локально на той же машине:

- тип: `SOCKS5`
- host: `127.0.0.1`
- port: `1080`
- username: пусто, если auth не включена
- password: пусто, если auth не включена

Если в manager включены `SOCKS5` credentials, в Telegram нужно указать те же `username/password`.

## Telegram DC Mapping

Если нужно вручную переопределить target IP для Telegram DC, это можно сделать в manager:

1. `Advanced`
2. `Configure Telegram DC mapping`

Формат ввода:

- `DC:IP` через запятую
- пример: `203:91.105.192.100, 2:149.154.167.220`

Эта настройка нужна не для обычной установки, а для проблемных сетей и отдельных кейсов маршрутизации, когда требуется проверить другой target IP для конкретного Telegram DC.

Чтобы вернуть стандартное поведение, в этом же пункте можно ввести:

- `default`

## Основные команды

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
sh tg-ws-proxy-go.sh remove
```

Если автозагрузка уже включена, для обновления обычно достаточно:

```bash
tgm update
```

`disable-autostart` выключает автозапуск и удаляет persistent copy, которую script создавал для него.

## Удаление

```bash
tgm remove
```

или через меню: **5) Remove binary and runtime files**

Команда останавливает прокси, отключает автозапуск и удаляет все установленные файлы:

- бинарник и version file из `/tmp`
- persistent copy (если была создана для автозапуска)
- launcher `tgm`
- init.d сервис
- manager script
- PID файл и state директорию

После этого от установки ничего не остаётся.

## Локальный запуск

Сборка:

```bash
go build ./cmd/tg-ws-proxy
```

Запуск:

```bash
./tg-ws-proxy --host 127.0.0.1 --port 1080 --verbose
```

Запуск с `SOCKS5 auth`:

```bash
./tg-ws-proxy --host 127.0.0.1 --port 1080 --username alice --password secret --verbose
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
- кросс-сборка OpenWrt binaries для поддержанных архитектур

## Основа проекта

Сейчас это Go-only версия вокруг минимального proxy core на базе [tg-ws-proxy](https://github.com/Flowseal/tg-ws-proxy) от [Flowseal](https://github.com/Flowseal)

## Благодарности

- `tg-ws-proxy` by [Flowseal](https://github.com/Flowseal)
- [StressOzz](https://github.com/StressOzz)

## Лицензия

[MIT License](LICENSE)

---

*принадлежит компании Meta, признанной экстремистской и запрещённой на территории РФ*

Данный репозиторий создан исключительно как пример и для ознакомительных целей, автор не несёт ответственности за использование проекта, его настройку, запуск и возможные последствия, а вся ответственность за такие действия лежит на пользователе.
