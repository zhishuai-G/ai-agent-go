# 示例网关文档

- `timeout` 的默认值为 30 秒，单位为秒。
- 可通过 YAML 配置文件或 `GATEWAY_TIMEOUT` 环境变量修改超时。
- 配置文件中的 `timeout` 优先级高于环境变量。
- 网关支持 HTTP 和 HTTPS 上游服务。
