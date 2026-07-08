.PHONY: build run clean

build:
	GONOSUMCHECK=* GONOSUMDB=* go build -o ytb2bili .

run: build
	./ytb2bili $(ARGS)

login: build
	./ytb2bili login

clean:
	rm -f ytb2bili
	rm -rf data/
