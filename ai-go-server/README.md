# ai-go-server

一个零依赖的 Go HTTP 服务骨架，提供服务首页和健康检查接口。

## 运行

```bash
go run .
```

默认监听 `:8080`，可通过 `PORT` 指定端口：

```bash
PORT=9090 go run .
```

## 接口

- `GET /`：返回服务名称。
- `GET /healthz`：返回健康状态。

## 测试

```bash
go test ./...
```
