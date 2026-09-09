# 发布说明硬性规则

每次创建版本标签前，必须先完成并提交同名发布说明。

1. 版本使用 `vYYYY.MM.DD`；同一天的第二个版本开始依次使用 `-2`、`-3`，禁止 `-1`。
2. 复制 `TEMPLATE.md` 为 `<tag>.md`，例如 `v2026.08.28-2.md`。
3. 五个二级章节必须完整、顺序固定；没有内容的章节填写 `- 无`，不得保留占位注释。
4. 发布说明必须与代码一起提交并推送，然后在该提交上创建并推送标签。
5. 标签必须指向 `main` 已包含的提交，不能从未合并分支直接发布。
6. Release 工作流必须通过前端格式、类型、Lint、测试与构建，以及后端 Vet、Race 测试与构建。
7. API 和前端 Dockerfile 都必须通过预构建；两个镜像的 amd64、arm64 架构与多架构清单全部发布成功后，才允许创建 GitHub Release。

工作流只会从已经完整存在、且 amd64/arm64 的 `version` 与 `revision` 标签分别一致的 API/前端 `latest` 对进行自动晋级；若两个标签都不存在，需先将同一个已验证版本、同一个提交 revision 的两个镜像分别设为 `latest`，并确认标签与摘要。若只存在一个，或两者的版本、revision 不同，则先修复标签对。旧版本不能覆盖较新的 `latest`，同一个版本标签也不能改指向另一个提交。这样可以避免首次晋级中断后留下半初始化、跨版本、跨提交或版本回退状态。

任何前置检查失败都会中止发布，不生成不完整 Release，也不更新 `latest` 多架构镜像。晋级期间失败时，或晋级完成但 GitHub Release 未成功发布时，工作流会重试恢复两个旧摘要；若注册表持续不可用导致回滚不完整，必须先检查并修复两个 `latest` 标签，再重试发布。两个独立镜像标签无法原子切换，生产部署仍必须固定同一个版本标签。

## 首次初始化 Docker Hub latest

发布目标为 Docker Hub 的 `mienvirtuoso/sub2api-console-api` 和 `mienvirtuoso/sub2api-console-frontend`。先创建两个镜像仓库，并在 GitHub Actions 的仓库 Secrets 中配置 `DOCKERHUB_TOKEN`（`mienvirtuoso` 账号的 Read & Write Access Token）。工作流内的 GitHub Token 仅用于访问代码和创建 GitHub Release。

首次标签发布通过两个 `Publish and verify … version manifest` 任务后，`latest` 晋级会因标签对尚不存在而停止。确认这是首次初始化、两个 `latest` 都不存在后，用同一个已验证版本完成初始化；以下版本号须替换为本次实际标签：

```bash
release_tag=v2026.09.09
docker login --username mienvirtuoso

docker buildx imagetools create \
  -t docker.io/mienvirtuoso/sub2api-console-api:latest \
  "docker.io/mienvirtuoso/sub2api-console-api:$release_tag"
docker buildx imagetools create \
  -t docker.io/mienvirtuoso/sub2api-console-frontend:latest \
  "docker.io/mienvirtuoso/sub2api-console-frontend:$release_tag"

docker buildx imagetools inspect "docker.io/mienvirtuoso/sub2api-console-api:$release_tag"
docker buildx imagetools inspect docker.io/mienvirtuoso/sub2api-console-api:latest
docker buildx imagetools inspect "docker.io/mienvirtuoso/sub2api-console-frontend:$release_tag"
docker buildx imagetools inspect docker.io/mienvirtuoso/sub2api-console-frontend:latest
```

登录时在密码提示中输入 Docker Hub Access Token。核对每个 `latest` 的 Digest 与其版本标签相同，再到 GitHub Actions 重跑失败任务；工作流会再次校验两个架构的版本及提交 revision，一致后完成 GitHub Release。后续发布不需要手动初始化。
