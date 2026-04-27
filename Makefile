SHELL := /bin/sh

BIN := tg-ws-proxy
LOCAL_BIN := ./$(BIN)
SCRIPT := tg-ws-proxy-go.sh
BUNDLE := build/tg-ws-proxy-go.sh
GO_LOCAL_CACHE := $(CURDIR)/.gocache
comma := ,
BATS ?= bats
BATS_FLAGS ?= --print-output-on-failure
BATS_VERBOSE_FLAGS ?= --print-output-on-failure --show-output-of-passing-tests

-include .env

# Support both current manager env names and older local aliases from .env.
# Make targets always run the freshly built local binary; BIN_PATH from .env
# is kept only for direct script usage outside of make.
BIN_PATH ?= $(LOCAL_BIN)
MODE ?= $(if $(PROXY_MODE),$(PROXY_MODE),socks5)
HOST ?= $(LISTEN_HOST)
PORT ?= $(if $(LISTEN_PORT),$(LISTEN_PORT),1080)
USERNAME ?= $(SOCKS_USERNAME)
PASSWORD ?= $(SOCKS_PASSWORD)
SECRET ?= $(MT_SECRET)
IP ?= $(MT_LINK_IP)
CF_FIRST ?= $(CF_PROXY_FIRST)
ifeq ($(strip $(value CF_BALANCE)),)
CF_BALANCE := 0
endif
MT_PLAIN_SECRET ?= $(MT_SECRET)
MT_DD_SECRET ?= dd$(MT_PLAIN_SECRET)
MT_EE_DOMAIN_HEX ?= 676f6f676c652e636f6d
MT_EE_SECRET ?= ee$(MT_PLAIN_SECRET)$(MT_EE_DOMAIN_HEX)
EE_GOOGLE_SECRET ?= ee$(MT_SECRET)$(shell printf '%s' "$(EE_GOOGLE_DOMAIN)" | xxd -p -c 256 | tr -d '\n')

MANAGER_ENV = \
	BIN_PATH=$(LOCAL_BIN) \
	PERSIST_STATE_DIR=$(CURDIR)/.dev-state \
	PROXY_MODE=$(MODE) \
	LISTEN_PORT=$(PORT) \
	$(if $(HOST),LISTEN_HOST=$(HOST),) \
	$(if $(SECRET),MT_SECRET=$(SECRET),) \
	$(if $(IP),MT_LINK_IP=$(IP),) \
	$(if $(PPROF_ADDR),PPROF_ADDR=$(PPROF_ADDR),) \
	$(if $(DC_IPS),DC_IPS=$(DC_IPS),) \
	$(if $(POOL_SIZE),POOL_SIZE=$(POOL_SIZE),) \
	$(if $(USERNAME),SOCKS_USERNAME=$(USERNAME),) \
	$(if $(PASSWORD),SOCKS_PASSWORD=$(PASSWORD),) \
	$(if $(CF_PROXY),CF_PROXY=$(CF_PROXY),) \
	$(if $(CF_FIRST),CF_PROXY_FIRST=$(CF_FIRST),) \
	$(if $(CF_BALANCE),CF_BALANCE=$(CF_BALANCE),) \
	$(if $(CF_DOMAIN),CF_DOMAIN=$(CF_DOMAIN),) \
	$(if $(VERBOSE),VERBOSE=$(VERBOSE),)

BIN_FLAGS = \
	--mode $(MODE) \
	--port $(PORT) \
	$(if $(HOST),--host $(HOST),) \
	$(if $(SECRET),--secret $(SECRET),) \
	$(if $(IP),--link-ip $(IP),) \
	$(if $(PPROF_ADDR),--pprof-addr $(PPROF_ADDR),) \
	$(foreach dc_ip,$(subst $(comma), ,$(strip $(DC_IPS))),--dc-ip $(dc_ip)) \
	$(if $(POOL_SIZE),--pool-size $(POOL_SIZE),) \
	$(if $(USERNAME),--username $(USERNAME) --password $(PASSWORD),) \
	$(if $(CF_PROXY),--cf-proxy,) \
	$(if $(CF_FIRST),--cf-proxy-first,) \
	$(if $(CF_BALANCE),--cf-balance,) \
	$(if $(CF_DOMAIN),--cf-domain $(CF_DOMAIN),) \
	$(if $(VERBOSE),--verbose,)

