# go-pdf

纯 Go、无 CGO 的 PDF 解析与内容提取组件。G0 提供文档打开、页面遍历、页面空间、元数据、资源预算与诊断基础；G1 提供内容流解释与基础显示列表；G2 增量提供简单字体与 Type0/CID 的 Unicode、字宽和预定义 CJK CMap。

## 状态

当前为 **Server Document Core**：Core 负责解析/IR/文本/结构，Preview 提供屏幕预览，Extended 通过进程或适配器接入 Poppler/OpenJPEG/Cairo 等后端。范围与 SLO 见 [ADR-0014](docs/adr/0014-server-document-core.md)。

- 经典 xref、xref 流、对象流、增量更新、混合引用、重建
- Flate、LZW、ASCII85、ASCIIHex、RunLength、Predictor
- 页面树、继承属性、`CropBox`/`Rotate`/`UserUnit` 页面空间
- 内容流解释：图形状态、路径、裁剪、基础颜色、文本状态
- 简单字体与标准 14 字体宽度、WinAnsi/StandardEncoding、简单字体 `/ToUnicode`
- Type0/CIDFont + Identity-H/V、ToUnicode CMap、CID `/W`/`/DW` 宽度
- 预定义 CMap：简体中文 `UniGB`/`GBK`/`GB-EUC`、Big5 `UniCNS`/`B5`/`B5pc`/`ETen`、日文 `UniJIS`/`90ms-RKSJ`/`90msp-RKSJ`/`90pv-RKSJ`/`EUC`/`H`、韩文 `UniKS`/`KSCms-UHC`/`KSC-EUC`/`KSCpc-EUC`/`KSC-Johab`（H/V）
- 嵌入 CMap 流与 `usecmap` 继承、1/2/4 字节 codespace、CID/Unicode 映射
- 间接 `/Resources /Font`、`WMode`、`W2`/`DW2` 竖排宽度与位置
- Type3 `CharProcs` 字形路径、`FlagFromType3`、`FontMatrix` 字宽与 ToUnicode
- `x/image/font/sfnt` 字体度量、`FontMetrics.Advances`、`CIDToGIDMap` 与文档级字体缓存
- 嵌入 TrueType/OpenType 字形轮廓读取（1/1000 文本空间，Y 向上，二次曲线转三次），覆盖 CIDToGID 映射
- Form XObject 展开：`Matrix`、`BBox` 裁剪、资源继承、递归/循环与深度预算
- Image XObject 与内联图像：`ImageRef.Raw/Decode`、DeviceGray/RGB/CMYK、1/2/4/8/16 bpc、Decode 数组、ImageMask、DCT/JPEG、CCITT G3 1-D/G4
- JBIG2/JPX 通过不可变 `DecoderFactory` Registry 注入；`pdf.NewEngine` 可跨文档复用配置，不再提供进程级全局注册
- Standard security R2-R6：RC4-40/128、AES-128、AES-256；`WithPassword` 支持用户/所有者密码与 SASLprep 归一化
- ExtGState 常量透明度 `ca`/`CA`、混合模式记录、线宽/端点/连接/虚线状态
- 颜色空间：CalGray/CalRGB/Lab、ICCBased matrix/TRC（RGB/Gray）、LUT8/LUT16/mAB CLUT（CMYK→Lab/XYZ）到 sRGB、Indexed、Separation/DeviceN；PDF Function 类型 0/2/3/4
- 轴向/径向渐变：`ShadingType 2/3`、64 色标采样、`Extend`、CTM 变换与裁剪
- 可选内容可见性：`/OCProperties` 的 `BaseState`/`ON`/`OFF`、`BDC /OC`、OCMD `/P`/`/VE` 与 `WithHiddenLayers`；OCG 名称写入 `Common.Layer`
- Tiling 图案；mesh shading 支持 type 4–7 顶点/patch 与 Function 颜色，流大小受预算限制，超限记录诊断
- `render.RenderPage` 按 CropBox、旋转和 UserUnit 后的页面空间整页渲染；`EncodePNG`/`EncodeJPEG` 输出图像
- `render.RenderRegion` 区域渲染：路径填充/EvenOdd、虚线、butt/round/projecting 端点与 miter/bevel/round 连接、ImageMask 模板、彩色/未着色 Tiling 图案基础平铺、type 4–7 mesh/function shading、Alpha/Luminosity 软蒙版（含 /BC backdrop）、透明组离屏合成（保留 BlendColorSpace、Backdrop；隔离/非隔离组按 sRGB 扁平化，DeviceGray 混合空间仍可生效；knockout 组按元素独立合成，非隔离组元素以当前 backdrop 为初始背景）、图像与色键遮罩、裁剪、常量 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字；DPI 默认 220、上限 300；`PageOptions.MinStrokeWidth` 控制最小设备像素线宽（默认 1px，负值关闭非零线宽下限）；`IncludeText` 默认使用本机系统字体，再回退到内置 Go sans/mono bold/italic；也可通过 `render.FontProvider`/`render.ChainFonts` 显式指定以保证主机无关输出
- 图像解码失败（如未注册 JBIG2/JPX）在区域渲染中显式返回 `ErrUnsupported`，禁止静默缺图
- ImageMask、色键 Mask、SMask 关联输出；`Page.Images` 与 `Page.Fonts` 清单
- 显示列表 `Version 2.0`；`PaintState` 保留填充/描边 alpha，`TextSpan` 用字形区间映射 ActualText；路径形状提示、`Source` 操作符追溯、相对几何 `Sig` 与 `Page.Scanned`
- `Page.Walk` / `Page.DisplayList` 输出文字、路径、图像与裁剪
- `Page.Text` 与公共 `text` 包提供字符、词、行、块与 ActualText 文本索引；按行段/列检测输出阅读顺序，支持 tagged-PDF `StructOrder`、纯/mixed RTL 行逻辑序和 NFC/NFKC 正规化，`Page.Columns`、`Page.Blocks`、`Line.Column`、`Line.Block`、`Line.Direction` 可查询结构；`Page.Select` 提供文本范围、逐行高亮 quads 与字符映射；`Char.Rotation`/`Char.Quad` 保留旋转文字方向与定向包围盒；90°/270° 文本按旋转基线分行；竖排默认从右到左分列，可用 `Options.VerticalOrder` 切换；竖排字符带 90° Rotation 与纵向 Quad；Selection/Hit 同时返回 `CharQuads`
- `Page.Links` 解析 Link 注释的矩形、URI、直接/命名 GoTo 目标；`displaylist.Page.Links` 同步填充
- `Document.Outlines` 解析大纲树的标题、页目标、URI 与子节点
- `Page.Annotations` 解析通用注释与 Widget 的 Rect、Subtype、Contents、URI、目标页、字段名/值/类型
- 注释 `/AP /N` 外观流展开到 `AnnotationAppearance`；`RenderRegion` 可用 `Annotations: AnnotationView` 叠加，并按 Invisible/Hidden/NoView 标志过滤
- `Document.FormFields` 解析 AcroForm 字段树、继承类型、值、Flags、页面与 Widget Rect
- `Document.StructureTree` 解析轻量 Tagged PDF 结构树、角色、标题、页面与 MCID
- `Document.Attachments` 解析 EmbeddedFiles 名称树并读取嵌入文件
- Core/Preview/Extended 产品档位（ADR-0012）；`pdf.CapabilitiesFor` 与 `pdf capabilities -profile` 输出分档能力清单
- `pdf.Capabilities()` 输出 Preview 静态功能支持/近似/外部/不支持清单
- 标记内容 `BDC` 的 `/MCID` 写入显示项 `Common.MCID`；`DisplayList` 按结构树回填 `StructRef`；结构元素保留 Type/RawType、Title/Alt/ActualText/Lang/ID、Classes、RoleMap/ClassMap 映射、`/A` 属性树与类属性；提供 `Attribute`、`BBox`、`ColSpan`、`RowSpan`、`Scope`、`Placement`、`ListNumbering` 访问器与 `AsTable` / `Page.Tables` / `Document.Tables` 轻量表格视图（行列位置、span、HeaderRefs、段落、嵌套表格、跨页同 ID 合并、`TablesMerged` 启发式合并、重复表头去重与 span 诊断）
- `Page.Label` / `Document.PageLabel` 解析 `/PageLabels` 的十进制、罗马数字、字母与前缀
- `Document.Preflight` / `PreflightWithOptions` 按 Parse/Extract/Render 报告状态与 Outcome，支持页选择、受限 workers 和稳定 JSON 输出
- `extension/poppler` 通过独立进程调用 pdftoppm，作为可选 Extended 高保真渲染后端；支持 `RenderPage`、连续页 `RenderEach` 与任意页集 `RenderPages`，不链接 GPL 代码
- `extension/cairo` 通过独立进程调用 pdftocairo 输出单页 SVG，作为可选 Extended 矢量输出
- `extension/openjpeg` 通过独立进程调用 opj_decompress 解码 JPX，注册为可选图像 provider
- `pdf.WithExtensionRegistry` 接入 `extension.Registry` 图像 provider；`DecodeSamples` 保留源样本，`Decode` 兼容为 image.Image；与旧 `DecoderRegistry` 重复配置返回参数错误
- `displaylist.FontInfo.Source` 暴露嵌入字体程序，`Paint.Profile` 保留 ICCBased 原始 profile；`render.Options.FontEngine` 可消费字体 provider
- 活跃文件内存预算 `MaxLiveBytes`；`Document.ResourceUsage` 输出 Live/Peak/Decoded/Objects
- 统一预算、诊断、`*pdf.Error`、`render.Renderer`/Profile/Capabilities、图像质量指标/阈值门禁与 `displaylist.Version 2.0`
- `Attachment.WriteTo` 将嵌入文件直接解码到调用方 writer，避免先聚合成完整 `[]byte`
- `Document.ExtractText` 批量提取：页选择、受限 workers、逐页错误、失败页列表/诊断码、JSONL CLI
- `Document.ExtractTextEach` 流式回调提取：不保留全部页文本，回调串行、完成顺序输出，结束时返回 `Processed/Succeeded/Failed/FailedPages/Failures` 汇总
- `Document.PreflightWithOptions` 批量预检：页选择、受限 workers、并发扫描

