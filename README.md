# go-pdf

纯 Go、无 CGO 的 PDF 解析与内容提取组件。G0 阶段提供文档打开、页面遍历、页面空间、元数据、资源预算与诊断基础；内容流解释、字体、颜色、图像、加密解密与显示列表输出在后续阶段实现。

## 状态

当前为 **G0 基础阶段**：

- 经典 xref、xref 流、对象流、增量更新、混合引用
- 严格模式与 `WithRepair(true)` 重建
- Flate、LZW、ASCII85、ASCIIHex、RunLength 与 Predictor
- 页面树、继承属性、`CropBox`/`Rotate`/`UserUnit` 页面空间换算
- Info 元数据与原始 XMP 透传
- 统一预算、诊断与 `*pdf.Error`
- `displaylist` 类型骨架与 `Version 1.0` 冻结

尚未实现：内容流解释、字体/CMap、颜色空间、图像解码、加密解密、区域渲染、写入与 CLI。未实现 API 会返回 `ErrUnsupported` 并写诊断，不会伪造空结果。

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
internal/budget/                      资源预算
internal/diag/                        诊断收集
internal/testkit/                     测试 PDF 构造工具
testdata/                             语料与金样
```

## 设计约束

- 仅标准库与 `golang.org/x/*`；新增依赖必须走 ADR 与许可审计。
- 普通错误以 `error` 返回，不向调用方传播 panic。
- 默认严格模式；重建必须显式开启并记录诊断。
- 诊断与错误信息不包含文档正文。
- 公共入口只暴露根包与 `displaylist` 的稳定类型。

## 许可

Apache-2.0。第三方依赖、内嵌资源与语料许可见 [THIRD_PARTY.md](THIRD_PARTY.md)。