.PHONY: help build bundle menu start start-bg stop restart status run profile-pprof profile-pprof-leakcheck test test-go test-go-leak test-go-compile test-shell test-shell-verbose test-shell-ci-local test-shell-file fmt-shell lint-shell clean install-git-hooks \
	socks5-auth socks5-noauth socks5-auth-nocf socks5-noauth-nocf \
	socks5-auth-menu socks5-auth-cf-menu socks5-noauth-menu socks5-auth-nocf-menu socks5-noauth-nocf-menu \
	socks5-menu-auth-cf menu-socks5-auth-cf link-socks5-auth link-socks5-noauth \
	mtproto-plain mtproto-plain-nocf mtproto-dd mtproto-dd-nocf mtproto-ee mtproto-ee-nocf \
	mtproto-plain-menu mtproto-plain-nocf-menu mtproto-dd-menu mtproto-dd-nocf-menu mtproto-ee-menu mtproto-ee-nocf-menu \
	mtproto-plain-auth-cf-menu mtproto-plain-auth-nocf-menu mtproto-hex-auth-cf-menu mtproto-dd-auth-cf-menu mtproto-dd-auth-nocf-menu mtproto-ee-auth-cf-menu mtproto-ee-auth-nocf-menu \
	menu-mtproto-ee-cf

help:
	@printf '%s\n' \
		'make build        - build fresh local ./$(BIN)' \
		'make menu         - build fresh local ./$(BIN) and open manager menu for it' \
		'make start        - build fresh local ./$(BIN) and start it in foreground' \
		'make start-bg     - build fresh local ./$(BIN) and start it in background' \
		'make stop         - manager stop' \
		'make restart      - manager restart' \
		'make status       - manager status' \
		'make run          - run binary directly without menu' \
		'make run PPROF_ADDR=127.0.0.1:6060 - run binary with pprof enabled' \
		'make profile-pprof PPROF_ADDR=127.0.0.1:6060 STEPS=3 LABELS=baseline,load,after_restart - capture and compare pprof steps' \
		'make profile-pprof PPROF_ADDR=127.0.0.1:6060 STEPS=5 LABELS=baseline,load,cooldown,restart,after_restart_cooldown STRICT_EXIT=1 - fail on likely leak candidate' \
		'make profile-pprof-leakcheck PPROF_ADDR=127.0.0.1:6060 - run the standard 5-step leak check scenario' \
		'make socks5-auth  - start SOCKS5 with auth from .env' \
		'make socks5-auth-menu - open menu with SOCKS5 auth preset' \
		'make socks5-auth-cf-menu - open menu with SOCKS5 auth preset, CF on' \
		'make socks5-menu-auth-cf - open menu with SOCKS5 auth preset, CF first and balance on' \
		'make socks5-noauth - start SOCKS5 without auth' \
		'make socks5-noauth-menu - open menu with SOCKS5 no-auth preset' \
		'make socks5-auth-nocf - start SOCKS5 with auth, CF off' \
		'make socks5-auth-nocf-menu - open menu with SOCKS5 auth preset, CF off' \
		'make socks5-noauth-nocf - start SOCKS5 without auth, CF off' \
		'make socks5-noauth-nocf-menu - open menu with SOCKS5 no-auth preset, CF off' \
		'make link-socks5-auth - print tg://socks link with auth' \
		'make link-socks5-noauth - print tg://socks link without auth' \
		'make mtproto-plain - start MTProto with current plain hex secret from .env' \
		'make mtproto-plain-menu - open menu with MTProto plain preset' \
		'make mtproto-plain-auth-cf-menu - open menu with MTProto plain hex preset, CF on' \
		'make mtproto-plain-auth-nocf-menu - open menu with MTProto plain hex preset, CF off' \
		'make mtproto-hex-auth-cf-menu - alias for mtproto-plain-auth-cf-menu' \
		'make mtproto-plain-nocf - start MTProto plain hex, CF off' \
		'make mtproto-plain-nocf-menu - open menu with MTProto plain preset, CF off' \
		'make mtproto-dd - start MTProto with dd secret' \
		'make mtproto-dd-menu - open menu with MTProto dd preset' \
		'make mtproto-dd-auth-cf-menu - open menu with MTProto dd preset, CF on' \
		'make mtproto-dd-auth-nocf-menu - open menu with MTProto dd preset, CF off' \
		'make mtproto-dd-nocf - start MTProto dd, CF off' \
		'make mtproto-dd-nocf-menu - open menu with MTProto dd preset, CF off' \
		'make mtproto-ee - start MTProto with ee FakeTLS secret for google.com' \
		'make mtproto-ee-menu - open menu with MTProto ee preset' \
		'make mtproto-ee-auth-cf-menu - open menu with MTProto ee preset, CF on' \
		'make mtproto-ee-auth-nocf-menu - open menu with MTProto ee preset, CF off' \
		'make menu-mtproto-ee-cf - open menu with MTProto ee preset, CF first and balance on' \
		'make mtproto-ee-nocf - start MTProto ee, CF off' \
		'make mtproto-ee-nocf-menu - open menu with MTProto ee preset, CF off' \
		'make test-go      - go test ./...' \
		'make test-go-leak - run Go tests with goleak checks enabled' \
		'make test-go-compile - compile Go packages and tests without running test cases' \
		'make test-shell   - run bats tests from ./test' \
		'make test-shell-verbose - run bats tests with verbose output' \
		'make test-shell-ci-local - run shell tests in local ubuntu docker like CI' \
		'make test-shell-file TEST=test/menu.bats - run one bats file' \
		'make test         - run Go and bats tests' \
		'make fmt-shell    - format all shell scripts with shfmt (writes in place)' \
		'make lint-shell   - check shell formatting with shfmt (diff, non-zero exit if unformatted)' \
		'make install-git-hooks - enable local pre-commit hook that runs make test' \
		'' \
		'You can override vars inline, for example:' \
		'make start MODE=socks5 PORT=1081 USERNAME=alice PASSWORD=secret VERBOSE=1'

