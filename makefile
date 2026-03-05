# 專案名稱
BINARY_NAME=pcai

# 偵測作業系統 (Windows 會內建 OS 變數為 Windows_NT)
ifeq ($(OS),Windows_NT)
    PLATFORM := Windows
    BINARY_EXT := .exe
    # Windows CMD 刪除檔案指令不同
    RM := del /Q
    # 修正 Windows 環境變數設定語法
    SET_ENV := set
    SEP := &&
else
    PLATFORM := $(shell uname -s)
    BINARY_EXT :=
    RM := rm -f
    SET_ENV := export
    SEP := ;
endif

.PHONY: all build build-win build-arm clean help

# 預設動作：根據當前系統編譯
all: build

## build: 根據目前作業系統編譯執行檔
build:
	@echo "Detected Platform: $(PLATFORM)"
	go build -o $(BINARY_NAME)$(BINARY_EXT) main.go
	@echo "Build successful: $(BINARY_NAME)$(BINARY_EXT)"

## build-win: 強制跨平台編譯 Windows 版本 (amd64)
build-win:
ifeq ($(OS),Windows_NT)
	set GOOS=windows&& set GOARCH=amd64&& go build -o $(BINARY_NAME).exe main.go
else
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME).exe main.go
endif
	@echo "Windows build successful: $(BINARY_NAME).exe"

## build-arm: 強制跨平台編譯 Linux ARM64 版本
build-arm:
ifeq ($(OS),Windows_NT)
	set GOOS=linux&& set GOARCH=arm64&& go build -o $(BINARY_NAME)-arm64 ./...
else
	GOOS=linux GOARCH=arm64 go build -o $(BINARY_NAME)-arm64 ./...
endif
	@echo "Linux ARM64 build successful: $(BINARY_NAME)-arm64"

## clean: 清理編譯產出的執行檔
clean:
	$(RM) $(BINARY_NAME) $(BINARY_NAME).exe $(BINARY_NAME)-arm64
	@echo "Cleaned up binaries."

## help: 顯示指令說明
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed -e 's/## //g' | column -t -s ':'

install:
	go build -o $(BINARY_NAME) ./...
	mv $(BINARY_NAME) /usr/local/bin/

test-tools:
	go test -v ./tools/...

s:
	git push -u origin main