预定义 CMap 中少数多字符 Unicode 目标（当前 1,878 条兼容序列）未展开；变体选择符归并到首字符。

不在 Core/Preview 范围：CCITT G3 2-D/无 EOL、自研 JBIG2/JPX（G3 2-D 与 JBIG2/JPX 可通过 Registry 注入）、CFF/Type1、精确 knockout、写入、签名、打印色彩与矢量输出。缺失能力写诊断或返回 `ErrUnsupported`，不会伪造结果。

## 使用

```go
doc, err := pdf.OpenFile(ctx, "in.pdf") // Unix 上优先 mmap，Close 释放映射
// pdf.Open(ctx, file, size) 在 reader 为 *os.File 时同样使用 mmap
if err != nil {
    return err
}
defer doc.Close()

n := doc.NumPages()
pg, err := doc.Page(0)
if err != nil {
    return err
}
w, h := pg.Size()
_ = w
_ = h
```

内存输入：

```go
doc, err := pdf.Open(ctx, readerAt, size)
```

区域渲染：

```go
img, err := render.RenderRegion(ctx, pg, displaylist.Rect{X1: 300, Y1: 200}, render.PageOptions{
    DPI: 220, Background: "white", IncludeText: true, Memory: pg,
})
```

系统字体回退默认启用：`IncludeText` 未指定 `Fonts` 时使用本机系统字体，
再回退到内置 Go 字体。需要主机无关输出或自定义目录时显式设置：

