# ADR-0014: Server Document Core 范围与 SLO

日期：2026-10-23｜状态：Accepted

## 背景

项目目标调整为 Go 服务端文档内核，而不是 Poppler 全功能替代品。服务端的主要
任务是安全接收不可信 PDF、解析和校验结构、提取文本/元数据、做预检，并在需要时
生成屏幕预览。继续维护打印色彩、写入、签名、完整排版和自研 JPX 会分散投入，
也不构成服务端内核的核心差异。

本 ADR 固定范围、非目标、API 边界和验收指标；后续变更需要新的 ADR。

## 范围

### Core

必须提供：

- PDF 对象模型、xref/xref 流、对象流、增量更新、修复
- Standard Security R2–R6 解密
- 页面树、资源、元数据、链接、大纲、页标签、附件
- 注释/Widget 元数据、AcroForm 字段树、结构树/MCID
- 内容解释、图形状态、路径/文本/图像/裁剪 IR
- 文本字符/词/行/块、搜索、ActualText、列感知、tagged 结构语义（RoleMap/ClassMap/A 属性/typed BBox/Table/List/Page/Document.Tables grid/header refs/paragraphs/nested/cross-page/heuristic merge/repeated headers/diagnostics）、RTL/mixed 词级逻辑序、NFC/NFKC 正规化、选区/搜索 quads、旋转文本分行、竖排分列/Quad、Char.Quad/CharQuads、Preflight JSON（含 failedPages）
- 预算、取消、诊断、错误分类、能力预检
- 页级并发和有界批量提取
- `ReaderAt` 输入：小文件直接读取；大文件按 1 MiB 块写入临时文件并 mmap，
  不让 Go heap 按文件大小增长。解析器仍使用随机访问视图，不做逐段增量解析。
- 纯 Go、`CGO_ENABLED=0`、标准库 + `x/*`

### Preview

必须提供：

- 屏幕预览所需路径、文字、图像、裁剪、渐变、图案、软蒙版、透明组
- 注释 `/AP` 叠加
- PNG/JPEG 输出、区域/整页/tile 渲染
- sRGB 屏幕近似，不承诺印刷正确

Preview 不承诺：

- 印刷色彩、专色、叠印、半色调、输出意图
- 完整 font shaping/hinting
- 与 Poppler 像素级一致

### Extended

通过进程或适配器接入，不进入核心默认依赖：

- Poppler：高保真渲染、表单/签名、矢量输出（`extension/poppler` 已提供单页、连续页与任意页集进程渲染）
- OpenJPEG：JPX 解码（`extension/openjpeg` 已提供进程 provider）
- Cairo：矢量/高分辨率输出（`extension/cairo` 已提供单页 SVG 进程输出）
- 其他经许可审计的 provider

Extended provider 必须报告名称、版本、许可、确定性和能力；核心不依赖它们。

## 非目标

- 自研完整 JPX/JBIG2
- 完整排版/断行/表格布局
- PDF 写入、页面编辑、表单保存
- 数字签名签署/验证
- 打印色彩、专色、叠印、输出意图
- OCR、版面分析、无障碍认证
- 与 Poppler 功能数量或像素级对等

签名、写入、打印色彩相关公开能力从 Core/Preview 能力表中移除；需要时由
Extended provider 提供。

## API

Core 稳定入口：

```go
doc, err := pdf.Open(ctx, readerAt, size, opts...)
defer doc.Close()

info := doc.Info()
page, err := doc.Page(i)
text, err := page.Text(ctx)
result, err := doc.ExtractText(ctx, pdf.ExtractOptions{Pages: []int{0, 1}})
summary, err := doc.ExtractTextEach(ctx, pdf.ExtractOptions{}, func(row pdf.PageText) error {
    return sink.Write(row)
})
report, err := doc.PreflightWithOptions(ctx, pdf.PreflightOptions{Workers: 4})
```

约束：

- 所有长任务接受 `context.Context`。
- 页错误不终止整个批量任务；错误写入结果，`ExtractResult.FailedPages` 汇总失败页，调用方决定重试。
- `Pages` 使用 0-based 索引；空表示全部。
- `Workers` 受 `Budget.MaxConcurrency` 约束；0 使用默认值。
- 文档关闭后所有页/批量操作返回 `ErrClosed`。
- 不向调用方传播 panic。

## SLO

以下为初始目标，需在冻结语料/基线上校准：

### 正确性

- 语料打开/解释/提取成功率 100%，已知损坏文件走修复或明确错误
- 不 panic；所有失败带错误码
- 批量任务单页失败不影响其他页
- `ExtractTextEach` 不保留全部页结果，回调串行执行，并返回最终失败页汇总
- 缺失 provider 返回 `Unsupported`/`NotScanned`，不伪造结果

### 资源

- 单文档峰值受 `Budget.MaxLiveBytes` 约束
- 图像/栅格/解码/字体缓存分别计量
- 批量提取遵守 `MaxConcurrency`，不按页数无限起 goroutine
- 取消后工作 goroutine 在有限时间内退出
- 大于 8 MiB 的非文件 `ReaderAt` 不产生同等大小的 Go heap 副本

### 性能

- 冻结语料上记录打开、单页文本、批量文本、预检的 P50/P95/P99
- 相对基线回归超过 10% 需要说明并附基准
- 批量提取吞吐随 `Workers` 增长，直至预算/CPU 饱和
- 不将 Preview 渲染性能与 Core 提取指标混合报告

### 安全

- 未知流长度、超大对象、深层 Form/页面树、恶意字典受预算限制
- fuzz 覆盖语法、过滤器、字体、图像、内容解释
- Extended 进程隔离崩溃/OOM，不拖垮核心
- 错误信息不包含文档正文或元数据值

## 迁移

- 删除 Core/Preview 中的 edit/export/verify 域和签名/打印色彩声明
- 保留 PNG/JPEG Preview 输出，不承诺 SVG/HTML/PDF/PS/EPS
- `extension` 继续作为 Extended 端口；核心不 import 具体 provider
- 旧 API 不保证向前兼容；按新范围直接收敛

## 验收

- Core 无 CGO 构建、race、vet、staticcheck、fuzz 通过
- 冻结语料 `ExtractText` 与单页文本结果一致
- 批量取消、页错误、并发上限测试通过
- Preview 明确标注 sRGB 和非印刷级
- Extended 缺失时核心行为可预测
