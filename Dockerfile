# --- 第一階段：編譯階段 (Build Stage) ---
FROM golang:1.21-alpine AS builder

# 安裝編譯必要的工具
RUN apk add --no-cache gcc musl-dev

# 設定工作目錄
WORKDIR /app

# 先複製 go.mod 與 go.sum 以利用 Docker 快取
COPY go.mod go.sum ./
RUN go mod download

# 複製其餘原始碼
COPY . .

# 編譯程式碼 (針對靜態連結進行優化，確保在 Alpine 運行正常)
# CGO_ENABLED=0 是因為我們使用了 modernc.org/sqlite，這不需要 CGO
RUN CGO_ENABLED=0 GOOS=linux go build -o pcai-app ./cmd/main.go

# --- 第二階段：運行階段 (Final Stage) ---
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata
# 設定時區為台北
ENV TZ=Asia/Taipei

WORKDIR /root/

# 從編譯階段複製執行檔
COPY --from=builder /app/pcai-app .

# 建立資料庫存放目錄
RUN mkdir -p /root/data

# 暴露 Web 介面端口 (假設你用 8080)
EXPOSE 8080

# 定義磁碟卷軸 (Volume)，確保 SQLite 資料庫持久化
VOLUME ["/root/data"]

# 啟動指令，指定資料庫路徑在掛載的目錄下
CMD ["./pcai-app", "--db", "/root/data/pcai.db"]