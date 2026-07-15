# ai-go-server

基于 [M01 Go 语言 AI 开发基础](https://liwenzhou.com/courses/ai-agent/01-go-ai-agent-basics/) 的零依赖实现。项目保留一个最小 HTTP 服务，同时提供课程配套的 `minicall` 命令行问答工具。

课程学习文档：

- [第一章：Go Agent 开发基础](docs/01-go-ai-agent-basics.md)
- [第二章：LLM 全平台接入](docs/02-llm-provider-adapter.md)
- [第三章：Prompt 与上下文工程基础](docs/03-prompt-context.md)

## 结构

```text
.
├── cmd/minicall          # OpenAI 兼容 API 的单次问答 CLI
├── cmd/docqa             # 基于本地资料的文档问答 CLI
├── internal/llm          # Provider 接口、统一消息模型与 JSON 解析
├── internal/prompt       # Prompt 模板、上下文预算与文档助手上下文
├── internal/provider     # OpenAI 兼容、Claude Provider 与注册表
├── internal/router       # 多 Provider 的优先级路由与降级
├── internal/schema       # 结构化输出/工具参数的 JSON Schema 生成
├── internal/cost         # token 用量和成本累计
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
make docqa DOCS=examples/docqa/gateway.md QUESTION='默认超时是多少？'
```
