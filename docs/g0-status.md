# G0 状态与 G1 输入

日期：2026-10-21｜范围：G0 基础阶段

## 已完成能力

- 经典 xref、xref 流、对象流、混合引用、增量更新、线性化文件读取
- 严格模式与 `WithRepair(true)` 重建（xref 失败、对象流补全、页面树阶段失败回退）
- Flate、LZW（EarlyChange 1/0）、ASCII85、ASCIIHex、RunLength、TIFF/PNG Predictor
- 页面树、继承属性、`CropBox`/`Rotate`/`UserUnit` 页面空间、Info 元数据与原始 XMP
- 统一预算、诊断、`*pdf.Error`、关闭与取消语义
- `displaylist` 类型骨架（G0 时为 `Version 1.0`）、规范化 JSON 与金样哈希
- 金样运行器（`UPDATE_GOLDEN=1`）、故障注入工具、fuzz 目标与 benchmark 基线

## 本地语料结果

`docs/pdf/` 私有语料（不提交）：

| 指标 | 结果 |
| --- | --- |
| 文件数 | 53 |
| 打开成功 | 53 |
| 遍历页数 | 10,862 |
| 损坏 / 加密 / 超预算 / 不支持 | 0 / 0 / 0 / 0 |

首次回归发现并修复：对象流头部多条目空白、xref 偏移前导空白。对应的合成回归用例已入库。

## G0 退出验收（2026-10-25）

执行：

```bash
GOTOOLCHAIN=go1.26.10 FUZZTIME=10s ./scripts/verify.sh
./scripts/release-check.sh
./scripts/check-licenses.sh
```

结果：

- 六目标交叉编译：linux/darwin/windows × amd64/arm64 通过。
- `go test -race`、`CGO_ENABLED=0` 构建/测试：通过。
- fuzz 冒烟：全部 target 通过。
- `govulncheck`（Go 1.26.10）：0 vulnerabilities。
- 许可清单：`golang.org/x/image`、`golang.org/x/text` 均登记并检测到许可证。
- 静态发布构建：`CGO_ENABLED=0`，strip 约 13.1 MB。
- benchstat：旧 G0 5% 硬门禁已由 P4 的同机 P95 10% 门禁取代
  （`scripts/perf-gate.sh`，P4 样本通过）。历史 `docs/perf/g0-bench.txt`
  继续保留作趋势参考；其与当前机器/工具链的绝对耗时不可直接作为阻断条件。

G0 的 display-list 规范文本冻结在 1.0；当前实现已演进到 2.0。归档时保留历史
delta specs，不回写主 specs，以免用过期 1.0 文本覆盖 2.0 现状。

## 已知限制

- 当前实现把输入完整读入内存；`io.ReaderAt` 已作为公共入口，但尚未实现分块/零拷贝读取。
- Repair 为扫描式恢复，未覆盖设计文档 4.4 的全部畸形变体。
- 页面树各 `Pages` 节点的 `Count` 与子树实际页数不一致时产生 `DOC-PAGE-COUNT` 警告诊断；页数仍以实际遍历为准。
- PDFDocEncoding 与 UTF-16 BOM 解码已统一用于 Info、Outline、表单、附件、OC 名称和页标签；UTF-16 支持代理对；PDFDocEncoding 的 0x7F/0x9F 未定义码位替换为 U+FFFD。XMP 同时保留原始字节；常用字段通过 XML tokenizer 解析，支持多值 creator/subject 与 x-default Alt。
- 加密文件仅识别并返回 `ErrPassword`/`ErrUnsupported`，解密在 G4/G2 末实现。
- DCT/CCITT 已在 G3 解码；JBIG2/JPX 通过外部解码器注册，核心不做解码。
- 显示列表解释、字体、颜色、图像与区域渲染在 G0 时未实现；G1 已补齐基础内容解释，见 `docs/g1-status.md`。
- 预算默认值与性能 SLO 尚未用语料固化；CI 的 benchstat 目前生成对比报告，5% 硬门禁待基线稳定后启用。
- 公共语料未入库，CI 只运行合成样本；本地语料回归通过 `TestLocalCorpus` 手动执行。

## G1 输入

1. 内容流词法与操作符分发（CNT-01），依赖当前对象模型与流解码。
2. 图形状态、路径与裁剪（CNT-02）。
3. 标准 14 字体与基础文字定位（FNT-01），先输出 `TextRun` 所需度量。
4. `displaylist.Walk` 单趟流式输出，保持与 `DisplayList` 顺序一致。
5. 将 `Common.Source` 与 `Flags` 接入真实解释结果，替换当前 `ErrUnsupported` 占位。
6. 在本地语料上增加文字位置与页面遍历耗时统计，作为 G1 退出标准的数据来源。
