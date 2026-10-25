# go-pdf

**纯 Go、无 CGO 的服务端 PDF 内核与屏幕预览引擎。**

当前版本：**v0.5.0**

模块：`github.com/F31/go-pdf`

go-pdf 提供从 PDF 解析、解密、内容解释、文本/结构提取，到页面预检和屏幕渲染的一套 Go API，同时保留 Poppler、OpenJPEG、JBIG2 等外部引擎作为可选的 Extended 后端。

---

## 产品简介

go-pdf 面向服务端文档处理，而不是桌面 PDF 阅读器或打印排版工具。

它解决四类问题：

- **读得开**：xref/对象流/增量更新/损坏修复、R Standard R2–R6 解密。
- **读得懂**：文本、字符位置、阅读顺序、结构树、表格、链接、表单字段、附件。
- **看得见**：路径、文字、图像、ICC、透明、软蒙版、Pattern、Mesh 的屏幕预览。
- **管得住**：资源预算、取消、页级并发、流式回调、显式错误和诊断。

典型使用场景：

- 文档中台、知识库、搜索索引、内容审核
- 合同/报告/票据的文本与结构抽取
- 批量文档预检、元数据归档、附件读取
- 服务端缩略图、页面预览、区域截图
- 用 Go 构建的文档处理流水线

---

## 定位

### 产品档位

| 档位 | 定位 | 主要能力 |
|---|---|---|
| **Core** | 服务端 PDF 读取与理解 | 解析、解密、页面树、内容解释、文本、结构/表格、预检、预算与取消 |
| **Preview** | 屏幕预览 | sRGB 路径/文字/图像、透明、软蒙版、Pattern、Mesh、区域/整页渲染 |
| **Extended** | 可选高保真与外部能力 | Poppler 光栅、pdftocairo SVG、OpenJPEG JPX、jbig2dec JBIG2、外部颜色/字体 provider |

Profile 是发布承诺和依赖边界，不是不同的解析器。`pdf.CapabilitiesFor` 和 `pdf capabilities -profile core|preview|extended` 可查询静态能力表。

### 明确边界

go-pdf v0.5.0 **不承诺**：

- PDF 写入、页面编辑、表单填写保存
- 数字签名生成/验证
- 打印色彩、PDF/X、专色/叠印
- Adobe/Poppler 像素级一致
- FreeType 级字体 hinting
- OCR、PDF/A 认证、无障碍认证
- 自研完整 JBIG2/JPX、CFF/Type1、完整 shaping

这些能力需要其他引擎、单独产品线或新的 ADR。缺失能力不会伪造结果：返回 `ErrUnsupported`、记录诊断，或明确标记为近似。

### 质量定位

Preview 目标是“屏幕上看对、缺失内容可解释”，不是“和 Adobe 完全一样”。

当前 v0.5.0 基线：

- 全量 10,866 页语料：**0 render_error**
- 英文文本位置抽样：词框边缘误差 **P95 0.012 pt**
- CI 覆盖 Linux/macOS/Windows、race、vet、staticcheck、govulncheck、fuzz
- 静态 stripped 二进制约 **13.1 MB**
- Core/Preview/Extended 质量边界见 [docs/perf/p4-baseline.md](docs/perf/p4-baseline.md)

---

## 技术架构

```text
                        ┌──────────────────────────────┐
                        │        Application           │
                        │  Go service / CLI / worker   │
                        └──────────────┬───────────────┘
                                       │
             ┌─────────────────────────┴─────────────────────────┐
             │                                                   │
      ┌──────▼──────┐                                    ┌───────▼───────┐
      │  pdf core   │                                    │ render         │
      │ Open/Page   │                                    │ Preview / tiles│
      │ Text/Tables │                                    │ Raster         │
      │ Preflight   │                                    └───────┬───────┘
      └──────┬──────┘                                            │
             │                                                   │
      ┌──────▼──────────────┐                            ┌───────▼────────┐
      │ internal/syntax     │                            │ displaylist IR │
      │ filter / xref       │                            │ Version 2.0    │
      │ crypt / document    │                            └───────┬────────┘
      └──────┬──────────────┘                                    │
             │                                                   │
      ┌──────▼───────────────────────────────────────────────────▼───────┐
      │ content interpreter: graphics state, paths, images, fonts,      │
      │ color, shading, patterns, transparency, text                    │
      └──────┬──────────────────────────────────────────────────────────┘
             │
      ┌──────▼──────────────────────────────────────────────┐
      │ text/            font/            color/            │
      │ chars/words      sfnt/CMap        ICC/sRGB          │
      └─────────────────────────────────────────────────────┘

      Optional Extended adapters (out of process):
      extension/poppler  extension/cairo  extension/openjpeg  extension/jbig2
```

