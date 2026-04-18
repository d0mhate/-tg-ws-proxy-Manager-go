# TG WS Proxy Go

[![CI](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml/badge.svg)](https://github.com/d0mhate/-tg-ws-proxy-Manager-go/actions/workflows/ci.yml)
![Go Version](https://img.shields.io/badge/go-1.22-00ADD8)
![License](https://img.shields.io/badge/license-MIT-green)
![OpenWrt](https://img.shields.io/badge/OpenWrt-mipsel__24kc%20%7C%20mips__24kc%20%7C%20armv7%20%7C%20aarch64%20%7C%20x86__64-blue)
![Linux](https://img.shields.io/badge/Linux-x86__64%20%7C%20aarch64%20%7C%20armv7%20%7C%20armv6%20%7C%20386%20%7C%20riscv64%20%7C%20loong64-orange)
![macOS](https://img.shields.io/badge/macOS-amd64%20%7C%20arm64-lightgrey)
![Windows](https://img.shields.io/badge/Windows-amd64%20%7C%20arm64-0078D6)
![FreeBSD](https://img.shields.io/badge/FreeBSD-amd64-red)

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

- это локальный `SOCKS5` или `MTProto` прокси для TG
- он пытается вести трафик через `TLS + websocket`
- если не получается, уходит в обычный `TCP fallback`
- текущий OpenWrt binary весит примерно `5 MB`

Проект появился как более компактная альтернатива [StressOzz/tg-ws-proxy-Manager](https://github.com/StressOzz/tg-ws-proxy-Manager) для маленьких OpenWrt storage.

## Содержание

- [Быстрый старт на роутере](#быстрый-старт-на-роутере)
- [Карта меню](#карта-меню)
- [Выбор релизной версии](#выбор-релизной-версии)
- [Настройки TG](#настройки-tg)
- [TG DC Mapping](#tg-dc-mapping)
- [CF(cloudflare) proxy](#cfcloudflare-proxy)
- [MTProto proxy](#mtproto-proxy)
- [Флаги CLI](#флаги-cli)
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

1. `Setup / Update` - подтвердить `y`
2. запустить прокси через пункт `Start proxy` → выбрать `t` (terminal) или `b` (background)
3. включить автозапуск через пункт `Enable autostart`, если нужен запуск после перезагрузки

`Enable autostart` сам:

- создаёт persistent copy, если её ещё нет
- включает `init.d` сервис
- сразу пытается его запустить
- синкает текущие параметры запуска

Если в постоянном хранилище роутера не хватит места, автозагрузка не включится и script напишет причину.

Если persistent storage для автозапуска не хватает, можно просто запустить прокси в фоне через `Start proxy` → `b` (background).

Если нужен `SOCKS5` логин/пароль: `Advanced` → `SOCKS5 auth`.

Если нужен `MTProto` режим:

1. `Advanced`
2. `17) Mode` → выбрать `mtproto`
3. `18) Secret` → сгенерировать случайный secret или вставить свой hex
4. `13) Public IP` → ввести публичный IP сервера

Без меню:

```bash
wget -O /tmp/tg-ws-proxy-go.sh https://github.com/d0mhate/-tg-ws-proxy-Manager-go/releases/latest/download/tg-ws-proxy-go.sh && sh /tmp/tg-ws-proxy-go.sh install && sh /tmp/tg-ws-proxy-go.sh start
```

Во время `start` прокси работает в foreground, логи идут прямо в терминал, остановка через `Ctrl+C`.

Для запуска в фоне без логов в текущей SSH-сессии:

```bash
sh /tmp/tg-ws-proxy-go.sh start-background
```

После выбора `b` в пункте `Start proxy` прокси запускается в фоне.

Чтобы остановить его потом:

- снова открыть `tgm` и выбрать `2) Stop proxy`
- или командой `tgm stop`

Script создаёт короткий launcher `tgm`. Обычно это `/usr/bin/tgm`, если туда нельзя писать, будет fallback в `/tmp/tgm`.

## Карта меню

Ниже перечислены все текущие пункты главного menu и `Advanced`, чтобы было понятно, где что находится.

### Главное menu

- `1) Setup / Update` - скачать или обновить бинарник
- `2) Start proxy` - если прокси не запущен, предложит запуск:
  - `t` - запуск в terminal с логами в текущей сессии
  - `b` - запуск в background
- `2) Stop proxy` - если прокси уже запущен, остановить его
- `3) Enable autostart` - включить автозапуск на OpenWrt
- `3) Disable autostart` - выключить автозапуск и убрать persistent copy
- `4) Advanced` - открыть расширенные настройки

### Advanced -> Info

- `1) Full status` - полный статус бинарника, процесса, автозапуска и окружения
- `2) Proxy settings` - краткий экран с текущими TG/SOCKS5/MTProto настройками
- `3) Quick commands` - список готовых CLI-команд для manager script

### Advanced -> Proxy

- `4) Toggle verbose` - включить или выключить подробные логи
- `5) Restart proxy` - перезапустить уже запущенный прокси с текущими настройками

### Advanced -> CF(cloudflare)

- `6) Toggle proxy` - включить или выключить CF proxy
- `7) Toggle order` - выбрать порядок `fallback` или `first`
- `8) Set domain` - сохранить один домен или список доменов через запятую
- `9) Check domain` - проверить websocket upgrade для `kws1..kws5` и `kws203`

### Advanced -> Settings

- `10) SOCKS5 auth` - задать или очистить логин/пароль для SOCKS5 режима
- `11) DC mapping` - задать кастомную карту `DC:IP` или вернуть `default`
- `12) Port` - сменить listen port прокси
- `13) Public IP` - указать IP для `tg://proxy` или `tg://socks` ссылки
- `14) Show QR code` - показать QR-код для ссылки подключения (mtproto или socks5)
- `15) Update source` - выбрать `release/latest`, конкретный release tag или preview branch
- `16) Remove binary` - удалить установленный бинарник, launcher и связанные файлы

### Advanced -> MTProto

- `17) Mode` - переключить режим между `socks5` и `mtproto`
- `18) Secret` - сгенерировать случайный secret или вставить свой вручную
- `19) Upstream proxies` - добавить, удалить или очистить список `HOST:PORT:SECRET` для upstream MTProto fallback

## Выбор релизной версии

По умолчанию manager обновляется на `latest release`.

Если нужно зафиксироваться на конкретном стабильном теге:

1. `Advanced`
2. `Update source`
3. выбрать `release`
4. выбрать `latest` или один из доступных тегов
5. вернуться в главное меню и проверить строку `track`
6. выполнить `Setup / Update` → подтвердить `y`

В меню доступны только release tags, начиная с `v1.1.29`.

После выбора конкретного тега в главном меню строка `track` будет выглядеть так:

- `release/latest`
- `release/v1.1.29`

## Настройки TG

### Режим SOCKS5 (по умолчанию)

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

Если в manager включены `SOCKS5` credentials, в TG нужно указать те же `username/password`.

### Режим MTProto

- тип: `MTProto`
- server: `IP роутера` (или `127.0.0.1` при локальном запуске)
- port: `1080`
- secret: значение из `Advanced → Secret`

Либо использовать готовую ссылку `tg://proxy?server=...&port=...&secret=...` - она отображается в меню после задания Secret и Public IP, и открывает TG с уже заполненными полями.

## TG DC Mapping

Если нужно вручную переопределить target IP для TG DC, это можно сделать в manager:

1. `Advanced`
2. `DC mapping`

Формат ввода:

- `DC:IP` через запятую
- пример: `203:91.105.192.100, 2:149.154.167.220`

Эта настройка нужна не для обычной установки, а для проблемных сетей и отдельных кейсов маршрутизации, когда требуется проверить другой target IP для конкретного TG DC.

Чтобы вернуть стандартное поведение, в этом же пункте можно ввести:

- `default`

## CF(cloudflare) proxy

How to configure your own cf proxy? read this -> [CF-proxy](https://github.com/Flowseal/tg-ws-proxy/blob/main/docs/CfProxy.md)

CF(cloudflare) proxy - дополнительный режим маршрутизации, при котором трафик до TG идёт через websocket-соединение к домену, защищённому CF(cloudflare). Помогает в сетях, где прямые соединения с серверами TG нестабильны.

По умолчанию режим **выключен**.

### Включение и режим работы

В меню доступны два переключателя:

- **`6) Toggle proxy`** - включает или выключает CF proxy. Когда включён, прокси использует CF(cloudflare) как дополнительный маршрут.
- **`7) Toggle order`** - меняет порядок попыток:
  - `fallback` (по умолчанию) - сначала пробует прямое соединение с TG, при неудаче переходит на CF
  - `first` - сначала пробует CF, прямое соединение используется как запасной вариант

Текущее состояние обоих переключателей видно в шапке главного меню:

```text
  CF      on / first / domain:yourdomain.com
```

### Домен

Для работы CF proxy нужен домен, проксируемый через CF(cloudflare) с поддержкой websocket. Встроенного домена нет - нужно указать свой.

- **`8) Set domain`** - задать свой домен или список доменов через запятую. Ввести `clear` - удалить домен.
- **`9) Check domain`** - проверить домен: manager выполнит websocket upgrade к `kws1-kws5` и `kws203` поддоменам и покажет, какие из них отвечают. Прервать проверку можно через `Ctrl+C`.

Важно:

- сохранение `CF domain` само по себе не включает CF-маршрут
- если `CF proxy` выключен, меню покажет явное предупреждение, что домен сохранён, но CF routing не активен
- чтобы реально использовать CF-маршрут, нужно включить `Toggle proxy`

Зарегистрируйте свой домен, добавьте его в CF(cloudflare) и укажите через пункт `8) Set domain`.

### Запуск без меню

Через env-переменные:

```bash
CF_PROXY=1 tgm start
CF_PROXY=1 CF_DOMAIN=yourdomain.com tgm start
CF_PROXY=1 CF_PROXY_FIRST=1 CF_DOMAIN=yourdomain.com tgm start
```

Флаги бинарника при прямом запуске:

```bash
./tg-ws-proxy --cf-proxy --cf-domain yourdomain.com
./tg-ws-proxy --cf-proxy --cf-proxy-first --cf-domain yourdomain.com
```

Если передать только `--cf-domain`, но не включить `--cf-proxy`, прокси сохранит домен в конфиге, но CF path использовать не будет.

## MTProto proxy

MTProto proxy - встроенный режим, при котором клиент TG подключается напрямую как к официальному MTProto-прокси, без SOCKS5. Трафик остаётся зашифрован обфускацией MTProto.

По умолчанию режим **выключен** (используется SOCKS5).

### Форматы секрета

Secret определяет режим шифрования между клиентом и этим сервером. Поддерживаются три формата:

| Формат | Длина | Пример prefix | Режим |
| ------ | ----- | ------------- | ----- |
| Plain | 32 hex (16 байт) | `a1b2c3...` | Стандартный MTProto obfuscation |
| `dd` prefix | 34 hex (17 байт) | `dd...` | Padded intermediate - рекомендуется |
| `ee` prefix | 34+ hex | `ee...` + hostname в hex | FakeTLS - трафик выглядит как TLS |

Secret берётся из ссылки `tg://proxy?...&secret=<ЗДЕСЬ>` полностью, включая префикс `dd` или `ee`.

Для FakeTLS (`ee`): после двух байт префикса идут 32 hex (16 байт ключа), затем hostname в UTF-8 hex. Например, для ключа `deadbeef...` и домена `example.com`:

```text
ee + deadbeef0102030405060708090a0b0c0d0e0f + 6578616d706c652e636f6d
    ^---- 32 hex (16 байт ключ) ----^         ^--- "example.com" в hex ---^
```

Hostname в FakeTLS - это SNI для фейкового TLS handshake. TG видит обычное TLS соединение к этому домену.

Сгенерировать секреты:

```bash
# Plain - 32 hex
openssl rand -hex 16

# dd-prefix - padded intermediate (рекомендуется для обычного MTProto proxy)
echo "dd$(openssl rand -hex 16)"

# ee-prefix - FakeTLS с доменом example.com
SECRET=$(openssl rand -hex 16)
DOMAIN_HEX=$(printf '%s' 'example.com' | xxd -p | tr -d '\n')
echo "ee${SECRET}${DOMAIN_HEX}"
```

### Настройка MTProto в меню

1. `Advanced`
2. `17) Mode` → выбрать `mtproto`
3. `18) Secret` → можно сгенерировать случайный secret или вставить свой (32/34+ hex)
4. `13) Public IP` → ввести публичный IP этого сервера

После заполнения Secret и Public IP в строке настроек главного меню появится `tg://proxy?...` ссылка. QR-код доступен через `14) Show QR code` как только задан Public IP.

### Запуск MTProto без меню

Через env-переменные:

```bash
# Plain secret
PROXY_MODE=mtproto MT_SECRET=a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 tgm start

# dd-prefix secret с публичным IP (выводит tg:// ссылку при старте)
PROXY_MODE=mtproto MT_SECRET=dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 MT_LINK_IP=1.2.3.4 tgm start

# ee-prefix FakeTLS secret
PROXY_MODE=mtproto MT_SECRET=eea1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d46578616d706c652e636f6d MT_LINK_IP=1.2.3.4 tgm start
```

Флаги бинарника при прямом запуске:

```bash
# Plain secret, слушать на всех интерфейсах, порт 1080
./tg-ws-proxy --mode mtproto --secret a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080

# dd-prefix secret с выводом tg:// ссылки
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 --link-ip 1.2.3.4

# ee-prefix FakeTLS secret
./tg-ws-proxy --mode mtproto --secret eea1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d46578616d706c652e636f6d --port 1080 --link-ip 1.2.3.4

# Verbose - логировать каждое подключение
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 --verbose
```

`--link-ip` - публичный IP, включается в `tg://` ссылку которую сервер выводит при старте. На маршрутизацию не влияет.

При старте с `--link-ip` сервер выводит в лог готовую ссылку:

```text
tg://proxy?server=1.2.3.4&port=1080&secret=dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4
```

Эту ссылку можно открыть в TG или отсканировать QR-кодом - TG заполнит поля прокси автоматически.

### Upstream MTProto proxy (--mtproto-proxy)

Если WebSocket до TG недоступен (например, заблокирован провайдером) и нет своего CF(cloudflare) домена, можно указать внешний MTProto прокси как fallback. Сервер сначала пробует прямое WebSocket соединение с TG, и только при неудаче подключается к upstream прокси.

Флаг `--mtproto-proxy` принимает строку формата `HOST:PORT:SECRET` и может повторяться - upstream'ы перебираются по порядку.

```bash
# Один upstream proxy (plain secret)
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 \
  --mtproto-proxy proxy.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f

# Два upstream proxy - перебираются по порядку при недоступности первого
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 \
  --mtproto-proxy proxy1.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f \
  --mtproto-proxy proxy2.example.com:443:dda0b1c2d3e4f5061728394a5b6c7d8e9f

# Upstream с FakeTLS секретом (ee-prefix)
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1080 \
  --mtproto-proxy tls.example.com:443:eef0e1d2c3b4a5968778695a4b3c2d1e0f6578616d706c652e636f6d
```

SECRET в `--mtproto-proxy` - это секрет upstream прокси (тот же формат что в `tg://proxy` ссылке этого upstream), а не секрет вашего сервера.

Схема работы при включённом upstream:

```text
TG App → [ваш сервер :1080] → WebSocket → TG DC  (приоритет)
                                  ↘ upstream MTProto proxy    (fallback при недоступности WS)
```

## Флаги CLI

Ниже перечислены флаги бинарника `tg-ws-proxy`, которые доступны при прямом запуске без menu script.

### Быстрый синтаксис

```bash
./tg-ws-proxy [flags]
```

### Список флагов

| Флаг | Значение по умолчанию | Что делает |
| --- | --- | --- |
| `--mode <socks5\|mtproto>` | `socks5` | Выбирает режим работы прокси. |
| `--host <IP>` | `127.0.0.1` | Адрес, на котором слушает сервер. |
| `--port <PORT>` | `1080` | Порт, на котором слушает сервер. |
| `--username <NAME>` | пусто | Логин для SOCKS5 auth. Используется только вместе с `--password`. |
| `--password <PASS>` | пусто | Пароль для SOCKS5 auth. Используется только вместе с `--username`. |
| `--verbose` | `off` | Включает подробные логи. |
| `--buf-kb <KB>` | `256` | Размер буфера сокета в KB. |
| `--pool-size <N>` | `1` | Размер пула заранее открытых WebSocket соединений. |
| `--dial-timeout <DURATION>` | `10s` | Таймаут TCP dial до TG, CF или upstream. |
| `--init-timeout <DURATION>` | `15s` | Таймаут ожидания начального handshake от клиента. |
| `--dc-ip <DC:IP>` | встроенная карта DC | Явно задаёт IP для конкретного TG DC. Флаг можно повторять. |
| `--cf-proxy` | `off` | Включает CF(cloudflare) proxy path. |
| `--cf-proxy-first` | `off` | Сначала пробует CF path, потом direct path. Имеет смысл только вместе с `--cf-proxy`. |
| `--cf-domain <DOMAIN[,DOMAIN2]>` | пусто | Домен или список доменов для CF websocket routing. Сам по себе флаг не включает `CF proxy`. |
| `--secret <HEX>` | пусто | MTProto secret. Обязателен для `--mode mtproto`. |
| `--link-ip <IP>` | пусто | IP, который попадает в `tg://proxy` ссылку при старте. На маршрутизацию не влияет. |
| `--mtproto-proxy <HOST:PORT:SECRET>` | пусто | Upstream MTProto proxy fallback. Флаг можно повторять. |

### Что важно помнить

- `--mode mtproto` требует `--secret`
- `--username` и `--password` нужно передавать вместе
- `--cf-domain` без `--cf-proxy` не включает CF routing
- `--cf-proxy-first` без `--cf-proxy` не даёт эффекта
- plain MTProto secret из 32 hex в логах показывается как `dd...` ссылка для TG, и это штатное поведение

Примечание:

- примеры secret в README нужны только чтобы показать формат
- для реального запуска лучше всегда генерировать свой случайный secret

### Понятные примеры запуска

SOCKS5 без авторизации:

```bash
./tg-ws-proxy --mode socks5 --host 127.0.0.1 --port 1080
```

SOCKS5 с авторизацией:

```bash
./tg-ws-proxy --mode socks5 --host 127.0.0.1 --port 1080 --username alice --password secret --verbose
```

SOCKS5 с CF fallback:

```bash
./tg-ws-proxy --mode socks5 --host 127.0.0.1 --port 1080 --username alice --password secret --cf-proxy --cf-domain yourdomain.com --verbose
```

SOCKS5 с CF first:

```bash
./tg-ws-proxy --mode socks5 --host 127.0.0.1 --port 1080 --username alice --password secret --cf-proxy --cf-proxy-first --cf-domain yourdomain.com --verbose
```

MTProto с plain secret:

```bash
./tg-ws-proxy --mode mtproto --secret a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1443 --link-ip 1.2.3.4 --verbose
```

MTProto с `dd` secret:

```bash
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1443 --link-ip 1.2.3.4 --verbose
```

MTProto с `ee` FakeTLS secret:

```bash
./tg-ws-proxy --mode mtproto --secret eea1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d46578616d706c652e636f6d --port 1443 --link-ip 1.2.3.4 --verbose
```

MTProto с CF first:

```bash
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --cf-proxy --cf-proxy-first --cf-domain yourdomain.com --port 1443 --link-ip 1.2.3.4 --verbose
```

MTProto с upstream fallback:

```bash
./tg-ws-proxy --mode mtproto --secret dda1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4 --port 1443 --link-ip 1.2.3.4 \
  --mtproto-proxy proxy1.example.com:443:ddf0e1d2c3b4a5968778695a4b3c2d1e0f \
  --mtproto-proxy proxy2.example.com:443:dda0b1c2d3e4f5061728394a5b6c7d8e9f \
  --verbose
```

Локальный запуск только для текущей машины:

```bash
./tg-ws-proxy --host 127.0.0.1 --port 1080 --verbose
```

Запуск для устройств в локальной сети:

```bash
./tg-ws-proxy --host 0.0.0.0 --port 1080 --verbose
```

### Как выбирать режим

- `socks5` - самый простой вариант для TG и других приложений, которые умеют SOCKS5 proxy
- `mtproto` - режим именно для TG-клиента, когда нужен secret и `tg://proxy` ссылка
- `cf-proxy` - полезен, когда direct путь до TG нестабилен или режется провайдером
- `mtproto-proxy` - запасной маршрут, если нет своего CF(cloudflare) домена или нужен внешний upstream fallback

## Основные команды

```bash
sh tg-ws-proxy-go.sh install
sh tg-ws-proxy-go.sh update
sh tg-ws-proxy-go.sh persist
sh tg-ws-proxy-go.sh start
sh tg-ws-proxy-go.sh start-background   # или start-bg
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

Если автозагрузка уже включена, для обновления обычно достаточно:

```bash
tgm update
```

`disable-autostart` выключает автозапуск и удаляет persistent copy, которую script создавал для него.

## Удаление

```bash
tgm remove
```

или через меню: **Advanced → 16) Remove binary**

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

принадлежит компании Meta, признанной экстремистской и запрещённой на территории РФ

Данный репозиторий создан исключительно как пример и для ознакомительных целей, автор не несёт ответственности за использование проекта, его настройку, запуск и возможные последствия, а вся ответственность за такие действия лежит на пользователе.
