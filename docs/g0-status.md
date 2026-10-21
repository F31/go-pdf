# G0 状态与 G1 输入

日期：2026-10-21｜范围：G0 基础阶段

## 已完成能力

- 经典 xref、xref 流、对象流、混合引用、增量更新、线性化文件读取
- 严格模式与 `WithRepair(true)` 重建（xref 失败、对象流补全、页面树阶段失败回退）
- Flate、LZW（EarlyChange 1/0）、ASCII85、ASCIIHex、RunLength、TIFF/PNG Predictor
- 页面树、继承属性、`CropBox`/`Rotate`/`UserUnit` 页面空间、Info 元数据与原始 XMP
- 统一预算、诊断、`*pdf.Error`、关闭与取消语义
- `displaylist` 类型骨架与 `Version 1.0`、规范化 JSON 与金样哈希
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

## 验证结果

`FUZZTIME=3s ./scripts/verify.sh` 通过：无 CGO 构建、`go vet`、`go test -race`、六目标交叉编译、四个 fuzz 目标冒烟、依赖白名单与 `staticcheck`。`govulncheck` 已安装但当前环境无法访问漏洞数据库（代理连接失败），因此尚未纳入通过结果；CI 环境需补跑。benchstat 对比在 CI 生成报告，5% 硬门禁待受控 runner 稳定后启用。

## 已知限制

- 当前实现把输入完整读入内存；`io.ReaderAt` 已作为公共入口，但尚未实现分块/零拷贝读取。
- Repair 为扫描式恢复，未覆盖设计文档 4.4 的全部畸形变体。
- 页面树 `Count` 仅作提示，尚未产生与实际页数不一致的诊断。
- Info 的 PDFDocEncoding 仅覆盖常用子集；XMP 仅透传原始字节，字段解析与优先级回退属 DOC-03。
- 加密文件仅识别并返回 `ErrPassword`/`ErrUnsupported`，解密在 G4/G2 末实现。
- DCT/CCITT/JBIG2/JPX 仅保留原始码流，解码属 G3。
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
