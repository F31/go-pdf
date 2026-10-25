# P1 Core 稳定化基线

日期：2026-10-25
用途：记录 Core 解析/资源/批量/错误/跨平台稳定化结果；Preview 质量另见
`docs/perf/p0-baseline.md` 与 `docs/perf/poppler-corpus-baseline.md`。

## 发现问题并修复

### 截断的 ReaderAt 导致 spool 死循环

`mapReaderAt` 在临时文件写入阶段遇到 `ReadAt` 返回 `0, io.EOF` 且尚未达到声明
大小时，会重复读取同一偏移。已改为清理临时文件并返回 `io.ErrUnexpectedEOF`。
新增 `TestMapReaderAtTruncated`，用 2 秒超时确保不会挂起。

### 取消错误不能匹配 context.Canceled

公共取消错误原先是独立字符串 sentinel，调用方无法同时使用
`errors.Is(err, context.Canceled)`。`ErrCanceled` 现在实现 `Is(context.Canceled)`，
保留 `pdf.ErrCanceled` 兼容性，并新增 `TestCanceledErrorMatchesContext`。

### spool 临时文件生命周期

新增：

- `TestMapReaderAtCancelsDuringCopy`：读取 1 MiB 后取消，验证错误和临时文件清理。
- `TestMapReaderAtTempCleanup`：成功映射后释放，验证 spool 文件删除。
- `TestOpenLargeReaderAtCleansSpool`：通过公共 `Open` 打开大于阈值的内存
  `ReaderAt`，关闭后验证临时文件删除。

## 覆盖补充

- `FuzzOpenFile` 新增损坏 startxref、错误 xref 偏移、错误流长度、截断对象、
  xref 流/对象流和加密 fixture 种子。
- 新增 `FuzzEncryptedOpen`，以 R2/R6 加密 fixture 为种子，模糊解密后的页面解释。
- `TestExtractTextMatchesSinglePageFailures`：批量与单页对损坏页返回一致的失败状态。
- `TestPreflightFailurePagesMatchExtract`：预检与批量提取的失败页集合一致。
- `TestExtractTextEachContextCancellation`：回调取消后返回 `context.Canceled` 语义。
- `TestStructureTreeCycleTerminates`：自引用 StructElem 在深度/visited 保护下终止。
- `TestPageTables` 增加重复调用一致性检查。

## 公共 API 与错误审计

- 外部包 `external_api_test.go` 继续编译通过，未暴露 internal PDF 对象类型。
- `Error` 保持为公共别名 `type Error = errs.Error`，用于兼容现有 `*pdf.Error`
  类型断言；`errs.Error` 是通用错误载体，不包含 PDF 对象或文档文本。
- `TestErrorsDoNotLeakText` 覆盖解析错误不泄漏内容流文本；错误结构仍只包含
  Code/Op/Offset/Obj/Page 和底层错误。
- `pdf.ErrCanceled` 与 `context.Canceled` 的兼容性作为公开取消契约。

## 本地门禁

```bash
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
go test ./internal/syntax -run=^$ -fuzz=FuzzOpenFile -fuzztime=15s
go test . -run=^$ -fuzz=FuzzEncryptedOpen -fuzztime=5s
```

结果：

- 单元测试通过
- race 通过
- vet/staticcheck 通过
- `FuzzOpenFile`：约 442 万次执行，无崩溃
- `FuzzEncryptedOpen`：种子可执行，无崩溃

## 跨平台

- P0 已加入 Linux/macOS/Windows CI；`gitattributes` 固定 golden 为 LF、PDF/字体/
  图片为 binary。
- 新增 spool/temp 测试在 macOS/Windows 也会运行。
- P1 完成后需等待远端 CI 全绿，作为 2.9 阶段门禁。
