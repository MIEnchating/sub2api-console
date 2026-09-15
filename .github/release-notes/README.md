# 发布说明硬性规则

每次创建版本标签前，必须先完成并提交同名发布说明。

1. 版本使用 `vYYYY.MM.DD`；同一天的第二个版本开始依次使用 `-2`、`-3`，禁止 `-1`。
2. 复制 `TEMPLATE.md` 为 `<tag>.md`，例如 `v2026.08.28-2.md`。
3. 五个二级章节必须完整、顺序固定；没有内容的章节填写 `- 无`，不得保留占位注释。
4. 发布说明必须与代码一起提交并推送，然后在该提交上创建并推送标签。
5. 标签必须指向 `main` 已包含的提交，不能从未合并分支直接发布。
6. Release 工作流必须通过前端格式、类型、Lint、测试与构建，以及后端 Vet、Race 测试与构建。
7. 当前使用包含 Go API、React 前端和 browser worker 的单体镜像；amd64、arm64 两个架构全部发布并通过版本、提交校验后，才允许晋级 `latest` 和创建 GitHub Release。

发布工作流串行执行。发布前校验标签格式、连续版本号、发布说明和提交已进入 `main`，并读取注册表中的版本与 `latest` 元数据。旧版本不能覆盖较新的 `latest`，同一版本不能改指向另一个提交；重跑时复用已经发布且校验通过的版本，不重新构建覆盖标签。注册表鉴权、网络或解析失败会中止发布，不视为镜像不存在。

晋级前再次校验两个架构的 `version`、`revision` 和版本顺序，使用已校验的不可变摘要更新 `latest` 并验证结果。首次缺少 `latest` 时先发布版本镜像，再停止晋级，按下文人工初始化。若更新 `latest` 后 GitHub Release 创建失败，镜像仍保留已验证版本，可重跑发布任务完成 Release。工作流的并发保护不覆盖人工修改注册表标签；生产部署建议固定版本标签或摘要。

## 首次初始化 Docker Hub latest

发布目标为 Docker Hub 的 `mienvirtuoso/sub2api-console`。先创建该镜像仓库，并在 GitHub Actions 的仓库 Secrets 中配置 `DOCKERHUB_TOKEN`（`mienvirtuoso` 账号的 Read & Write Access Token）。工作流内的 GitHub Token 仅用于访问代码和创建 GitHub Release。

首次标签发布已完成版本镜像推送及两个架构校验后，`latest` 晋级会因标签尚不存在而停止。确认这是首次初始化后，用该已验证版本完成初始化；以下版本号须替换为本次实际标签：

```bash
release_tag=v2026.09.09
docker login --username mienvirtuoso

docker buildx imagetools create \
  -t mienvirtuoso/sub2api-console:latest \
  "mienvirtuoso/sub2api-console:$release_tag"

docker buildx imagetools inspect "mienvirtuoso/sub2api-console:$release_tag"
docker buildx imagetools inspect mienvirtuoso/sub2api-console:latest
```

登录时在密码提示中输入 Docker Hub Access Token。核对 `latest` 的 Digest 与版本标签相同，再到 GitHub Actions 重跑失败任务；工作流会再次校验两个架构的版本及提交 revision，一致后完成 GitHub Release。后续发布不需要手动初始化。
