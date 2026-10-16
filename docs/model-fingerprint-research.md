# 模型检测与指纹方案调研

调研日期：2026-10-16。阅读项目 README 与下列源码，没有执行第三方代码或向真实模型发送请求。现已将公开参考比较接入统一「模型检测」，覆盖用户选定的 18 个精确型号记录；其中 Gemini 3.7 Flash 缺少有效对照值。实施、请求预算、来源和判定限制见 [公开指纹方案](public-model-fingerprints.md)。下文保留早期方法调研，厂商工具调用／JSON Schema 检测仍属候选。

## 当前能力

现有 Go 引擎按平台使用 OpenAI Responses／兼容协议、Anthropic Messages 和 Gemini 原生协议，但能力题和指纹采样仍共用同一套规则。默认执行 7 项能力题，以及基准、中文、英文、代码、Unicode 五类文本各两次的 10 项分词采样。

`token-delta-v1` 记录输入用量、协议、返回型号，检查重复采样是否稳定，并减去基准文本用量。它是分词与接口行为的测量，尚无经过校准的模型身份分类器。多个型号可能共用分词器；中转也可能修改用量、系统提示词和返回型号。不能把“稳定”解释成“确为该型号”。

官方对照入口已按此前要求移除，本次不恢复。下述研究方法如需参考样本，应另行设计有来源、版本和适用条件的指纹数据；没有参考时只报告采集结果与稳定性。

## 阅读结果与取舍

