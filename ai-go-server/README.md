# ai-go-server

基于 [M01 Go 语言 AI 开发基础](https://liwenzhou.com/courses/ai-agent/01-go-ai-agent-basics/) 的零依赖实现。项目保留一个最小 HTTP 服务，同时提供课程配套的 `minicall` 命令行问答工具。

## 结构

```text
.
├── cmd/minicall          # OpenAI 兼容 API 的单次问答 CLI
├── internal/llm          # Provider 接口、统一消息模型与 JSON 解析
├── internal/stream       # Context 感知的泛型 Channel 消费工具
├── internal/transport    # 连接池、限流接口、重试退避 HTTP 客户端
└── main.go               # 保留的服务首页和健康检查
```

`internal` 只被本模块的入口程序引用，外部调用方不会依赖具体传输或模型实现。

## 运行 HTTP 服务

```bash
go run .
```

默认监听 `:8080`，可通过 `PORT` 指定端口：

```bash
PORT=9090 go run .
```

- `GET /` 返回服务名称。
- `GET /healthz` 返回健康状态。

## 运行 minicall

准备一个 OpenAI Chat Completions 兼容服务的地址和凭据：

```bash
export LLM_BASE_URL=https://api.example.com/v1
export LLM_API_KEY=your-api-key
export LLM_MODEL=your-model
go run ./cmd/minicall -- "用一句话解释什么是 AI Agent"
```

输出包含完整回答与本次的 `input/output` token 用量。按 `Ctrl+C` 会取消请求，也会中断重试等待。

传输层对网络错误、`429` 与 `5xx` 使用指数退避和抖动重试，优先遵守服务端的 `Retry-After`。请求必须通过 `http.NewRequestWithContext` 构造；对于带 Body 的重试，推荐使用 `bytes.Reader` 或 `strings.Reader`，以便安全重放请求体。

## 常用命令

```bash
make test
make vet
make fmt
make minicall QUESTION='你好'
```
