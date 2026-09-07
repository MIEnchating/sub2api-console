# 全局样式与组件

## 使用约定

- 页面使用 `PageLayout`、`PageHeading`、`PageActions`，页面滚动由 `PageLayout` 内容区域管理。
- 表格使用 `DataTablePanel`、`TableFilterToolbar`、`SearchField`、`FilterMenu` 和 `DataTablePagination`。
- 宽表格的空数据或无匹配状态使用 `TableEmptyState`；文案宽度以表格滚动容器为准，横向滚动时仍保持可见。
- 按钮使用 `Button`，行内图标操作使用 `TableActionButton`，刷新使用 `RefreshButton`。按压反馈不改变按钮尺寸或位置。
- 模式和标签切换使用 `SegmentedControl`。标签列表默认横向滚动；键盘导航遵守 `aria-orientation`，跳过禁用项。
- 单选和多选使用 `Select`、`MultiSelect`。菜单限制在可用视口内，长选项允许换行，列表自身负责纵向滚动。
- 弹窗和抽屉分别使用 `Dialog`、`Sheet`。标题区域为关闭按钮保留空间，长标题和说明允许换行；打开时由 Base UI 管理焦点，关闭时恢复焦点。
- 状态展示使用 `StatusBadge`。调用方的 `role`、`aria-live` 等 DOM 属性会传到实际状态元素。
- 表单控件使用 `Input`、`Textarea` 等基础组件；普通文本域仅允许纵向调整大小，自动增长模式禁止手动调整。

## 样式归属

颜色和字体令牌在 `src/theme.css`，主题预设在 `src/theme-presets.css`。Public Sans 由全局 CSS 加载，字体名称为 `Public Sans Variable`。默认亮暗主题主按钮的常态和悬停态文字对比度均由浏览器测试校验，要求至少 4.5:1。

组件尺寸、标题留白和溢出策略优先在共享组件中定义。避免通过全局 `button` 变换或覆盖 Base UI 的滚动锁来修补某一个页面。

## 回归检查

```bash
bun run typecheck
bun run lint
bun run format:check
bun run test --maxWorkers=3 --minWorkers=1
bun run build
bunx playwright install chromium
bun run test:e2e
```

浏览器测试启动独立的 3013 端口，拦截所有 API 请求，包括写入和 SSE，不使用真实后端。已有 Chromium 时可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定浏览器路径。

当前浏览器回归覆盖桌面亮色、移动端暗色、短屏筛选菜单、选中后列表滚动、宽表格空状态、字体加载、主按钮对比度、按钮按压位置和移动端抽屉焦点。截图和失败跟踪存放在 `test-results/`，不提交生成文件。