build:
	go build -o $(LOCAL_BIN) ./cmd/tg-ws-proxy/

bundle:
	sh scripts/build-manager-standalone.sh $(BUNDLE)

menu: build
	$(MANAGER_ENV) sh $(SCRIPT)

start: build
	$(MANAGER_ENV) sh $(SCRIPT) start

start-bg: build
	$(MANAGER_ENV) sh $(SCRIPT) start-bg

stop:
	$(MANAGER_ENV) sh $(SCRIPT) stop

restart: build
	$(MANAGER_ENV) sh $(SCRIPT) restart

status:
	$(MANAGER_ENV) sh $(SCRIPT) status

run: build
	$(LOCAL_BIN) $(BIN_FLAGS)

profile-pprof:
	sh scripts/profile-pprof.sh --addr "http://$(if $(PPROF_ADDR),$(PPROF_ADDR),127.0.0.1:6060)" --steps "$(if $(STEPS),$(STEPS),3)" $(if $(LABELS),--labels "$(LABELS)",) $(if $(SLEEP),--sleep "$(SLEEP)",) $(if $(TOP),--top "$(TOP)",) $(if $(OUT),--out "$(OUT)",) $(if $(KEEP_RAW),--keep-raw,) $(if $(STRICT_EXIT),--strict-exit,)

profile-pprof-leakcheck:
	$(MAKE) profile-pprof PPROF_ADDR="$(if $(PPROF_ADDR),$(PPROF_ADDR),127.0.0.1:6060)" STEPS="$(if $(STEPS),$(STEPS),5)" LABELS="$(if $(LABELS),$(LABELS),baseline,load,cooldown,restart,after_restart_cooldown)" TOP="$(if $(TOP),$(TOP),8)" $(if $(SLEEP),SLEEP="$(SLEEP)",) $(if $(OUT),OUT="$(OUT)",) KEEP_RAW="$(if $(KEEP_RAW),$(KEEP_RAW),1)" STRICT_EXIT=1

socks5-auth: MODE := socks5
socks5-auth: SECRET :=
socks5-auth: start

socks5-auth-menu: MODE := socks5
socks5-auth-menu: SECRET :=
socks5-auth-menu: menu

socks5-auth-cf-menu: MODE := socks5
socks5-auth-cf-menu: SECRET :=
socks5-auth-cf-menu: CF_PROXY := 1
socks5-auth-cf-menu: socks5-auth-menu

socks5-menu-auth-cf: MODE := socks5
socks5-menu-auth-cf: SECRET :=
socks5-menu-auth-cf: CF_PROXY := 1
socks5-menu-auth-cf: CF_FIRST := 1
socks5-menu-auth-cf: CF_BALANCE := 1
socks5-menu-auth-cf: socks5-auth-menu

menu-socks5-auth-cf: socks5-menu-auth-cf

socks5-noauth: MODE := socks5
socks5-noauth: SECRET :=
socks5-noauth: USERNAME :=
socks5-noauth: PASSWORD :=
socks5-noauth: start

