# 数据网关（dataway）· Go
# 两阶段：静态编译 → 最小运行镜像。CGO 关掉，产物不依赖 libc。
FROM golang:1.22-alpine AS build
WORKDIR /src

# 先只拷依赖清单，让 go mod download 这一层能被缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dataway .

FROM alpine:3.20
# 非 root 运行。容器内 UID 与 K8s 的 runAsUser 保持同一数值，避免挂载卷权限错位
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/dataway /app/dataway
USER app
EXPOSE 8090
ENV SERVER_PORT=8090
ENTRYPOINT ["/app/dataway"]
