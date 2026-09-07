# 全局审查逐文件索引（2026-09-07）

本索引从当前工作区枚举 **123 个后端生产文件（40 个包）与 207 个前端生产文件**。每行连接到人工审查及缺陷证据，短 SHA-256 用于辨认审查时版本。最终检查对应的 706 个源代码/测试/配置文件与已保存验证快照逐项比对，内容无变化。

本表补齐文件索引，不把文件枚举、哈希匹配或测试通过当作独立的人工审查证明。实际阅读、失败复现、修复及测试范围见链接报告。审查范围统计不是测试行覆盖率。

## 后端生产文件

| 文件 | SHA-256（前 12 位） | 人工审查记录 |
| --- | --- | --- |
| `backend/cmd/server/main.go` | `4d6685aae7a6` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/accountdelete/service.go` | `603dac5c67fd` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/accountops/model_sync.go` | `a9560e813fbe` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/accountops/pool_mode_sync.go` | `4675dd5e4cd0` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/accountops/service.go` | `83c8adc4b72e` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/adminclient/client.go` | `e69326bac540` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/adminclient/model_pricing.go` | `06a9bb2db2a3` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/adminclient/request_latency.go` | `793bcd7db7f9` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/alerting/service.go` | `dc021e72e29d` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/api/login_throttle.go` | `ee84a07e331b` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/api/server.go` | `6e5ebe7e5444` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/authrecovery/captcha.go` | `0de07ba44738` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/authrecovery/service.go` | `dacb148d3dc0` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/business/account_block.go` | `d4636d852d1f` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_control.go` | `baf4dee64860` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_maintenance.go` | `117e8ee88695` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_model_sync.go` | `978f2574dc83` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_operations.go` | `e10a03abdc28` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_recovery.go` | `ccb64e1d94e1` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/account_state.go` | `eae556c81ef6` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/alert_evaluation.go` | `8e412b09188c` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/alert_policy.go` | `c8a2455495b7` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/alerts.go` | `d604ad157f47` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/auth_recovery.go` | `ca6d3072da96` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/billing_quota_unit.go` | `9b4168bac18e` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/catalog.go` | `5049207bcaab` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/cleanup.go` | `681034333051` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/evidence.go` | `458eebc68cdc` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/group_allocation.go` | `eb59d50210fd` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/group_policy.go` | `636559a82a2e` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/group_probe_models.go` | `b961dd0cf2cd` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/health_history.go` | `b34d6ec3cb62` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/history.go` | `533ff862ed79` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/inspection.go` | `3c01be4f98a8` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/management_sync.go` | `eefcede6ecb9` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/manual_priority.go` | `391226611381` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/model_check_configuration.go` | `70ec8e280f60` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/mutation_lease.go` | `0f9530a16cbe` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/mutation_protection.go` | `ddccece4a89d` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/newapi_management.go` | `05e542ac94ea` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/onboarding.go` | `1c766661aa8d` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/platform.go` | `40d96cd20d4d` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/policy.go` | `7d12d23cdb76` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/policy_revision.go` | `e737732a4986` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/pricing_backups.go` | `975b341e44cd` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/pricing_catalog.go` | `87aa034c6289` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/probe.go` | `52c49978c147` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/readmodels.go` | `2f4f4f61d2fb` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/revenue_catalog.go` | `1540d500e56a` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/routing.go` | `26f4a9fef4c4` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/routing_write.go` | `d5a716a4168b` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/schema.go` | `2bfe843eaf65` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/status_values.go` | `b541ab229891` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/store.go` | `09590ed14e32` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/traffic_ranking.go` | `2f047664f560` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_auth_seed.go` | `c8c4101fc212` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_catalog_identity.go` | `2d632c460607` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_configuration.go` | `83a48179ae33` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_delete.go` | `88afd5bdf687` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_group_binding_audit.go` | `60099c7d0186` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_group_history.go` | `e5d6e5171f78` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_identity.go` | `e5eec46acfd4` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/business/upstream_sync.go` | `bc125ee01d8f` | [主审查](global-review-2026-09-07.md)、[账号与上游](global-review-accounts-upstreams-2026-09-07.md)、[运行领域](operations-review-2026-09-07.md) |
| `backend/internal/config/config.go` | `5d81002307e8` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/auth.go` | `5dcf69ef1b14` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/auth_recovery_preference.go` | `431f0a743bdf` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/model_pricing_cache.go` | `0dfcc5bddcd8` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/newapi_platform.go` | `8c1c14f38133` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/store.go` | `2bc5b5e4dbdf` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/upstream_key_secret.go` | `a4fe43ec700f` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/configstore/vault.go` | `0afd8dae095f` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/decimalutil/parse.go` | `9c8b7ede1ce7` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/evidence/service.go` | `7dfa1723910d` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/inspection/manual.go` | `4ab3a8132747` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/inspection/runner.go` | `d4f0f4a6f16f` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/inspection/scheduler.go` | `d2fdf36ded53` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/logs/maintenance.go` | `9c67e2a699ff` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/logs/service.go` | `d060c7b1d81b` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/management/service.go` | `ee26fe94724e` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/modelcheck/checker.go` | `bc34f9d5ffd8` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/modelcheck/configuration.go` | `98e349823267` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/modelcheck/direct.go` | `eceb5ee4bc4c` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/modelcheck/profiles.go` | `ac5b323dcbb2` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/modelcheck/service.go` | `e0fc799b918c` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/mutationguard/guard.go` | `e255de3f79c8` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/naming/naming.go` | `3db5c44a8427` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/newapimanagement/pricing_cache.go` | `78dbf6702272` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/newapimanagement/service.go` | `5563ea6330d3` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/notification/service.go` | `ea59da9c2955` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/notificationtarget/gateway.go` | `4fe72386776b` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/notificationtarget/service.go` | `228cf0b2aec9` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/onboarding/key_cleanup.go` | `38aca807edc1` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/onboarding/probe.go` | `aff6c554425c` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/onboarding/service.go` | `6e2dc6cd5989` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/opstraffic/service.go` | `f87412bdfaa6` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/pricing/revenue.go` | `944c89dea2a9` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/pricing/service.go` | `a9c2d4b7fcb8` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/probe/manual_batch.go` | `9dd7c6431c65` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/probe/service.go` | `4619bc3b399a` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/redact/secrets.go` | `6c053543e1e7` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/routing/price_scoring.go` | `a697d98b4bab` | [运行领域](operations-review-2026-09-07.md)、[调度专项闭环](scheduling-policy-fixes-2026-09-07.md) |
| `backend/internal/routing/recovery_status.go` | `e53685939e74` | [运行领域](operations-review-2026-09-07.md)、[调度专项闭环](scheduling-policy-fixes-2026-09-07.md) |
| `backend/internal/routing/scoring.go` | `eefd1b8aa491` | [运行领域](operations-review-2026-09-07.md)、[调度专项闭环](scheduling-policy-fixes-2026-09-07.md) |
| `backend/internal/routing/service.go` | `657c95008d82` | [运行领域](operations-review-2026-09-07.md)、[调度专项闭环](scheduling-policy-fixes-2026-09-07.md) |
| `backend/internal/routingwrite/batch.go` | `e83db0a3b227` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/routingwrite/service.go` | `61329468fad5` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/runtimepolicy/mode.go` | `16ae4be574bf` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/sqliteutil/dsn.go` | `4cf4631d54c9` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/sqliteutil/permissions.go` | `83b48a83e4f3` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/systeminfo/collector.go` | `5284130b9c25` | [运行领域](operations-review-2026-09-07.md) |
| `backend/internal/targetguard/guard.go` | `9036a27bbe2a` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/taskcontext/context.go` | `f22e944da56d` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/taskrunner/feed.go` | `019d6083f159` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/taskrunner/runner.go` | `de8887f0bfa1` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/taskstore/persistence.go` | `8c3fadec2e92` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/taskstore/store.go` | `9ad8ce3dd4e9` | [主审查](global-review-2026-09-07.md) |
| `backend/internal/upstreamauth/client.go` | `e0b86bca8b0c` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamconfig/service.go` | `cbc394afc96e` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamdelete/service.go` | `aaee7a6ccd4c` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamdetect/service.go` | `4aca1943645e` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamsync/billing.go` | `b4268cf6ffed` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamsync/client.go` | `1336625c28df` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |
| `backend/internal/upstreamsync/service.go` | `414152243ec8` | [账号与上游](global-review-accounts-upstreams-2026-09-07.md) |