### 数据流

```text
bytes / ReaderAt
  → syntax + filters + xref
  → document/page tree
  → content interpreter
  → displaylist
      ├→ text extraction / selection / tables / structure
      ├→ preflight / diagnostics
      └→ render (whole page, region, tiles, streaming pages)
```

### 设计约束

- Core 只使用 Go 标准库和 `golang.org/x/*`。
- Core 默认 `CGO_ENABLED=0`，不链接 Poppler、OpenJPEG、FreeType、lcms2。
- Extended 通过独立进程、CGO/build tag 或显式 provider 接入，不进入 Core 默认依赖。
- 错误不包含文档正文、元数据值或流内容。
- 近似渲染必须可诊断，不允许静默缺图/缺字。
- `displaylist.Version` 当前为 **2.0**，只保证增字段的 Minor 兼容。

---

## 功能特性

### 解析与安全

- 经典 xref、xref stream、object stream、增量更新、混合引用、损坏修复
- Flate、LZW、ASCII85、ASCIIHex、RunLength、Predictor
- Standard security R2–R6：RC4-40/128、AES-128、AES-256
- 用户/所有者密码、SASLprep、加密边界
- `ReaderAt` 输入；`*os.File` 优先 mmap；大于阈值自动 spool 到临时文件
- 文件、对象、流、解码总量、嵌套、操作符、路径、图像像素、栅格和活跃内存预算

### 文档语义

- 页面树、继承属性、`CropBox`/`Rotate`/`UserUnit`
- 链接、大纲、页标签
- 注释/Widget 元数据与 AP 渲染
- AcroForm 字段树、值、Flags、页面与 Widget Rect
- Tagged PDF 结构树、RoleMap/ClassMap、MCID/StructRef
- 表格视图：行列、span、HeaderRefs、段落、嵌套表、跨页合并、启发式合并
- 附件列表与流式写入
- XMP 核心属性
- 预检：Parse/Extract/Render 状态、失败页、诊断码

### 文本

- 简单字体、Type0/CID、Identity-H/V、Type3、嵌入 TrueType/OpenType
- 预定义 CJK CMap 与嵌入 CMap、1/2/4 字节 codespace
- ToUnicode、ActualText、NFC/NFKC
- 字符、词、行、块、列感知阅读顺序
- RTL/mixed、90°/270° 旋转、竖排 RTL/LTR
- `Char.Origin/Advance/Bounds/Rotation/Quad`
- 选区/highlight quads、搜索命中与 `CharQuads`
- 文本位置以 `Glyph.Matrix`、CTM 缩放和 `FontInfo.Ascent/Descent` 计算

### Preview 渲染

- 路径填充/描边、EvenOdd、虚线、cap/join、裁剪
- 图像、ImageMask、色键 Mask、SMask
- CalGray/CalRGB/Lab、ICCBased、Indexed、Separation/DeviceN
- 轴向/径向渐变、function shading
- tiling pattern、shading pattern、type 4–7 mesh + Function
- 透明组、soft mask、backdrop、knockout、16 种混合模式
- OCG/OCMD 可见性
- 整页、区域、固定/自适应 tile、页级流式渲染
- 输出 PNG/JPEG，支持透明背景
- `MinStrokeWidth` 显式控制最小设备像素线宽（默认 1px）

### Server 与 CLI

- `Document.ExtractText`：有界并发、逐页错误、失败页汇总
- `Document.ExtractTextEach`：流式回调，不保留全部页文本
- `Document.PreflightWithOptions`：批量预检
- `render.RenderPagesEach`：有界并发、按页释放栅格
- `render.RenderTiles` / `RenderPageTiles`：tile 流式输出
- CLI：`info`、`text`、`search`、`render`、`svg`、`links`、`annotations`、`outlines`、`labels`、`forms`、`tables`、`attachments`、`preflight`、`capabilities`、`compare`

### Extended

- `extension/poppler`：pdftoppm 高保真光栅
- `extension/cairo`：pdftocairo SVG
- `extension/openjpeg`：opj_decompress JPX
- `extension/jbig2`：jbig2dec JBIG2
- `extension.Registry`：image/color/font provider
- 所有进程 adapter 有输出上限、超时、临时目录清理、错误截断、版本/许可/确定性报告
- Core 不导入具体 provider；`scripts/check-core-deps.sh` 强制检查

---

## 快速开始

要求 Go 1.25+；发布构建建议 Go 1.26.10+。

### 打开文档

