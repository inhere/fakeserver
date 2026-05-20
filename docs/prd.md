# Fake Server PRD

需要使用 Go 实现一个 fake/mock server，用于模拟后端接口响应。

- 使用 github.com/gookit/rux 作为web框架
- 如不做任何配置默认类似 httpbin.org 的 echo 响应
- 支持自定义 data 配置 json(5)文件，用于指定接口响应数据
- 支持指定接口响应：延迟时间，响应状态码, 响应头, 响应体, 响应体格式
- 支持常用变量替换（如 {{ .request.method }} 使用go模板语法）
  - 以及内置的一些 函数，如 random, timestamp, uuid 等
- 后续支持 ws, sse 等协议 server 模拟响应

> 参考项目 https://github.com/typicode/json-server
