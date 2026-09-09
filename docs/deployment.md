# 进阶部署

默认 Docker Compose 部署只需配置初始化令牌，可选调整访问端口。浏览器和 `/api` 使用同一个地址，后端自动允许同源请求；通过服务器 IP 或域名直接访问前端端口时，不需要额外配置 Origin。

## 应用配置

先运行 `docker compose config --quiet` 检查格式，再执行 `docker compose up -d` 重建受影响的服务以应用配置；仅修改 `.env` 不会改变已经运行的容器。