```go
doc, err := pdf.OpenFile(ctx, "in.pdf") // Unix 优先 mmap
if err != nil {
    return err
}
defer doc.Close()

page, err := doc.Page(0)
if err != nil {
    return err
}
w, h := page.Size()
_, _ = w, h
```

内存输入：

```go
doc, err := pdf.Open(ctx, readerAt, size, pdf.WithPassword("secret"))
```

### 提取文本

```go
pageText, err := page.Text(ctx)
if err != nil {
    return err
}
fmt.Println(pageText.Text)

for _, ch := range pageText.Chars {
    fmt.Println(ch.Text, ch.Origin.X, ch.Origin.Y, ch.Bounds)
}
```

批量提取：

```go
result, err := doc.ExtractText(ctx, pdf.ExtractOptions{
    Workers: 4,
})
```

### 预检

```go
report, err := doc.PreflightWithOptions(ctx, pdf.PreflightOptions{
    Workers: 4,
})
fmt.Println(report.Pages, report.Failed, report.FailedPages)
```

### 区域渲染

```go
img, err := render.RenderRegion(ctx, page, displaylist.Rect{
    X1: 300, Y1: 200,
}, render.PageOptions{
    DPI:         220,
    Background:  "white",
    IncludeText: true,
    Memory:      page,
})
```

需要显式管理输出栅格生命周期时使用 `RenderRegionLease` / `RenderPageLease`，并调用 `Release`。

### CLI

```bash
CGO_ENABLED=0 go build -o pdf ./cmd/pdf

./pdf info -json in.pdf
./pdf text -page 1 in.pdf
./pdf text -jsonl in.pdf
./pdf search "invoice" in.pdf
./pdf render -page 1 -dpi 150 -o page.png in.pdf
./pdf preflight in.pdf
./pdf capabilities -profile core
```

---

## 部署配置

### 构建静态二进制

```bash
GOTOOLCHAIN=go1.26.10 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o pdf ./cmd/pdf
```

Linux amd64 静态 stripped 二进制约 **13.1 MB**。

发布构建可输出到 `dist/`（已忽略）；按目标 OS/Arch 交叉编译并记录 SHA256。

### 运行时预算

`pdf.Budget` 默认值：

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `MaxFileSize` | 512 MiB | 输入文件上限 |
| `MaxObjects` | 5,000,000 | 对象数量上限 |
| `MaxStreamBytes` | 256 MiB | 单流解码上限 |
| `MaxTotalDecoded` | 2 GiB | 单文档累计解码上限 |
| `MaxInflateRatio` | 200 | 压缩比上限 |
| `MaxXRefChain` | 256 | xref 链长度 |
| `MaxPageTreeDepth` | 64 | 页面树深度 |
| `MaxFormDepth` | 16 | Form XObject 嵌套 |
| `MaxNestingDepth` | 64 | 内容/对象嵌套 |
| `MaxOperators` | 20,000,000 | 单页操作符上限 |
| `MaxPathSegments` | 5,000,000 | 单页路径段上限 |
| `MaxImagePixels` | 256,000,000 | 图像像素上限 |
| `MaxFunctionSteps` | 100,000 | Function 计算步数 |
| `MaxConcurrency` | GOMAXPROCS | 默认并发上限 |
| `MaxRasterPixels` | 64,000,000 | 单次栅格像素上限 |
| `MaxLiveBytes` | 1 GiB | 活跃内存上限 |

按业务调整：

```go
b := pdf.DefaultBudget
b.MaxFileSize = 128 << 20
b.MaxLiveBytes = 512 << 20
b.MaxConcurrency = 4

doc, err := pdf.OpenFile(ctx, path, pdf.WithBudget(&b))
```

### 打开选项

| 选项 | 作用 |
|---|---|
| `WithRepair(bool)` | 启用/关闭损坏修复 |
| `WithBudget(*Budget)` | 资源预算 |
| `WithPassword(string)` | R2–R6 密码 |
| `WithHiddenLayers(bool)` | 是否包含隐藏 OCG |
| `WithPreserveForms(bool)` | 保留透明 Form group |
| `WithColorManager(ColorManager)` | 外部 ICC/CMYK 转换 |
| `WithExtensionRegistry(*extension.Registry)` | 图像等 provider |

### 渲染选项

| `PageOptions` | 说明 |
|---|---|
| `DPI` | 默认 220，上限 300 |
| `MaxPixels` | 默认 64,000,000 |
| `Background` | `"white"` 或 `"transparent"` |
| `IncludeText` | 默认关闭；开启后渲染字体轮廓 |
| `Fonts` | 显式字体 provider，保证主机无关 |
| `FontEngine` | 外部字体 provider |
| `Annotations` | 注释 AP 叠加策略 |
| `Memory` | 页级/自定义内存预算 |
| `MinStrokeWidth` | 最小设备像素线宽；默认 1px，负值关闭非零线宽下限 |

