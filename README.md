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
- 间接 `/Resources /Font`、`WMode` 与竖排标记
- Type3 `CharProcs` 字形路径、`FlagFromType3`、`FontMatrix` 字宽与 ToUnicode
- `x/image/font/sfnt` 字体度量、`FontMetrics.Advances`、`CIDToGIDMap` 与文档级字体缓存
- 嵌入 TrueType/OpenType 字形轮廓读取（1/1000 文本空间，Y 向上，二次曲线转三次），覆盖 CIDToGID 映射
- Form XObject 展开：`Matrix`、`BBox` 裁剪、资源继承、递归/循环与深度预算
- Image XObject 与内联图像：`ImageRef.Raw/Decode`、DeviceGray/RGB/CMYK、1/2/4/8/16 bpc、Decode 数组、ImageMask、DCT/JPEG、CCITT G3 1-D/G4
- JBIG2/JPX 通过 `pdf.RegisterExternalDecoder` 注册外部解码器
- ExtGState 常量透明度 `ca`/`CA`、混合模式记录、线宽/端点/连接/虚线状态
- 颜色空间：CalGray/CalRGB/Lab、ICCBased 回退、Indexed、Separation/DeviceN；PDF Function 类型 0/2/3/4
- 轴向/径向渐变：`ShadingType 2/3`、64 色标采样、`Extend`、CTM 变换与裁剪
- 可选内容可见性：`/OCProperties` 的 `BaseState`/`ON`/`OFF`、`BDC /OC` 与 `WithHiddenLayers`
- Tiling 图案与 type 4/5 mesh 内容；type 6/7 mesh 原始码流透传
- `render.RenderRegion` 区域渲染：路径填充/EvenOdd、虚线、butt/round/projecting 端点与 miter/bevel/round 连接、ImageMask 模板、彩色/未着色 Tiling 图案基础平铺、type 4–7 mesh、function shading、Alpha/Luminosity 软蒙版、透明组离屏合成、图像与色键遮罩、裁剪、常量 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字；DPI 默认 220、上限 300
- 图像解码失败（如未注册 JBIG2/JPX）在区域渲染中显式返回 `ErrUnsupported`，禁止静默缺图
- ImageMask、色键 Mask、SMask 关联输出；`Page.Images` 与 `Page.Fonts` 清单
- 显示列表 `Version 1.11`；路径形状提示（矩形/圆角矩形/椭圆/直线/折线）、`Source` 操作符追溯、相对几何 `Sig`、`ActualText` 与 `Page.Scanned`
- `Page.Walk` / `Page.DisplayList` 输出文字、路径、图像与裁剪
- 统一预算、诊断、`*pdf.Error` 与 `displaylist.Version 1.11`

预定义 CMap 中少数多字符 Unicode 目标（当前 1,878 条兼容序列）未展开；变体选择符归并到首字符。

尚未实现：CCITT G3 2-D/无 EOL 变体、JBIG2/JPX 核心解码（仅外部解码器）、knockout 透明组的精确合成、加密解密、CLI。未实现内容会写诊断或返回 `ErrUnsupported`，不会伪造结果。

## 使用

```go
doc, err := pdf.OpenFile(ctx, "in.pdf")
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
    DPI: 220, Background: "white", IncludeText: true,
})
```

可用选项：`WithRepair`、`WithBudget`、`WithPassword`（G0 仅识别加密并返回 `ErrPassword`）、`WithHiddenLayers`（是否包含隐藏图层）、`WithPreserveForms`（保留 Form XObject 透明组）。

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
internal/syntax/                      词法、对象、xref、重建
internal/filter/                      过滤器与 Predictor
internal/doc/                         页面树、页面空间、元数据
internal/font/                        字体、CMap、字宽（data/ 为嵌入映射）
internal/budget/                      资源预算
internal/diag/                        诊断收集
internal/testkit/                     测试 PDF 构造工具
testdata/                             语料与金样
```

## 设计约束

- 仅标准库与 `golang.org/x/*`；新增依赖必须走 ADR 与许可审计。
- 当前外部依赖：`golang.org/x/image`（sfnt 度量）、`golang.org/x/text`（传递依赖），见 ADR-0003。
- 普通错误以 `error` 返回，不向调用方传播 panic。
- 默认严格模式；重建必须显式开启并记录诊断。
- 诊断与错误信息不包含文档正文。
- 公共入口暴露根包、`displaylist` 与 `render` 的稳定类型。

## 许可

Apache-2.0。第三方依赖、内嵌资源与语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