| 项目 | 实际做法 | 借鉴价值与限制 |
| --- | --- | --- |
| [veridrop](https://github.com/canarybyte/veridrop)（AGPL-3.0） | 各协议维护独立检测器注册表；OpenAI 有工具调用、结构化输出、长上下文、用量检查；Anthropic 增加 thinking／signature、PDF 等；Gemini 有原生工具调用、结构化输出、协议及用量检查 | 适合借鉴“公共检测 + 厂商能力检测”的组织方式。signature 检测实际检查字段与长度，没有验证签名，不能据此认证身份。不能把协议兼容失败直接当作假模型 |
| [modelprint](https://github.com/unclecode/modelprint)（MIT） | 固定文本重复采样，减去短文本用量；同时探测模板开销、错误行为、响应头和可选 logprobs 几何特征 | 分词差分与当前实现同类，可参考更丰富文本和缺失数据诊断。响应头／错误更接近网关特征；logprobs 只适用于确实支持并返回候选概率的接口。源码中的识别力宣传未经本项目验证 |
| [llm-fingerprint-detector](https://github.com/ToseaAI/llm-fingerprint-detector)（MIT） | 重复请求短回答任务，统计各题／语言的回答分布，用 Jensen–Shannon divergence 比较参考；还比较奇偶采样子集检查自身稳定性 | 最适合作为可选“行为指纹”实验方向。示例为 8 个单元 × 25 次，即约 200 请求；源码要求每侧每单元至少 10 个有效样本、至少 4 个可比单元。协议、系统提示词、temperature 与思考配置必须匹配；仓库的固定阈值不能直接当作我们的验收标准 |
| [KBF](https://github.com/Ooo0ption/KBF)（Apache-2.0） | 使用特定模型在知识边界上的稳定回答／错误模式；参考自测噪声经 Clopper–Pearson 上界校准，再对目标差异做单侧二项检验；覆盖不足返回无法判定 | 比统一简单算术题更适合研究型号区别，但需要版本明确的模型专属题集、自测与覆盖率。其 `SAME` 实际含义是未拒绝当前统计假设，不能翻译为已证明同一模型。后续直接读取确认 2026 年 9 月目录含 28 份实际 JSON，本次选用其中 10 份；子目录 README 缺失不影响数据存在 |
| [LLMmap](https://github.com/pasquini-dario/LLMmap)（MIT） | 将回答编码后送入训练过的模型，与行为模板比较；提供开放集模板及训练流程 | 可借鉴多提示词配置下采集模板和独立评估的思路。现成实现依赖 Python／PyTorch，默认模板包含较旧型号，不能直接加入本项目 Go 业务运行时，也不能用旧模板识别未知新型号 |

补充阅读 [llm-verify](https://github.com/mintesnot-teshome/llm-verify) 的指纹服务：统计长度、Markdown、措辞、结构和元数据。这些适合做可解释观察项，容易受提示词与语言影响，优先级低于可重复、可校准的指纹方法。

## 按厂商与型号选择检测项

建议按实际配置的协议与已验证能力选择检测器；模型名称仅用于匹配已登记能力配置，不能用名称猜测身份。

| 范围 | 建议检测项 | 结果含义 |
| --- | --- | --- |
| 公共层 | 当前能力题、复测、分词差分、响应完整性、实际协议与型号记录 | 能力是否通过、采样是否稳定、协议事实 |
| OpenAI 兼容接口 | 区分 Responses 与 Chat；原生工具调用／JSON Schema、流式用量、按能力支持的 reasoning 参数；可选 logprobs | 声称的接口能力是否可用；不支持的能力标为不适用 |
| Anthropic 接口 | Messages 内容块、工具调用、stop reason、用量语义、适用型号的 thinking 与 signature 字段 | 协议和能力一致性；签名字段存在不代表已经验证签名 |
| Gemini 接口 | 原生 parts／functionCall、结构化输出、finishReason／usageMetadata、适用型号的思考配置 | 原生能力与结果完整性，不能以 OpenAI 字段缺失判失败 |
| 同厂商不同型号／版本 | 经过验收的能力配置、长上下文阶梯抽查、独立行为指纹或模型专属知识边界题集 | 型号差异的证据；上下文测试须显式预算，少量短题不能验证全部上下文长度 |

当前结构化能力题只验证文本回答符合题目要求，不能代替 API 原生 JSON Schema／工具调用参数检测。共用通用题仍有价值，新增厂商检测应作为独立维度。

## 建议实施顺序

1. 将检测器注册为公共项与协议能力项，保存检测器版本、适用条件、请求预算和不适用原因。未知型号只执行已知支持项，不能靠反复报错试出一套“认证”分数。
2. 保留现有低成本分词采样，把协议／能力／分词结论分别展示；不把缺失用量补零，不以采样稳定冒充身份相符。
3. 增加独立、默认关闭的行为指纹模式，参考短回答分布、自身稳定性、最低样本数、无效样本分离等方法。参数不同的指纹不可比较；参考样本需注明来源、准确型号、采集时间、协议、提示词版本、采样及思考配置。
4. 已接入固定版本 KBF 题集的条件性参考比较，保留原统计阈值并明确其假设与移植差异；真实提供商的误报／漏报评估尚未完成，不宣称“真假概率”。

业务实现继续使用 Go，凭据仅由后端持有。统计比较可以离线读取已保存样本，不需反复调用模型。新模式在开始前展示真实请求预算，鉴权／限流失败停止，取消保留部分结果；不写健康、调度或自动删除证据。

## 源码依据

- veridrop `e87ed92`：[OpenAI 注册表](https://github.com/canarybyte/veridrop/blob/e87ed92ae37250f87a6503b321c0bc7109618008/src/relay_detector/protocols/openai/detectors/__init__.py)、[Anthropic 注册表](https://github.com/canarybyte/veridrop/blob/e87ed92ae37250f87a6503b321c0bc7109618008/src/relay_detector/protocols/anthropic/detectors/__init__.py)、[Gemini 注册表](https://github.com/canarybyte/veridrop/blob/e87ed92ae37250f87a6503b321c0bc7109618008/src/relay_detector/protocols/gemini/detectors/__init__.py)、[signature 判定](https://github.com/canarybyte/veridrop/blob/e87ed92ae37250f87a6503b321c0bc7109618008/src/relay_detector/protocols/anthropic/detectors/thinking_signature.py)。
- modelprint `87b529d`：[中文分词差分](https://github.com/unclecode/modelprint/blob/87b529d9c739b2703a2724788609ac26553cb10e/probes/tok-chinese.js)、[logprobs 探测](https://github.com/unclecode/modelprint/blob/87b529d9c739b2703a2724788609ac26553cb10e/probes/lp-geometry.js)。
- llm-fingerprint-detector `94dc446`：[采样](https://github.com/ToseaAI/llm-fingerprint-detector/blob/94dc44686a553595581b6d06405c17f070df9b8d/src/sampler.ts)、[统计与自身比较](https://github.com/ToseaAI/llm-fingerprint-detector/blob/94dc44686a553595581b6d06405c17f070df9b8d/src/stats.ts)、[阈值](https://github.com/ToseaAI/llm-fingerprint-detector/blob/94dc44686a553595581b6d06405c17f070df9b8d/src/constants.ts)。仓库引用论文的效果属于其作者声明，本轮未独立复现实验。
- KBF `68aefbb`：[覆盖率、自测基线及统计判定](https://github.com/Ooo0ption/KBF/blob/68aefbbab879469152aaac9fd988a86c30ee4739/scripts/kbf_test.py)。
- LLMmap `f661d55`：[推理实现](https://github.com/pasquini-dario/LLMmap/blob/f661d550e3409d1bdf7e00b34bcec15296c5215e/LLMmap/inference.py)、[模板与训练说明](https://github.com/pasquini-dario/LLMmap/blob/f661d550e3409d1bdf7e00b34bcec15296c5215e/README.md)。

## 已实现：行为分布采样 v1

- 独立模型检测勾选“采集行为指纹”（默认关闭），添加账号探活选择“行为指纹”。复用 Go 检测引擎、原生协议适配及临时 Key 清理，不写健康或调度证据。
- `short-answer-distribution-v1` 使用数字、颜色、硬币、方向四类任务，每类中英文各一个单元，共 8 × 25 = 200 次额外采样。每轮交错执行 8 个单元；不是连续执行完一个单元再开始下一个。探活总计 217 项，独立检测预算为账号模型组合数 ×（轮次 × 7 + 可选分词 10 + 可选行为 200）。协议兼容回退可能产生额外 HTTP 尝试，各项记录实际尝试次数。
- 仅接受完整的规定选项，统一首尾空白和英文字母大小写；拒答、解释、多个候选、截断不算有效样本。错误和未执行单独记录，不把缺失补零。原始题目、回答、请求 ID、时间、协议和返回型号随报告保存。
- 每个单元统计选项计数，至少 10 个有效样本且原始采样序号的奇偶两侧各至少 5 个才计算以 2 为底的 Jensen–Shannon divergence（0～1）。不按过滤后的索引重新分组。型号或协议缺失／变化时标记不可比；样本不完整时保留已满足覆盖要求的局部 JSD。
- 当前复用 `verification-medium-v1` 请求配置：直接 API 使用 4096 输出上限、流式、medium 思考等级，Gemini 保持默认思考配置；OAuth 沿用其固定 instructions、medium 且不指定输出上限。温度均未指定，报告明确保存 `provider_default`，不把上游默认值假定为 1。请求配置随 `parameters` 保存，实际协议在每个样本记录。
- 这是对分布采集和奇偶自身比较方法的独立 Go 实现，不是第三方 OneToken 温度／短输出设置的复现；未复制第三方代码、引入业务运行时或新增依赖。
- JSD 只描述本次采样差异，未设“真假”阈值，也未引入参考身份库。所有身份结果为无法判定；采样完整、JSD 为 0、返回型号一致均不证明模型权重或同一身份。隐藏系统提示词、网关缓存和默认温度会影响结果。
- 鉴权、限流、不支持请求停止剩余请求；取消和超时保留部分报告。接入前探活仍有 10 分钟任务上限，独立模型检测仍有 30 分钟任务上限；启动前展示额外预算，不承诺全部采样必能在保护时限内完成。

后续要做型号识别，仍需版本明确的参考数据及跨网关、跨配置的独立误报／漏报验证。本次未恢复官方对照入口或自动官方调用。