socks5-noauth-menu: MODE := socks5
socks5-noauth-menu: SECRET :=
socks5-noauth-menu: USERNAME :=
socks5-noauth-menu: PASSWORD :=
socks5-noauth-menu: menu

socks5-auth-nocf: MODE := socks5
socks5-auth-nocf: SECRET :=
socks5-auth-nocf: CF_PROXY := 0
socks5-auth-nocf: CF_FIRST := 0
socks5-auth-nocf: CF_BALANCE := 0
socks5-auth-nocf: CF_DOMAIN :=
socks5-auth-nocf: start

socks5-auth-nocf-menu: MODE := socks5
socks5-auth-nocf-menu: SECRET :=
socks5-auth-nocf-menu: CF_PROXY := 0
socks5-auth-nocf-menu: CF_FIRST := 0
socks5-auth-nocf-menu: CF_BALANCE := 0
socks5-auth-nocf-menu: CF_DOMAIN :=
socks5-auth-nocf-menu: menu

socks5-noauth-nocf: MODE := socks5
socks5-noauth-nocf: SECRET :=
socks5-noauth-nocf: USERNAME :=
socks5-noauth-nocf: PASSWORD :=
socks5-noauth-nocf: CF_PROXY := 0
socks5-noauth-nocf: CF_FIRST := 0
socks5-noauth-nocf: CF_BALANCE := 0
socks5-noauth-nocf: CF_DOMAIN :=
socks5-noauth-nocf: start

socks5-noauth-nocf-menu: MODE := socks5
socks5-noauth-nocf-menu: SECRET :=
socks5-noauth-nocf-menu: USERNAME :=
socks5-noauth-nocf-menu: PASSWORD :=
socks5-noauth-nocf-menu: CF_PROXY := 0
socks5-noauth-nocf-menu: CF_FIRST := 0
socks5-noauth-nocf-menu: CF_BALANCE := 0
socks5-noauth-nocf-menu: CF_DOMAIN :=
socks5-noauth-nocf-menu: menu

link-socks5-auth: MODE := socks5
link-socks5-auth: SECRET :=
link-socks5-auth:
	@printf 'tg://socks?server=%s&port=%s%s%s\n' \
		'$(IP)' '$(PORT)' \
		'$(if $(USERNAME),&user=$(USERNAME),)' \
		'$(if $(PASSWORD),&pass=$(PASSWORD),)'

link-socks5-noauth: MODE := socks5
link-socks5-noauth: SECRET :=
link-socks5-noauth: USERNAME :=
link-socks5-noauth: PASSWORD :=
link-socks5-noauth:
	@printf 'tg://socks?server=%s&port=%s\n' '$(IP)' '$(PORT)'

mtproto-plain: MODE := mtproto
mtproto-plain: SECRET := $(MT_PLAIN_SECRET)
mtproto-plain: start

mtproto-plain-menu: MODE := mtproto
mtproto-plain-menu: SECRET := $(MT_PLAIN_SECRET)
mtproto-plain-menu: menu

mtproto-plain-auth-cf-menu: MODE := mtproto
mtproto-plain-auth-cf-menu: SECRET := $(MT_PLAIN_SECRET)
mtproto-plain-auth-cf-menu: CF_PROXY := 1
mtproto-plain-auth-cf-menu: mtproto-plain-menu

mtproto-plain-auth-nocf-menu: mtproto-plain-nocf-menu

mtproto-hex-auth-cf-menu: mtproto-plain-auth-cf-menu

mtproto-plain-nocf: MODE := mtproto
mtproto-plain-nocf: SECRET := $(MT_PLAIN_SECRET)
mtproto-plain-nocf: CF_PROXY := 0
mtproto-plain-nocf: CF_FIRST := 0
mtproto-plain-nocf: CF_BALANCE := 0
mtproto-plain-nocf: CF_DOMAIN :=
mtproto-plain-nocf: start

mtproto-plain-nocf-menu: MODE := mtproto
mtproto-plain-nocf-menu: SECRET := $(MT_PLAIN_SECRET)
mtproto-plain-nocf-menu: CF_PROXY := 0
mtproto-plain-nocf-menu: CF_FIRST := 0
mtproto-plain-nocf-menu: CF_BALANCE := 0
mtproto-plain-nocf-menu: CF_DOMAIN :=
mtproto-plain-nocf-menu: menu

mtproto-dd: MODE := mtproto
mtproto-dd: SECRET := $(MT_DD_SECRET)
mtproto-dd: start

mtproto-dd-menu: MODE := mtproto
mtproto-dd-menu: SECRET := $(MT_DD_SECRET)
mtproto-dd-menu: menu

