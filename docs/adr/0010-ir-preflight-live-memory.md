# ADR-0010: Incremental IR fidelity, preflight and live file memory

日期：2026-10-22｜状态：Accepted

## 背景

V1.6 要求保真 IR、按文档能力报告和活跃内存模型，但当前显示列表仍是唯一
消费者契约。一次性引入完整 `scene` 包会把解释器、渲染器和下游同时拖入大
重构，且尚未冻结字段语义。

## 决策

1. `displaylist` 继续作为实验性 IR v0 投影，通过只增字段逐步补保真语义：
   - `Common.PaintState` 保存填充/描边 alpha、叠印模式与渲染意图；
   - `TextRun.Spans` 用字形区间表达 ActualText，不再复制到每个字形；
   - `Version` 升至 1.15，新增字段不改变旧消费者对 `Alpha` 的解释。
2. 新增 `Document.Preflight`，按 Parse/Extract/Render/Edit/Export/Verify
   分别报告支持状态与执行 Outcome；静态 `Capabilities` 不再是唯一答案。
3. 预检扫描全部页面，输出扫描页数、缺 Unicode 字形数、图像/渐变计数和
   已知近似项；它不宣称像素或规范一致性。
4. 新增活跃内存租约：
   - `Budget.MaxLiveBytes`；
   - `Tracker.Acquire/Release`、`Live`、`Peak`；
   - `Document` 持有输入文件字节租约，`Close` 幂等释放；
   - `Document.ResourceUsage` 输出当前值和高水位；
   - `Page.AcquireMemory` 与 `render.PageOptions.Memory` 把路径/文字/透明组/软蒙版/clip mask、图像解码结果和输出栅格计入同一预算；
   - 文档字体缓存按估算保留字节持有租约，预算不足时仍可使用但不缓存；
   - 图案/软蒙版展开缓存按估算字节持有文档生命周期租约，预算不足时跳过缓存并写诊断。
5. 独立公共 `scene` 包、ColorManager 和 CacheBudget 仍待后续 ADR；本 ADR
   不冻结它们的字段。页级有界 Executor 已作为 `render.RenderPages` 落地，
   固定/自适应 tile 规划与流式渲染已作为 `render.PlanTiles*`、
   `RenderTiles*` 落地。

## 后果

- 下游可以开始读取 PaintState/TextSpan，但必须容忍更多字段。
- 近似渲染仍写明确 note，不因字段存在而宣称完整合成。
- `Close` 后 `LiveBytes` 归零，`PeakBytes` 保留用于诊断。
- 预检成本与页面数线性相关，调用方应显式承担。

## 未覆盖

- 源 ICC 到透明组混合空间的完整保真；
- GID 双轨、Type3 子场景、OCG 表达式与 Backdrop；
- 场景缓存活跃租约与自适应 tile 计划；
- 写入、签名、SVG/HTML 输出。
