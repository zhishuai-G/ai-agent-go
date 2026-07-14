# 第一章：Go Agent 开发基础

本章对应课程 [M01 Go 语言 AI 开发基础](https://liwenzhou.com/courses/ai-agent/01-go-ai-agent-basics/)。目标不是直接实现复杂 Agent，而是建立后续 Agent、工具调用和流式对话都会复用的工程底座。

完成本章后，可以使用 `minicall` 向 OpenAI Chat Completions 兼容的模型服务发送一次问题，得到完整回答和 token 用量。

## 学习目标

1. 用 `internal` 和 `cmd` 分离可复用能力与程序入口。
2. 用 `context.Context` 管理请求超时、取消和重试等待。
3. 用接口抽象模型 Provider，用泛型处理结构化输出和 Channel 流。
4. 构建带连接池、限流接口和退避重试的 HTTP 客户端。
5. 通过 `httptest` 验证模型请求、响应解析和重试逻辑，而不是依赖真实 API。

## 本章交付物

```text
ai-go-server/
├── cmd/minicall/             # 单次问答 CLI 入口
├── internal/llm/             # Provider 抽象、请求/响应模型、JSON 工具
├── internal/stream/          # Context 感知的泛型 Channel 工具
├── internal/transport/       # 连接池、限流和重试 HTTP 客户端
├── .env.example              # 本地模型配置模板，不含真实密钥
└── main.go                   # 独立的最小 HTTP 服务与健康检查
```

依赖方向保持单向：`cmd/minicall` 依赖 `llm` 与 `transport`；底层包不依赖命令行或 HTTP 服务入口。这样后续增加 Web API、定时任务或其他 Agent 时，可以直接复用底层能力。

## 核心实现

| 主题 | 实现位置 | 关键点 |
| --- | --- | --- |
| 统一消息模型 | [`internal/llm/llm.go`](../internal/llm/llm.go) | `Role`、`Message`、`ChatRequest`、`ChatResponse` 与最小 `Provider` 接口 |
| 复杂 JSON | [`internal/llm/content.go`](../internal/llm/content.go) | `MessageContent` 支持文本或多模态分块的联合类型，并对出入站 JSON 对称编码 |
| 泛型流 | [`internal/stream/stream.go`](../internal/stream/stream.go) | `Process` 和 `Collect` 在读取 Channel 时优先响应取消信号 |
| HTTP 客户端 | [`internal/transport/client.go`](../internal/transport/client.go) | 连接池、可选限流、指数退避、抖动和 `Retry-After` |
| CLI 闭环 | [`cmd/minicall/main.go`](../cmd/minicall/main.go) | 读取环境变量、创建带 Context 的请求、解析回答与 token |

### Context 必须贯穿调用链

程序使用 `signal.NotifyContext` 将 `Ctrl+C` 和 `SIGTERM` 转为取消信号。该 Context 被传给 `http.NewRequestWithContext`，并进一步传给限流等待和重试等待。

因此，用户取消命令后，正在进行的 HTTP 请求和退避等待都会尽快结束。不要在底层函数中重新创建 `context.Background()`，否则会丢失上游取消信号。

### 为什么重试要重建请求体

HTTP 请求体只能读取一次。对 `429` 或 `5xx` 重试时，若直接复用已经读取过的 Body，服务端可能收到空的 POST 内容。

`minicall` 使用 `bytes.NewReader` 创建请求；标准库会据此设置 `Request.GetBody`。传输层在每次尝试前调用 `GetBody` 获取新的读取器。若调用方提供的 Body 不可重放，第二次尝试会返回 `ErrRequestBodyNotReplayable`，而不会冒险发送空请求。

### 哪些请求会重试

传输层最多重试三次（不含首次），针对以下临时错误：

- 网络层错误；
- HTTP `429 Too Many Requests`；
- HTTP `5xx`。

等待时间按指数增长并加入抖动。服务端提供 `Retry-After` 时优先使用该值。其他 `4xx` 错误，例如鉴权失败或参数错误，不会重试，应由调用方修正配置或请求内容。

## 运行 minicall

从模板创建本地配置。`.env` 已被 Git 忽略，不能提交真实 API Key：

```bash
cd ai-go-server
cp .env.example .env
```

编辑 `.env` 后加载变量并提问：

```bash
source .env
go run ./cmd/minicall -- "用一句话解释什么是 AI Agent"
```

以 DeepSeek 为例，配置为：

```bash
export LLM_BASE_URL=https://api.deepseek.com
export LLM_API_KEY='你的密钥'
export LLM_MODEL=deepseek-chat
```

预期输出为两行：第一行是模型回答，第二行形如 `token: input=12 output=34`。命令运行期间按 `Ctrl+C`，应立即取消请求或等待中的重试。

`minicall` 是本章的 CLI 验收工具，不是 Web API。当前 `main.go` 只提供 `/` 与 `/healthz`；若后续需要供前端或 Postman 调用，应增加 `POST /v1/chat/completions`，并复用本章的 `llm` 和 `transport` 包。

## 验证清单

不需要真实密钥的验证：

```bash
make test
make vet
```

测试覆盖范围如下：

| 场景 | 测试位置 |
| --- | --- |
| Provider 响应与多模态 JSON 解析 | [`internal/llm/llm_test.go`](../internal/llm/llm_test.go) |
| Channel 正常收集和 Context 取消 | [`internal/stream/stream_test.go`](../internal/stream/stream_test.go) |
| `Retry-After`、429 重试、请求体重放、取消等待 | [`internal/transport/client_test.go`](../internal/transport/client_test.go) |
| CLI 请求头、请求 JSON 和输出格式 | [`cmd/minicall/main_test.go`](../cmd/minicall/main_test.go) |

验证最小 HTTP 服务：

```bash
go run .
curl http://localhost:8080/healthz
```

预期结果：

```json
{"status":"ok"}
```

## 本章结论

一个可靠的 Agent 起点不在于先写复杂规划逻辑，而在于把模型调用做成可取消、可重试、可测试、可替换的基础设施。后续章节无论接入不同 Provider、增加流式响应还是暴露 HTTP API，都应建立在这些边界之上。