mtproto-dd-auth-cf-menu: MODE := mtproto
mtproto-dd-auth-cf-menu: SECRET := $(MT_DD_SECRET)
mtproto-dd-auth-cf-menu: CF_PROXY := 1
mtproto-dd-auth-cf-menu: mtproto-dd-menu

mtproto-dd-auth-nocf-menu: mtproto-dd-nocf-menu

mtproto-dd-nocf: MODE := mtproto
mtproto-dd-nocf: SECRET := $(MT_DD_SECRET)
mtproto-dd-nocf: CF_PROXY := 0
mtproto-dd-nocf: CF_FIRST := 0
mtproto-dd-nocf: CF_BALANCE := 0
mtproto-dd-nocf: CF_DOMAIN :=
mtproto-dd-nocf: start

mtproto-dd-nocf-menu: MODE := mtproto
mtproto-dd-nocf-menu: SECRET := $(MT_DD_SECRET)
mtproto-dd-nocf-menu: CF_PROXY := 0
mtproto-dd-nocf-menu: CF_FIRST := 0
mtproto-dd-nocf-menu: CF_BALANCE := 0
mtproto-dd-nocf-menu: CF_DOMAIN :=
mtproto-dd-nocf-menu: menu

mtproto-ee: MODE := mtproto
mtproto-ee: SECRET := $(MT_EE_SECRET)
mtproto-ee: start

mtproto-ee-menu: MODE := mtproto
mtproto-ee-menu: SECRET := $(MT_EE_SECRET)
mtproto-ee-menu: menu

mtproto-ee-auth-cf-menu: MODE := mtproto
mtproto-ee-auth-cf-menu: SECRET := $(MT_EE_SECRET)
mtproto-ee-auth-cf-menu: CF_PROXY := 1
mtproto-ee-auth-cf-menu: mtproto-ee-menu

mtproto-ee-auth-nocf-menu: mtproto-ee-nocf-menu

menu-mtproto-ee-cf: MODE := mtproto
menu-mtproto-ee-cf: SECRET := $(EE_GOOGLE_SECRET)
menu-mtproto-ee-cf: CF_PROXY := 1
menu-mtproto-ee-cf: CF_FIRST := 1
menu-mtproto-ee-cf: CF_BALANCE := 1
menu-mtproto-ee-cf: mtproto-ee-menu

mtproto-ee-nocf: MODE := mtproto
mtproto-ee-nocf: SECRET := $(MT_EE_SECRET)
mtproto-ee-nocf: CF_PROXY := 0
mtproto-ee-nocf: CF_FIRST := 0
mtproto-ee-nocf: CF_BALANCE := 0
mtproto-ee-nocf: CF_DOMAIN :=
mtproto-ee-nocf: start

mtproto-ee-nocf-menu: MODE := mtproto
mtproto-ee-nocf-menu: SECRET := $(MT_EE_SECRET)
mtproto-ee-nocf-menu: CF_PROXY := 0
mtproto-ee-nocf-menu: CF_FIRST := 0
mtproto-ee-nocf-menu: CF_BALANCE := 0
mtproto-ee-nocf-menu: CF_DOMAIN :=
mtproto-ee-nocf-menu: menu

test: test-go test-shell

test-go:
	go test ./...

test-go-leak:
	GOCACHE=$(GO_LOCAL_CACHE) go test . ./cmd/tg-ws-proxy ./internal/...

test-go-compile:
	GOCACHE=$(GO_LOCAL_CACHE) go test -run TestNonExistent ./...

test-shell:
	$(BATS) $(BATS_FLAGS) test

test-shell-verbose:
	$(BATS) $(BATS_VERBOSE_FLAGS) test

test-shell-ci-local:
	sh ./scripts/run-shell-tests-ci-local.sh

test-shell-file:
	@test -n "$(TEST)" || { printf '%s\n' 'TEST is required, for example: make test-shell-file TEST=test/menu.bats'; exit 1; }
	$(BATS) $(BATS_FLAGS) "$(TEST)"

fmt-shell:
	shfmt -w -i 4 tg-ws-proxy-go.sh lib scripts test

lint-shell:
	shfmt -d -i 4 tg-ws-proxy-go.sh lib scripts test

# if need to uninstall (git config --local --unset core.hooksPath))
install-git-hooks:
	git config --local core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@printf '%s\n' 'Installed local git hooks: pre-commit will run make test'

clean:
	rm -f $(BIN) $(BUNDLE)
