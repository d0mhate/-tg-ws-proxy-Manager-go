BIN      := tg-ws-proxy
SCRIPT   := tg-ws-proxy-go.sh
BUNDLE   := build/tg-ws-proxy-go.sh
comma    := ,

-include .env

# override any of these: make menu MODE=mtproto PORT=1443 IP=1.2.3.4 CF_DOMAIN=x.xyz VERBOSE=1
BIN_PATH  ?= ./$(BIN)
HOST      ?=
PORT      ?= 1080
MODE      ?= socks5
SECRET    ?=
IP        ?=
DC_IPS    ?=
POOL_SIZE ?=
USERNAME  ?=
PASSWORD  ?=
CF_PROXY  ?=
CF_FIRST  ?=
CF_DOMAIN ?=
VERBOSE   ?=

.PHONY: build bundle menu menu-mtproto-local run test clean

# build go binary for current platform
build:
	go build -o $(BIN) ./cmd/tg-ws-proxy/

# pack all lib/*.sh into one standalone script -> build/
bundle:
	sh scripts/build-manager-standalone.sh $(BUNDLE)

# open the menu with local binary, pass vars from above
menu: build
	BIN_PATH=$(BIN_PATH) \
	PROXY_MODE=$(MODE) \
	LISTEN_PORT=$(PORT) \
	$(if $(HOST),LISTEN_HOST=$(HOST),) \
	$(if $(SECRET),MT_SECRET=$(SECRET),) \
	$(if $(IP),MT_LINK_IP=$(IP),) \
	$(if $(DC_IPS),DC_IPS=$(DC_IPS),) \
	$(if $(POOL_SIZE),POOL_SIZE=$(POOL_SIZE),) \
	$(if $(USERNAME),SOCKS_USERNAME=$(USERNAME),) \
	$(if $(PASSWORD),SOCKS_PASSWORD=$(PASSWORD),) \
	$(if $(CF_PROXY),CF_PROXY=$(CF_PROXY),) \
	$(if $(CF_FIRST),CF_PROXY_FIRST=$(CF_FIRST),) \
	$(if $(CF_DOMAIN),CF_DOMAIN=$(CF_DOMAIN),) \
	$(if $(VERBOSE),VERBOSE=$(VERBOSE),) \
	bash $(SCRIPT)

# local MTProto menu on the current machine, using the locally built binary
menu-mtproto-local: build
	BIN_PATH=$(BIN_PATH) \
	PROXY_MODE=mtproto \
	LISTEN_HOST=127.0.0.1 \
	LISTEN_PORT=$(PORT) \
	$(if $(SECRET),MT_SECRET=$(SECRET),) \
	$(if $(POOL_SIZE),POOL_SIZE=$(POOL_SIZE),) \
	MT_LINK_IP=127.0.0.1 \
	CF_PROXY=0 \
	CF_PROXY_FIRST=0 \
	bash $(SCRIPT)

# open menu: installed v1.1.33, latest v1.1.34 (latest track)
menu-update-demo: build
	@mkdir -p /tmp/tg-ws-proxy-demo
	@printf "v1.1.33\n" > /tmp/tg-ws-proxy-demo/version
	@printf "v1.1.34\n%s\n" "$$(date +%s)" > /tmp/tg-ws-proxy-demo/latest_version_cache
	@rm -f /tmp/tg-ws-proxy-demo/release_tag /tmp/tg-ws-proxy-demo/update_channel /tmp/tg-ws-proxy-demo/preview_branch
	BIN_PATH=$(BIN_PATH) \
	PROXY_MODE=$(MODE) \
	LISTEN_PORT=$(PORT) \
	VERSION_FILE=/tmp/tg-ws-proxy-demo/version \
	PERSIST_STATE_DIR=/tmp/tg-ws-proxy-demo \
	LATEST_VERSION_CACHE_FILE=/tmp/tg-ws-proxy-demo/latest_version_cache \
	bash $(SCRIPT)

# open menu: installed v1.1.30, pinned to v1.1.30, latest v1.1.34
menu-pinned-demo: build
	@mkdir -p /tmp/tg-ws-proxy-demo
	@printf "v1.1.30\n" > /tmp/tg-ws-proxy-demo/version
	@printf "v1.1.30\n" > /tmp/tg-ws-proxy-demo/release_tag
	@printf "v1.1.34\n%s\n" "$$(date +%s)" > /tmp/tg-ws-proxy-demo/latest_version_cache
	@rm -f /tmp/tg-ws-proxy-demo/update_channel /tmp/tg-ws-proxy-demo/preview_branch
	BIN_PATH=$(BIN_PATH) \
	PROXY_MODE=$(MODE) \
	LISTEN_PORT=$(PORT) \
	VERSION_FILE=/tmp/tg-ws-proxy-demo/version \
	PERSIST_STATE_DIR=/tmp/tg-ws-proxy-demo \
	LATEST_VERSION_CACHE_FILE=/tmp/tg-ws-proxy-demo/latest_version_cache \
	bash $(SCRIPT)

# open menu: installed v1.1.33, preview channel branch "my-feature", latest v1.1.34
menu-preview-demo: build
	@mkdir -p /tmp/tg-ws-proxy-demo
	@printf "v1.1.33\n" > /tmp/tg-ws-proxy-demo/version
	@printf "preview\n" > /tmp/tg-ws-proxy-demo/update_channel
	@printf "my-feature\n" > /tmp/tg-ws-proxy-demo/preview_branch
	@printf "v1.1.34\n%s\n" "$$(date +%s)" > /tmp/tg-ws-proxy-demo/latest_version_cache
	@rm -f /tmp/tg-ws-proxy-demo/release_tag
	BIN_PATH=$(BIN_PATH) \
	PROXY_MODE=$(MODE) \
	LISTEN_PORT=$(PORT) \
	VERSION_FILE=/tmp/tg-ws-proxy-demo/version \
	PERSIST_STATE_DIR=/tmp/tg-ws-proxy-demo \
	LATEST_VERSION_CACHE_FILE=/tmp/tg-ws-proxy-demo/latest_version_cache \
	bash $(SCRIPT)

# run proxy directly without menu
run: build
	./$(BIN) --mode $(MODE) --port $(PORT) \
	$(if $(HOST),--host $(HOST),) \
	$(if $(SECRET),--secret $(SECRET),) \
	$(if $(IP),--link-ip $(IP),) \
	$(foreach dc_ip,$(subst $(comma), ,$(strip $(DC_IPS))),--dc-ip $(dc_ip) ) \
	$(if $(POOL_SIZE),--pool-size $(POOL_SIZE),) \
	$(if $(USERNAME),--username $(USERNAME) --password $(PASSWORD),) \
	$(if $(VERBOSE),--verbose,)

# run all tests
test:
	go test ./...

# remove built binary and bundle
clean:
	rm -f $(BIN) $(BUNDLE)
