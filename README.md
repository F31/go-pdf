# go-pdf

纯 Go、无 CGO 的 PDF 解析与内容提取组件。G0 提供文档打开、页面遍历、页面空间、元数据、资源预算与诊断基础；G1 提供内容流解释与基础显示列表；G2 增量提供简单字体与 Type0/CID 的 Unicode、字宽和预定义 CJK CMap。

## 状态

当前为 **G3 内容对象与 G3b 区域渲染阶段**（G0/G1/G2 已完成）：

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
- 颜色空间：CalGray/CalRGB/Lab、ICCBased matrix/TRC（RGB/Gray）、LUT8/LUT16/mAB CLUT（CMYK→Lab/XYZ）与 mBA/B2A0 反向 LUT 解析、Indexed、Separation/DeviceN；PDF Function 类型 0/2/3/4
- 轴向/径向渐变：`ShadingType 2/3`、64 色标采样、`Extend`、CTM 变换与裁剪
- 可选内容可见性：`/OCProperties` 的 `BaseState`/`ON`/`OFF`、`BDC /OC`、OCMD `/P`/`/VE` 与 `WithHiddenLayers`；OCG 名称写入 `Common.Layer`
- Tiling 图案与 type 4/5 mesh 内容；type 6/7 mesh 原始码流透传
- `render.RenderPage` 整页 CropBox 渲染；`EncodePNG`/`EncodeJPEG` 输出图像
- `render.RenderRegion` 区域渲染：路径填充/EvenOdd、虚线、butt/round/projecting 端点与 miter/bevel/round 连接、ImageMask 模板、彩色/未着色 Tiling 图案基础平铺、type 4–7 mesh、function shading、Alpha/Luminosity 软蒙版（含 /BC backdrop）、透明组离屏合成（保留 BlendColorSpace、ICC BlendProfile（隔离/非隔离组级合成）、Backdrop；隔离/非隔离组 DeviceGray 混合空间生效（实心路径/文字/渐变/图像按元素转灰度）；DeviceCMYK/ICC 组按元素在设备分量上合成，图像优先使用源样本进入组空间，隔离组最终合成支持组 Blend 模式；knockout 组按元素独立合成，非隔离组元素以当前 backdrop 为初始背景）、图像与色键遮罩、裁剪、常量 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字；DPI 默认 220、上限 300；可通过 `render.FontProvider` 为无嵌入轮廓的字体注入字形
- 图像解码失败（如未注册 JBIG2/JPX）在区域渲染中显式返回 `ErrUnsupported`，禁止静默缺图
- ImageMask、色键 Mask、SMask 关联输出；`Page.Images` 与 `Page.Fonts` 清单
- 显示列表 `Version 1.18`；`PaintState` 保留填充/描边 alpha 与叠印意图，`TextSpan` 用字形区间映射 ActualText；路径形状提示、`Source` 操作符追溯、相对几何 `Sig` 与 `Page.Scanned`
- `Page.Walk` / `Page.DisplayList` 输出文字、路径、图像与裁剪
- `Page.Text` 与公共 `text` 包提供字符、词、行与 ActualText 的基础文本索引
- `Page.Links` 解析 Link 注释的矩形、URI、直接/命名 GoTo 目标；`displaylist.Page.Links` 同步填充
- `Document.Outlines` 解析大纲树的标题、页目标、URI 与子节点
- `Page.Annotations` 解析通用注释与 Widget 的 Rect、Subtype、Contents、URI、目标页、字段名/值/类型
- 注释 `/AP /N` 外观流展开到 `AnnotationAppearance`；`RenderRegion` 可用 `Annotations: AnnotationView/Print` 叠加，并按 Invisible/Hidden/NoView/Print 标志过滤
- `Document.FormFields` 解析 AcroForm 字段树、继承类型、值、Flags、页面与 Widget Rect
- `Document.StructureTree` 解析轻量 Tagged PDF 结构树、角色、标题、页面与 MCID
- `Document.Attachments` 解析 EmbeddedFiles 名称树并读取嵌入文件
- `pdf.Capabilities()` 输出静态功能支持/近似/外部/不支持清单
- 标记内容 `BDC` 的 `/MCID` 写入显示项 `Common.MCID`；`DisplayList` 按结构树回填 `StructRef`
- `Page.Label` / `Document.PageLabel` 解析 `/PageLabels` 的十进制、罗马数字、字母与前缀
- `Document.Preflight` 按 Parse/Extract/Render/Edit/Export/Verify 报告状态与 Outcome
- 活跃文件内存预算 `MaxLiveBytes`；`Document.ResourceUsage` 输出 Live/Peak/Decoded/Objects
- 统一预算、诊断、`*pdf.Error`、`render.Renderer`/Profile/Capabilities、CMYK ICC profile 注入、图像质量指标/阈值门禁与 `displaylist.Version 1.18`

预定义 CMap 中少数多字符 Unicode 目标（当前 1,878 条兼容序列）未展开；变体选择符归并到首字符。

尚未实现：CCITT G3 2-D/无 EOL 变体、JBIG2/JPX 核心解码（仅外部解码器）、CFF/Type1、knockout 精确合成、写入/签名与矢量输出。未实现内容会写诊断或返回 `ErrUnsupported`，不会伪造结果。

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

可用选项：`WithRepair`、`WithBudget`、`WithPassword`（Standard R2-R6）、`WithHiddenLayers`（是否包含隐藏图层）、`WithPreserveForms`（保留 Form XObject 透明组）、`WithColorManager`（DeviceCMYK/ICCBased 外部转换）。渲染默认使用页面所属文档的活跃内存预算；`render.PageOptions.Memory` 可覆盖为其他 `MemoryBudget`。路径/文字/透明组/软蒙版/clip mask、图像解码结果、输出栅格、字体缓存与图案/软蒙版缓存都会计费。`render.RenderPageLease` 返回需 `Release` 的 `Raster`；`render.RenderPages` 提供有界页级并发；`render.RenderTiles`/`RenderPageTiles` 以固定像素块流式输出并复用同一份页面显示列表，单块租约在回调返回后释放；`render.PlanTiles`/`PlanTilesAdaptive` 预计算网格；`RenderTilesTo`/`RenderPageTilesTo` 可直接组装到调用方画布。

## 构建与测试

构建需要 Go 1.25+。

```bash
CGO_ENABLED=0 go build ./...
go vet ./...
go test -race ./...
go test ./... -run=^$ -bench=. -benchmem
go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime=30s
```

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
cmd/pdf/                              CLI（info/text/search/render/links/outlines/forms/attachments/preflight）
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

Poppler 24.02 的本机实测体积、RSS、吞吐与输出体积见
[`docs/perf/poppler-24.02-baseline.md`](docs/perf/poppler-24.02-baseline.md)。该基线用于同机同语料对比，
不是通用厂商基准。

## 设计约束

- 仅标准库与 `golang.org/x/*`；新增依赖必须走 ADR 与许可审计。
- 当前外部依赖：`golang.org/x/image`（sfnt 度量）、`golang.org/x/text`（SASLprep/NFKC 与 sfnt 传递），见 ADR-0003、ADR-0009。
- 普通错误以 `error` 返回，不向调用方传播 panic。
- 默认严格模式；重建必须显式开启并记录诊断。
- 诊断与错误信息不包含文档正文。
- 公共入口暴露根包、`displaylist`、`render` 与 `text` 的稳定类型。

## 许可

Apache-2.0。第三方依赖、内嵌资源与语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
