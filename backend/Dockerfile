# BUILDPLATFORM 在 buildx 下自动注入；经典构建可能为空，给默认值避免 FROM 解析失败
ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM node:22-alpine AS admin_builder

WORKDIR /src/frontend/admin
COPY frontend/admin/package*.json ./
COPY frontend/admin/vendor ./vendor
RUN npm ci
COPY frontend/admin/ ./
RUN npm run build

ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS builder

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# 前端产物覆盖占位目录后嵌入二进制
COPY --from=admin_builder /src/frontend/admin/dist ./internal/web/admin
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/islet ./cmd/server
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/importlsky ./cmd/importlsky

FROM alpine:3.24.3

RUN apk add --no-cache tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && adduser -D -H -u 10001 islet \
    && mkdir -p /data \
    && chown islet:islet /data

ENV TZ=Asia/Shanghai
USER islet

WORKDIR /app
COPY --from=builder /out/islet /app/islet
COPY --from=builder /out/importlsky /app/importlsky

EXPOSE 8080 8081
VOLUME ["/data"]
ENTRYPOINT ["/app/islet"]