```go
sys, err := render.SystemFonts("/opt/fonts", "/srv/fonts")
if err == nil {
    opts.Fonts = render.ChainFonts(sys, render.BuiltinFonts())
}
```

`SystemFonts()` 扫描平台 TTF/OTF/TTC/OTC，按 Family/Style 匹配；CJK 字符优先尝试 CJK 字体，并按 SC/TC/JP/KR locale 与字符脚本排序，缺字时继续尝试后续候选；`ChainFonts` 可再接 CJK 或业务字体 provider。没有 Unicode cmap 的简单嵌入字体子集会被拒绝用于轮廓，自动进入回退。

生产颜色扩展：

```go
engine, _ := registry.ColorEngine("lcms")
cm := pdf.NewColorManagerFromEngine(engine)
defer cm.(interface{ Close() error }).Close()
doc, err := pdf.Open(ctx, readerAt, size, pdf.WithColorManager(cm))
```

`ColorEngine` 的 transform 按源空间/profile 缓存，目标为 DeviceRGB/sRGB；Indexed、Separation、DeviceN 会使用同一颜色管理路径。

可用选项：`WithRepair`、`WithBudget`、`WithPassword`（Standard R2-R6）、`WithHiddenLayers`（是否包含隐藏图层）、`WithPreserveForms`（保留 Form XObject 透明组）、`WithColorManager`（DeviceCMYK/ICCBased 外部转换）、`WithExtensionRegistry`（显式注册扩展 provider；源样本路径保留 plane，`Decode` 兼容路径适配到 image.Image）。

