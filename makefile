.PHONY: test build run deploy clean install BINARY_NAME=pcai

build:
	go build -o $(BINARY_NAME) ./...

build-arm:
	GOOS=linux GOARCH=arm64 go build -o $(BINARY_NAME)-arm64 ./...

build-win:
	set GOOS=windows; set GOARCH=amd64; go build -o $(BINARY_NAME).exe ./...

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-arm64 $(BINARY_NAME).exe
	go clean

install:
	go build -o $(BINARY_NAME) ./...
	mv $(BINARY_NAME) /usr/local/bin/

test-tools:
	go test -v ./tools/...

s:
	git push -u origin main