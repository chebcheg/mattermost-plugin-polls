PLUGIN_ID := com.chebcheg.plugin-polls
VERSION := $(shell node -p "require('./plugin.json').version")

DIST_DIR := dist
SERVER_DIST := server/dist

all: dist

.PHONY: dist
dist: server webapp bundle

.PHONY: server
server:
	@echo "Сборка серверной части $(VERSION)..."
	mkdir -p $(SERVER_DIST)
	# CGO_ENABLED=0 обязателен: с cgo в бинарник вшивается glibc сборочной машины,
	# и на более старом дистрибутиве плагин падает до рукопожатия.
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "-s -w" -o dist/plugin-linux-amd64 .
	chmod 755 $(SERVER_DIST)/plugin-linux-amd64

.PHONY: webapp
webapp:
	@echo "Сборка веб-части..."
	# webpack запускается через node: у node_modules из чужих поставок нет бита
	# выполнения, и npm-скрипт падает с «Permission denied».
	cd webapp && node node_modules/webpack/bin/webpack.js --mode=production

.PHONY: test
test:
	cd server && go vet ./... && go test ./...

.PHONY: bundle
bundle: server
	@echo "Упаковка $(PLUGIN_ID)-$(VERSION).tar.gz..."
	mkdir -p $(DIST_DIR)
	# --sort/--mtime/--owner: архив не зависит от времени сборки, повторная
	# сборка из того же тега даёт те же байты (проверяется sha256).
	tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
		-czf $(DIST_DIR)/$(PLUGIN_ID)-$(VERSION).tar.gz \
		plugin.json \
		assets \
		$(SERVER_DIST)/plugin-linux-amd64 \
		webapp/dist/main.js

.PHONY: clean
clean:
	rm -rf $(DIST_DIR) $(SERVER_DIST)