`Open` 对 `*os.File` 使用 mmap；其他 `ReaderAt` 小于 8 MiB 直接读取，大于阈值以 1 MiB 块写入临时文件后映射，避免按文件大小增长 Go heap；分块复制响应 context 取消。渲染默认使用页面所属文档的活跃内存预算；`render.PageOptions.Memory` 可覆盖为其他 `MemoryBudget`。路径/文字/透明组/软蒙版/clip mask、图像解码结果、输出栅格、字体缓存与图案/软蒙版缓存都会计费。`render.RenderPageLease` 返回需 `Release` 的 `Raster`；`render.RenderPages` 保留请求顺序并返回全部栅格；`render.RenderPagesEach` 提供有界并发流式回调，回调结束即释放页面栅格，适合服务端避免整册驻留；`render.RenderTiles`/`RenderPageTiles` 以固定像素块流式输出并复用同一份页面显示列表，单块租约在回调返回后释放；`render.PlanTiles`/`PlanTilesAdaptive` 预计算网格；`RenderTilesTo`/`RenderPageTilesTo` 可直接组装到调用方画布。

## 构建与测试

构建需要 Go 1.25+。

```bash
CGO_ENABLED=0 go build ./...
go vet ./...
go test -race ./...
go test ./... -run=^$ -bench=. -benchmem
./scripts/bench-server.sh
go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime=30s
POPPLER_QUALITY=1 go test ./render -run TestPopplerReferenceQuality -v
POPPLER_CORPUS=1 go test ./render -run TestPopplerCorpusQuality -v -timeout 20m
POPPLER_CORPUS=1 POPPLER_CORPUS_PAGES=0 \
  POPPLER_CORPUS_REPORT=/tmp/poppler-corpus.csv \
  go test ./render -run TestPopplerCorpusQuality -v -timeout 24h
```

全量分片执行：

```bash
POPPLER_CORPUS_SHARDS=16 ./scripts/corpus-poppler-full.sh
```

脚本只构建一次测试二进制，按页块动态调度，默认使用 16 个 worker，
Poppler 输出 PPM，合并为 `/tmp/poppler-corpus/poppler-corpus.csv`。

可调参数：