## 前端生产文件

| 文件 | SHA-256（前 12 位） | 人工审查记录 |
| --- | --- | --- |
| `frontend/src/App.tsx` | `9f063bf589b1` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/api.ts` | `d49e2fa1c9bb` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/__root.tsx` | `e23273490cc3` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/accounts.tsx` | `cef556cf504f` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/alert-policy.tsx` | `94d0de60b84f` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/alerts.tsx` | `7400f655a81a` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/auto-inspection.tsx` | `d139ce14cb57` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/config.tsx` | `05132be73ab0` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/groups.tsx` | `e04298ffc6c3` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/index.tsx` | `42623d55bb5c` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/logs.tsx` | `47e126e5e399` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/model-check.tsx` | `970c8da27e6c` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/channels.tsx` | `3c9e5caf572b` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/differences.tsx` | `774b370aea56` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/groups.tsx` | `a380524281a3` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/index.tsx` | `325890308210` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/prices.tsx` | `8c613fae87ff` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/newapi/route.tsx` | `8bae8490998d` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/onboarding.tsx` | `0a19e9a77a6b` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/policy.tsx` | `71ceb34b60fa` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/pricing-config.tsx` | `4ad950f65496` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/pricing.tsx` | `0aa93f75a7c4` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/profile.tsx` | `57729a137153` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/revenue-analysis.tsx` | `d5a2add0d33e` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/system-info.tsx` | `9161468176fa` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/trace.tsx` | `fd3e0583dc17` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/traffic.tsx` | `a49720118bb1` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/upstreams.tsx` | `0811fb00a795` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-routes/vault.tsx` | `0c8b3071ba9a` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/app-shell-context.ts` | `d233d09b8408` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/components/account-health-score.tsx` | `38c1d4f0a87c` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/account-recent-results.tsx` | `8f3775096b1b` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/aria-date-primitives.tsx` | `762ef765e0fd` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/confirm-action-dialog.tsx` | `a2bf8285348c` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/empty-state.tsx` | `fecd6b24d601` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/filter-menu.tsx` | `e524eceb02f1` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/filter-toolbar.tsx` | `6a750367ab34` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/number-range-filter.tsx` | `15cf137b692a` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/pagination.tsx` | `dbbcbcb23e46` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/search-field.tsx` | `941ed78a6a75` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/table-action-button.tsx` | `3f3154aa0166` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/data-table/table-panel.tsx` | `07f2be66c3a9` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/date-picker.tsx` | `8793709d7014` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/date-time-picker-utils.ts` | `034abed5baf7` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/field-help-tooltip.tsx` | `2b3f6b3f7156` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/multi-select.tsx` | `46ea6fe69f17` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/onboarding-selection-skeleton.tsx` | `7c4e581dedc4` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/page-actions.tsx` | `8bf756063fc1` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/page-heading.tsx` | `f0de505cb4d8` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/page-layout.tsx` | `2f2b161ba7f9` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/query-error-toast.tsx` | `8139e505a046` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/refresh-button.tsx` | `1f39c19d9cd8` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/result-summary-row.tsx` | `1c8a8edcc8f7` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/status-badge.tsx` | `b40db679554d` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/task-startup-state.tsx` | `ea480d29a974` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/badge.tsx` | `c2746d39c2d8` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/button.tsx` | `fd3a1ee9a1e2` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/card.tsx` | `2fa4e5393431` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/checkbox.tsx` | `639bd832f545` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/combobox.tsx` | `873f563fbabc` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/dialog.tsx` | `033cfc3ed5c5` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/dropdown-menu.tsx` | `98ea5fcf8713` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/dropdown-search-focus.ts` | `a6f11f0f630f` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/input.tsx` | `1edb5a60b554` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/progress.tsx` | `9659a225cfb4` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/segmented-control.tsx` | `e9cce6148f3f` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/select.tsx` | `c1be5eb2dce4` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/sheet.tsx` | `7b657c1a79fa` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/sidebar.tsx` | `64f9e41c5353` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/skeleton.tsx` | `9366404cfe5b` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/sonner.tsx` | `2d496349d989` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/switch.tsx` | `39a0c7204109` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/table-overflow-tooltip.tsx` | `f3f25976507c` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/table.tsx` | `12b294de870e` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/textarea.tsx` | `529667df94e2` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/components/ui/tooltip.tsx` | `8577d478781a` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/env.d.ts` | `85879669a567` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-batch-probe-dialog.tsx` | `d772500b47c8` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-delete-dialog.tsx` | `b5352ae62167` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-detail-dialog.tsx` | `9282cb5ad533` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-model-sync-dialog.tsx` | `6feefc870f04` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-operation-buttons.tsx` | `f73ab097e7b3` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-operation-controls.tsx` | `9aef169aebfc` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-pool-cells.tsx` | `73ec4261f8cd` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-probe-dialog.tsx` | `6def4143898c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-recovery-status.tsx` | `c5b4875466ec` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-settings-panel.tsx` | `e39776cc4e2f` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-sort-header.tsx` | `17fd00223177` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/account-status-tabs.tsx` | `1625f02f65da` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/base-url-check-results.tsx` | `96521ec513d6` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/manual-priority-dialog.tsx` | `71cdc6f6b135` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/components/platform-probe-dialog.tsx` | `7e0e47d10a08` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-deletion-progress.ts` | `7282dff3438e` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-labels.ts` | `a662d89e7606` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-pool.ts` | `17f6aaa21075` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-selection.ts` | `a12810b4311d` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-sort.ts` | `33a05a406026` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/account-state.ts` | `25159b738184` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/accounts/lib/platform-probe-schema.ts` | `3b0090b16640` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alert-policy/components/alert-policy-page.tsx` | `6eb4c0715b70` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alert-policy/constants.ts` | `5cae29320f43` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alert-policy/lib/alert-policy-schema.ts` | `44b927237165` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alerts/components/alert-list-actions.tsx` | `32edee4beb0c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alerts/components/notification-queue-status.tsx` | `347f14b288d4` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/alerts/lib/alert-display.ts` | `4567f27f4d55` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/account-creation-policy-form.tsx` | `8e16867a1205` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/account-creation-settings-card.tsx` | `1241bfa1fbb3` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/config-section-tabs.tsx` | `380a762fdb77` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/model-sync-settings-card.tsx` | `b37dc63c50ec` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/navigation-settings-card.tsx` | `a5c8f23cffe9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/platform-probe-models-form.tsx` | `923acddac10b` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/components/settings-footer.tsx` | `4608c9021293` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/constants.ts` | `0bbcf3ea5afa` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/lib/account-creation-settings-schema.ts` | `bd6de1a829fc` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/lib/model-sync-settings-schema.ts` | `af17ae3a9fa2` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/config/lib/platform-probe-models-schema.ts` | `50a150c37736` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/groups/components/group-allocation-dialog.tsx` | `7c7ca22350b3` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/groups/components/group-policy-editor-fields.tsx` | `3138514692f7` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/logs/components/account-rate-sync-result-table.tsx` | `7ff6d77ee594` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/logs/components/log-details-dialog.tsx` | `90d8d69606db` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/logs/components/logs-center-page.tsx` | `96cc6c54a8b9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/logs/lib/log-display.ts` | `da9ba922b5d8` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/components/model-check-configuration-dialog.tsx` | `287c8fb1ec1d` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/components/model-check-page.tsx` | `658c322318f5` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/components/model-check-result.tsx` | `5f88f0d00120` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/components/model-check-selection.tsx` | `ecb0aa64b95e` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/lib/model-check-configuration-schema.ts` | `7c40d0135106` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/model-check/lib/model-check-schema.ts` | `ae14b8522133` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/batch-model-price-dialog.tsx` | `ff0f5e0f027f` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/channel-configuration-step.tsx` | `c826965859ce` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/channel-form.tsx` | `60285fc29845` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/channel-model-dialog.tsx` | `d1a55f343b02` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/group-bindings.tsx` | `bfd558f67ced` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/model-prices.tsx` | `e9ed8f80dc73` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/newapi-management-page.tsx` | `04663c45ea2f` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/platform-dialog.tsx` | `237d4738d961` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/price-comparison.tsx` | `7a6041e709fe` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/components/raw-pricing-source-dialog.tsx` | `c57ae86d8c6b` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/constants.ts` | `21a724f4ff3a` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/lib/model-price-adjustment-schema.ts` | `2642a1678995` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/lib/model-price-adjustment.ts` | `aa8b2b8f3db4` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/lib/pricing-number.ts` | `a4542a3da682` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/newapi-management/lib/schemas.ts` | `ee617df0de0a` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/overview/components/overview-activity.tsx` | `15bb4f3840d4` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/overview/components/overview-page.tsx` | `01a13037f7ee` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/overview/lib/overview-health.ts` | `310c47f14b51` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/pricing/components/pricing-page.tsx` | `e572e87b02c9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/pricing/components/revenue-analysis-page.tsx` | `f1ae28b062cb` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/profile/components/profile-page.tsx` | `27badf0af026` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/request-trace/components/system-log-search-panel.tsx` | `907b334a0d64` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/request-trace/components/trace-account-actions.tsx` | `17d38888ac73` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/system-info/components/system-info-page.tsx` | `3e5eeaed2b2b` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/system-info/constants.ts` | `101a31a142b5` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/traffic-ranking/components/traffic-ranking-page.tsx` | `cec2a56cea9c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/traffic-ranking/lib/traffic-ranking.ts` | `00073cf9e278` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-batch-workspace.tsx` | `182454ae0833` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-candidate-visibility-filter.tsx` | `cb09ae7adbc8` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-confirm-dialog.tsx` | `172dcc7ef6f9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-group-binding-select.tsx` | `289edf5168c9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-heading-actions.tsx` | `60aaaa9ff57a` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-key-cleanup-dialog.tsx` | `abd39ff847eb` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-maintenance-actions.tsx` | `3867b8c96d42` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/onboarding-probe-action.tsx` | `59c5c0ab93c3` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-bound-account-select.tsx` | `ed7b698e3f69` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-edit-dialog.tsx` | `897f080bbec1` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-group-binding-audit.tsx` | `c68b6be1f671` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-group-binding-editor.tsx` | `b31e69ff8eaf` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-group-dialog-header.tsx` | `092057b4ad87` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-group-history.tsx` | `b5a65d755c1c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-identity.tsx` | `d5219710259c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/components/upstream-recovery-selection-toolbar.tsx` | `15afc54bb467` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/lib/onboarding-candidate-visibility.ts` | `238f3af373ac` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/lib/onboarding-requests.ts` | `f487aac28f28` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/lib/upstream-edit-schema.ts` | `172742b636d9` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/upstreams/lib/upstream-rate-labels.ts` | `1c4d89129fdd` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/features/vault/components/vault-page.tsx` | `fb691da3a56c` | [前端功能](frontend-features-review-2026-09-07.md) |
| `frontend/src/hooks/use-client-pagination.ts` | `77c0b2d47405` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/hooks/use-mobile.tsx` | `e47a06e06d0a` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/browser-preferences.ts` | `9ad2cc088ff2` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/captcha-challenge.ts` | `d3886794fcc0` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/domain-dictionaries.ts` | `106a382fccb5` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/group-policy-display.ts` | `89d1b7d97269` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/json-string-map.ts` | `f14903389cdd` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/navigation-preferences.ts` | `a75d92133d90` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/onboarding-entry.ts` | `33845eaf68f7` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/operation-feedback.ts` | `1d0a4911561d` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/query-client.ts` | `ed6b3c511350` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/scheduling-display.ts` | `99961b1066ad` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/scheduling-strategy.ts` | `49d7efe3af31` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/sensitive-field.ts` | `6c75aed70b56` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/session-auth.ts` | `32f8a45736a3` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/task-refresh.ts` | `e560aef513ab` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/task-result.ts` | `ef523ebea8bc` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/task-state.ts` | `8710d50a8a2a` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/utils.ts` | `520ef9b69532` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/lib/vault-entry-label.ts` | `5d6b8708039f` | [前端共享](frontend-shared-review-2026-09-07.md) |
| `frontend/src/main.tsx` | `471a43d5b5ff` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/newapi-index.css` | `370e12edb822` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/router.tsx` | `76fd8d963589` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/routes/alert-policy-route.tsx` | `1198492f70a6` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/routes/config-route.tsx` | `43fe7c33acf9` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/routes/newapi-routes.tsx` | `7bde20ea1d7e` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/routes/overview-route.tsx` | `837b59961f5c` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/routes/pricing-routes.tsx` | `07e57f46ebff` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/styles.css` | `65f52789b124` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/theme-presets.css` | `f5b566fe9f20` | [前端完整范围](frontend-review-2026-09-07.md) |
| `frontend/src/theme.css` | `4c187d7c6c66` | [前端完整范围](frontend-review-2026-09-07.md) |

## 工程、测试入口及生成边界

下列文件按运行、生成、发布及测试隔离边界审查；测试入口不计入上述生产文件总数。依赖锁文件验证包管理与构建一致性，生成路由树由 Rsbuild 插件和类型检查验证，第三方依赖源码不列入人工维护文件范围。

| 文件 | 验证边界 |
| --- | --- |
| `.githooks/pre-commit` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/release-notes/README.md` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/release-notes/TEMPLATE.md` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/generate-release-notes.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-browser-gates.test.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-compose.test.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-container-entrypoint.test.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-release-order.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-release-tag.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-release-tag.test.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/scripts/validate-release-workflow.test.mjs` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/workflows/ci.yml` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.github/workflows/release.yml` | CI/发布顺序、权限、脚本和提交门禁；36 项部署/脚本测试 |
| `.gitignore` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `AGENTS.md` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `README.md` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `backend/.dockerignore` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `backend/Dockerfile` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `backend/docker-entrypoint.sh` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `backend/go.mod` | Go/Bun 依赖声明、锁文件、类型检查和生产构建 |
| `backend/go.sum` | Go/Bun 依赖声明、锁文件、类型检查和生产构建 |
| `docker-compose.yml` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/.dockerignore` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/.prettierignore` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/Dockerfile` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/STYLE-GUIDE.md` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/bun.lock` | Go/Bun 依赖声明、锁文件、类型检查和生产构建 |
| `frontend/docker-entrypoint.d/40-configure-trusted-proxies.sh` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/e2e/__tests__/account-table-layout.e2e.ts` | 隔离 API、浏览器行为与 26 项桌面/移动回归 |
| `frontend/e2e/__tests__/session-cache.e2e.ts` | 隔离 API、浏览器行为与 26 项桌面/移动回归 |
| `frontend/e2e/__tests__/storage-unavailable.e2e.ts` | 隔离 API、浏览器行为与 26 项桌面/移动回归 |
| `frontend/e2e/global-styles.e2e.ts` | 隔离 API、浏览器行为与 26 项桌面/移动回归 |
| `frontend/eslint.config.mjs` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/index.html` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/knip.json` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/nginx.conf` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/package.json` | Go/Bun 依赖声明、锁文件、类型检查和生产构建 |
| `frontend/playwright.config.ts` | 隔离 API、浏览器行为与 26 项桌面/移动回归 |
| `frontend/rsbuild.config.ts` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/test-nginx-proxy.sh` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/testdata/echo-api.nginx.conf` | 镜像、启动权限、代理信任；两类镜像构建和隔离代理集成 |
| `frontend/tsconfig.json` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/vitest.config.ts` | 工程规范、构建/测试配置和环境边界；见主审查与前端报告 |
| `frontend/src/routeTree.gen.ts` | 文件路由生成结果、路由插件和 TypeScript 类型检查 |

## 最终结论依据

- 分配范围覆盖全部人工维护的生产代码、前后端契约、鉴权/敏感数据边界、任务与取消、事务补偿、调度与人工保护、加载/错误/键盘交互以及部署门禁。
- 原调度专项 10 项已逐项转入正式回归并闭环；旧报告中的未完成表述已标注为历史进度或改为指向最终结果。
- Go 全量 race、994 项 Vitest、26 项 Playwright、typecheck、lint、format、Knip、前后端构建及部署专项结果均已核对。
- 未访问生产数据库、真实密码箱、真实上游或通知渠道；没有提交或部署。生产联调不在本次隔离审查结果中。
