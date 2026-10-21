# go-pdf

纯 Go、无 CGO 的 PDF 解析与内容提取组件。G0 提供文档打开、页面遍历、页面空间、元数据、资源预算与诊断基础；G1 提供内容流解释与基础显示列表；G2 增量提供简单字体与 Type0/CID 的 Unicode、字宽和预定义 CJK CMap。

## 状态

当前为 **G1/G2 字体增量阶段**（G0 已完成）：

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
- `Page.Walk` / `Page.DisplayList` 输出文字、路径与裁剪
- 统一预算、诊断、`*pdf.Error` 与 `displaylist.Version 1.0`

预定义 CMap 中少数多字符 Unicode 目标（当前 1,878 条兼容序列）未展开；变体选择符归并到首字符。

尚未实现：区域渲染、Type3 嵌套裁剪的精确合成、ActualText、竖排精确定位、Form XObject、图像解码、渐变/图案、透明合成、加密解密、CLI。未实现内容会写诊断或返回 `ErrUnsupported`，不会伪造结果。

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

可用选项：`WithRepair`、`WithBudget`、`WithPassword`（G0 仅识别加密并返回 `ErrPassword`）、`WithHiddenLayers`、`WithPreserveForms`。

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
- 公共入口只暴露根包与 `displaylist` 的稳定类型。

## 许可

Apache-2.0。第三方依赖、内嵌资源与语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