- `POPPLER_CORPUS_SHARDS`：并行 worker 数，默认 16。
- `POPPLER_CORPUS_CHUNK_PAGES`：每个任务的页数，默认 128。
- `POPPLER_CORPUS_CHUNK`：单次 pdftoppm 批量页数，默认 32。
- `POPPLER_CORPUS_FORMAT=ppm|png`：默认 ppm。
- `POPPLER_CORPUS_REF_CACHE=/path`：缓存 Poppler 参考图，供重复回归。
- `POPPLER_CORPUS_FONT_DIR=/path1:/path2`：固定字体目录；默认使用 `render.BuiltinFonts()`。
- `POPPLER_CORPUS_SYSTEM_FONTS=1`：使用主机字体，报告标记 host-dependent。
- `POPPLER_CORPUS_SAMPLE_CHUNKS=N`：每 N 个页块抽 1 个。
- `POPPLER_CORPUS_PAGE_START/PAGE_END`：限定页范围，便于单页排查。

`TestPopplerReferenceQuality` 和 `TestPopplerCorpusQuality` 需要本机
`pdftoppm`，并分别由 `POPPLER_QUALITY=1`、`POPPLER_CORPUS=1` 显式启用；
默认 CI 不依赖 Poppler。`POPPLER_CORPUS_PAGES=0` 表示全部页面，
`POPPLER_CORPUS_REPORT` 输出逐页 CSV；报告可能包含本地文件名，不应提交。
语料基线与指标分层见 `docs/perf/poppler-corpus-baseline.md`、`docs/perf/quality-layers.md`。

依赖白名单检查：

```bash
./scripts/check-deps.sh
```

本地私有语料放在 `docs/pdf/`（已被 `.gitignore` 排除，不会提交）。存在时运行：

```bash
go test -run TestLocalCorpus -v -timeout 20m
```

该测试会打开目录内全部 PDF、遍历页面并汇总加密/损坏/预算等分类；CI 中目录不存在时自动跳过。

## 目录

```text
pdf.go document/page/options/errors   公共 API
displaylist/                          显示列表契约
render/                               区域渲染公共 API
text/                                 字符/词/行文本索引
extension/                            可选 provider 接口、不可变注册表与 legacy 适配器
extension/poppler/                   可选 Extended pdftoppm 进程后端
extension/cairo/                     可选 Extended pdftocairo SVG 进程后端
extension/openjpeg/                  可选 Extended opj_decompress JPX provider
cmd/pdf/                              CLI（info[-json]/text/search[-jsonl]/render/svg/links[-jsonl]/annotations[-jsonl]/outlines[-json]/labels[-json]/forms[-json]/tables[-jsonl,-json]/attachments[-json,-extract]/preflight）
internal/syntax/                      词法、对象、xref、重建
internal/filter/                      过滤器与 Predictor
internal/doc/                         页面树、页面空间、元数据
internal/font/                        字体、CMap、字宽（data/ 为嵌入映射）
internal/budget/                      资源预算
internal/diag/                        诊断收集
internal/testkit/                     测试 PDF 构造工具
testdata/                             语料与金样
```

## 性能基线

批量文本一致性可用 `CORPUS_CONSISTENCY=1 go test . -run TestLocalCorpusBatchConsistency` 在本地语料上校验。Server Core 批量提取/预检基线见
[`docs/perf/server-core-baseline.md`](docs/perf/server-core-baseline.md)。Poppler 24.02 的本机实测体积、RSS、吞吐与输出体积见
[`docs/perf/poppler-24.02-baseline.md`](docs/perf/poppler-24.02-baseline.md)。该基线用于同机同语料对比，
不是通用厂商基准。

## 设计约束

- 仅标准库与 `golang.org/x/*`；新增依赖必须走 ADR 与许可审计。
- 当前外部依赖：`golang.org/x/image`（sfnt 度量）、`golang.org/x/text`（SASLprep/NFKC 与 sfnt 传递），见 ADR-0003、ADR-0009。
- 普通错误以 `error` 返回，不向调用方传播 panic。
- 默认严格模式；重建必须显式开启并记录诊断。
- 诊断与错误信息不包含文档正文。
- 公共入口暴露根包、`displaylist`、`render`、`text` 与 `extension` 的稳定类型。

## 许可

Apache-2.0。第三方依赖、内嵌资源与语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