可复现生产输出必须显式配置字体，例如：

```go
opts := render.PageOptions{
    DPI:         150,
    IncludeText: true,
    Fonts:       render.BuiltinFonts(), // 或固定字体目录
}
```

`IncludeText` 未设置 `Fonts` 时默认使用主机系统字体，结果依赖主机环境。

固定字体目录：

```go
sys, err := render.SystemFonts("/opt/fonts", "/srv/fonts")
if err == nil {
    opts.Fonts = render.ChainFonts(sys, render.BuiltinFonts())
}
```

### 服务端部署建议

1. 每个请求设置 context 超时，所有 Open/Extract/Render 都传入该 context。
2. 为不可信上传设置较小的 `MaxFileSize`、`MaxLiveBytes`、`MaxConcurrency`。
3. 批量提取使用 `ExtractText` 或 `ExtractTextEach`，不要无界 goroutine。
4. 整册预览使用 `RenderPagesEach` 或 tiles，避免整本文档栅格驻留。
5. 生产字体固定，不要依赖主机系统字体。
6. Preview 只用于屏幕预览；打印/高保真走 Poppler Extended。
7. Extended 进程放在受控子进程/容器中，设置超时、临时目录和输出上限。
8. 监控 `Unsupported`、`Approximate`、失败页和 render error，不要只看平均 PSNR。

### Extended provider 示例

```go
r, err := poppler.New("")
if err != nil {
    return err
}
img, err := r.RenderPage(ctx, "in.pdf", 0)
```

或者通过 `extension.Registry` 注册 JPX/JBIG2 图像 provider，再用 `pdf.WithExtensionRegistry` 传入文档。

---

## 质量与发布

本仓库固定四层质量边界：

| 层 | 基准 |
|---|---|
| Core | `docs/perf/p1-baseline.md` |
| Preview | `docs/perf/p2-baseline.md`、`docs/perf/poppler-corpus-baseline.md` |
| Extended | `docs/perf/p3-baseline.md` |
| 发布 | `docs/perf/p4-baseline.md`、`docs/release.md` |

常用检查：

```bash
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
./scripts/check-core-deps.sh
./scripts/check-licenses.sh
./scripts/verify.sh
PERF_GATE=1 ./scripts/release-check.sh
```

跨引擎与语料：

```bash
./scripts/cross-engine.sh in.pdf 1
./scripts/fetch-english-corpus.sh /tmp/english-corpus
./scripts/english-text-position.sh /tmp/english-corpus
POPPLER_CORPUS_SHARDS=16 ./scripts/corpus-poppler-full.sh
```

Poppler 像素全等不是发布门禁；语义正确性、显式诊断、资源边界和可复现性优先。

---

## 已知限制

- 无 PDF 写入、页面编辑、表单保存、数字签名。
- 无打印色彩/PDF-X/专色/叠印。
- 无内置 JBIG2/JPX/G3 2-D；通过 Extended provider 或 Unsupported 处理。
- 无完整 CFF/Type1、完整 shaping、FreeType hinting。
- 无 OCR、PDF/A 认证、无障碍认证。
- Preview 不是 Adobe/Poppler 像素级替代品。
- 默认系统字体会导致跨主机差异；可复现输出请使用固定字体。
- v0.5.0 仍是 pre-1.0；公共 API 和 `displaylist` 尚未承诺长期稳定。

---

## 目录

```text
pdf.go / document / page / options   公共 API
displaylist/                          显示列表契约（Version 2.0）
render/                               屏幕渲染 API 与 tile/page 执行器
text/                                 字符、词、行、块与选区
extension/                            可选 provider 接口与注册表
extension/poppler/                    pdftoppm 高保真光栅
extension/cairo/                      pdftocairo SVG
extension/openjpeg/                   opj_decompress JPX
extension/jbig2/                      jbig2dec JBIG2
cmd/pdf/                              CLI
internal/syntax/                      词法、对象、xref、重建
internal/filter/                      过滤器与 Predictor
internal/doc/                         页面树、页面空间、文档语义
internal/font/                        字体、CMap、字宽、轮廓
internal/color/                       ICC、色彩空间、sRGB
internal/content/                     内容流解释器
internal/budget/                      资源预算
internal/diag/                        诊断
testdata/                             合成语料与金样
scripts/                              质量、发布、语料工具
```

---

## 许可

Apache-2.0。第三方依赖、内嵌资源和语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。

发布与支持边界见 [docs/release.md](docs/release.md)。
