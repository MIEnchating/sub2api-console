> ## Documentation Index
> Fetch the complete documentation index at: https://platform.minimaxi.com/docs/llms.txt
> Use this file to discover all available pages before exploring further.

# 按量计费

> MiniMax按量计费定价

按量计费使用开放平台普通 API Key，并按实际用量消耗账户余额。积分是通过订阅 Key 使用的独立预付余额，资源覆盖范围与 Token Plan 相同。积分定价和使用规则请参考 [Token Plan 定价](/docs/guides/pricing-token-plan)。

## 语言模型

[立即充值](https://platform.minimaxi.com/user-center/payment/balance)

<Tabs>
  <Tab title="标准">
    | **模型**                                                                                                                                                                                                   | **输入价格**<br /> 元/百万 tokens | **输出价格**<br /> 元/百万 tokens | **缓存读取**<br /> 元/百万 tokens |
    | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :------------------------: | :------------------------: | :------------------------: |
    | **MiniMax-M3**<br />≤ 512k 输入 tokens <span className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300">永久五折</span>   |        ~~4.20~~ 2.10       |       ~~16.80~~ 8.40       |        ~~0.84~~ 0.42       |
    | **MiniMax-M3**<br />> 512k 输入 tokens\* <span className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300">永久五折</span> |        ~~8.40~~ 4.20       |       ~~33.60~~ 16.80      |        ~~1.68~~ 0.84       |
  </Tab>

  <Tab title="优先*">
    | **模型**                                                                                                                                                                                                 | **输入价格**<br /> 元/百万 tokens | **输出价格**<br /> 元/百万 tokens | **缓存读取**<br /> 元/百万 tokens |
    | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :------------------------: | :------------------------: | :------------------------: |
    | **MiniMax-M3**<br />≤ 512k 输入 tokens <span className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300">永久五折</span> |        ~~6.30~~ 3.15       |       ~~25.20~~ 12.60      |        ~~1.26~~ 0.63       |
    | **MiniMax-M3**<br />> 512k 输入 tokens <span className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-700 dark:bg-red-900/30 dark:text-red-300">永久五折</span> |       ~~12.60~~ 6.30       |       ~~50.40~~ 25.20      |        ~~2.52~~ 1.26       |

    \* 优先服务可让请求获得优先准入，从而更快响应并降低失败率。调用时将 `service_tier` 设为 `priority` 即可启用。该层级按标准价格的 1.5 倍计费。
  </Tab>
</Tabs>

| **模型**                     | **输入价格**<br /> 元/百万 tokens | **输出价格**<br /> 元/百万 tokens | **缓存读取**<br /> 元/百万 tokens | **缓存写入**<br /> 元/百万 tokens |
| :------------------------- | :------------------------: | :------------------------: | :------------------------: | :------------------------: |
| **MiniMax-M2.7**           |             2.1            |             8.4            |            0.42            |            2.625           |
| **MiniMax-M2.7-highspeed** |             4.2            |            16.8            |            0.42            |            2.625           |

<Accordion title="历史模型">
  | **模型**                     | **输入价格**<br /> 元/百万 tokens | **输出价格**<br /> 元/百万 tokens | **缓存读取**<br /> 元/百万 tokens | **缓存写入**<br /> 元/百万 tokens |
  | :------------------------- | :------------------------: | :------------------------: | :------------------------: | :------------------------: |
  | **MiniMax-M2.5**           |             2.1            |             8.4            |            0.21            |            2.625           |
  | **MiniMax-M2.5-highspeed** |             4.2            |            16.8            |            0.21            |            2.625           |
  | **MiniMax-M2.1**           |             2.1            |             8.4            |            0.21            |            2.625           |
  | **MiniMax-M2.1-highspeed** |             4.2            |            16.8            |            0.21            |            2.625           |
  | **MiniMax-M2**             |             2.1            |             8.4            |            0.21            |            2.625           |
</Accordion>

<Info>
  请注意：

  1. 计费项是token数；tokens字符比值根据使用场景的不同略有浮动，以实际消耗为准，字符数包括标点等
  2. Token与字符比（估算）：1600 中文字符约消耗 1000 tokens
</Info>
