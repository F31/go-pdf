# ADR-0006: Immutable decoder factory registry

日期：2026-10-21｜状态：已采纳

## 背景

V1.6 要求公共扩展 SPI 不使用 internal 类型，并避免进程级可变注册影响并发任务。原
全局切片会让测试、批处理和多个 Document 之间互相污染。

## 决策

- `DecoderRegistry` 保存 `DecoderFactory`，构造后不可变，可被多个 Document 共享。
- `DecoderFactory.NewDecoder` 默认每个图像任务创建一个解码会话；无状态且明确并发安全的
  实现可以返回共享实例。
- Document 保存 Registry 快照；`WithOverride` 派生新 Registry，不修改共享实例。
- 新增应用级 `Engine`，一次配置 Registry/预算，批量打开多个 Document。
- 删除进程级 `RegisterExternalDecoder`；未配置 Registry 时，透传编码返回
  `ErrUnsupported`。
- 解码任务自行持有取消、预算与可变会话；Document 只保存配置快照。

## 影响

- 外部 module 可独立实现并注入 JBIG2/JPX 解码器，不依赖 internal 包。
- Registry 可安全跨文档共享，但解码器实例的线程安全由 Factory 明确声明。
- 后续 FontProvider/ColorManager 应按同一不可变 Registry + 任务会话模式设计。
