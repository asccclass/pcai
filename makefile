.PHONY: test build run deploy clean install BINARY_NAME=pcai

build:
	go build -o $(BINARY_NAME) main.go

build-arm:
	# 針對你的 GX10 機器 (Linux ARM64)
	GOOS=linux GOARCH=arm64 go build -o $(BINARY_NAME)-arm64 main.go

build-win:
	# 針對 Windows
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME).exe main.go

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-arm64 $(BINARY_NAME).exe
	go clean

install:
	go build -o $(BINARY_NAME) main.go
	mv $(BINARY_NAME) /usr/local/bin/

test-tools:
	go test -v ./tools/...

s:
	git push -u origin main