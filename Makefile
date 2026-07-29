.PHONY: build run login clean install

BINARY_NAME = ytb
INSTALL_DIR = $(HOME)/.local/bin

build:
	GONOSUMCHECK=* GONOSUMDB=* go build -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME) $(ARGS)

login: build
	./$(BINARY_NAME) login

install: build
	@mkdir -p $(INSTALL_DIR)

	cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "✅ $(BINARY_NAME) 已安装到 $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "   在终端中执行 ytb 即可使用"

clean:
	rm -f $(BINARY_NAME)
	rm -rf data/
