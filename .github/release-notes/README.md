# 发布说明硬性规则

每次创建版本标签前，必须先完成并提交同名发布说明。

1. 版本使用 `vYYYY.MM.DD`；同一天的第二个版本开始依次使用 `-2`、`-3`，禁止 `-1`。
2. 复制 `TEMPLATE.md` 为 `<tag>.md`，例如 `v2026.08.28-2.md`。
3. 五个二级章节必须完整、顺序固定；没有内容的章节填写 `- 无`，不得保留占位注释。
4. 发布说明必须与代码一起提交并推送，然后在该提交上创建并推送标签。
5. 标签必须指向 `main` 已包含的提交，不能从未合并分支直接发布。
6. 发布必须通过前端格式、类型、Lint、Knip、单元测试、完整浏览器回归与构建，以及后端 Vet、Race 测试与构建。CI 和发布回退检查共用 `quality.yml`；仅允许复用同仓库、同一完整提交 SHA、`main` 的 `push` 事件且最新运行成功的 CI。
7. 当前使用包含 Go API 和 React 前端的单体镜像（授权及安全 SDK 兼容层在 Go 进程内运行，无 browser worker）；amd64、arm64 两个架构全部发布并通过版本、提交校验后，才允许晋级 `latest` 和创建 GitHub Release。

多个版本的发布工作流串行执行；同一版本的独立质量检查并行执行。发布前校验标签格式、连续版本号、发布说明和提交已进入 `main`，并读取注册表中的版本与 `latest` 元数据。旧版本不能覆盖较新的 `latest`，同一版本不能改指向另一个提交；重跑时复用已经发布且校验通过的版本，不重新构建覆盖标签。注册表鉴权、网络或解析失败会中止发布，不视为镜像不存在。

晋级前再次校验两个架构的 `version`、`revision` 和版本顺序，使用已校验的不可变摘要更新 `latest` 并验证结果。首次缺少 `latest` 时先发布版本镜像，再停止晋级，按下文人工初始化。若更新 `latest` 后 GitHub Release 创建失败，镜像仍保留已验证版本，可重跑发布任务完成 Release。工作流的并发保护不覆盖人工修改注册表标签；生产部署建议固定版本标签或摘要。

## 集中检查与一次推送

1. 开始前读取 `AGENTS.md`，统计相对上一版本的完整变更，说明发布范围、检查阶段和耗时依据。先确定受影响模块、共享组件、接口、数据库及部署配置，不能只检查最后一个功能。
2. 先运行受影响测试与静态检查，保留每项输出并收集独立检查的所有失败。只在基础环境故障阻止继续时先修环境；不要发现一个问题就立即推送。
3. 将发现的问题按原因归类、集中修复，先跑失败项和受影响回归。代码稳定后执行所需完整验证；不在每修一个用例后重跑全套，也不删测试或放宽断言来掩盖缺陷。
4. 保存命令、范围、对应提交/工作区变更、结果与日志位置。检查已通过且对应内容未变时直接使用有效结果；新修改仅使受影响检查失效，影响不明时扩大验证。发布配置检查命令为 `node --test .github/scripts/*.test.mjs .github/scripts/__tests__/*.test.mjs`。
5. 将发布说明和全部本轮代码纳入最终候选提交，确认所有已知失败解决、受影响检查通过后统一推送。允许本地拆分逻辑提交，禁止以反复推送作为本地验证的替代。
6. 等待该提交 CI 完成，集中查看所有失败分片与诊断；如远端出现新增问题，集中修复、验证后统一补推。成功后才创建标签，发布复用该完整提交的成功 CI。用户仅要求改进流程时，不创建新版本标签。
7. 执行期间说明发现的问题、剩余阶段和实际阻塞；最终提供提交/版本、验证结果和未解决限制。不承诺一次检查能发现所有缺陷，但必须在本轮已知问题闭环前避免零碎推送。

## 自动化提速及失败保护

- `quality.yml` 将配置、前端、后端与浏览器检查放在独立任务中；浏览器使用 4 个分片且 `fail-fast: false`，收集全部分片失败，各分片保存独立诊断文件。测试范围与本地全量命令一致。
- 发布始终先校验来源和发布说明，再通过 `release-ci.mjs` 查询固定的 `ci.yml`。同一提交的 CI 正在运行时最多等待约 30 分钟；没有有效成功结果时执行完整共享检查，不能以读取失败作为放行依据。
- 发布门禁要求来源校验成功，且有可信的成功 CI 或本轮完整检查成功。失败、取消和不符合预期的跳过均不放行；旧提交、PR 或其他分支的结果不能复用。
- 发布镜像通过 BuildKit 的 GitHub Actions 缓存复用未变化构建层，缓存不会替代源码校验与双架构版本/提交校验。全部镜像校验通过才更新 `latest` 和创建 Release。
- 提速幅度以实际 Actions 耗时为准；分片减少等待但会增加同时使用的 runner，首次缓存构建也可能没有收益。

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
