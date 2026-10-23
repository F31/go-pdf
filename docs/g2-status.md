# G2：字体栈、ToUnicode 与预定义 CJK CMap

日期：2026-10-21｜范围：Type0/CID、ToUnicode、预定义/嵌入 CMap、Type3、sfnt 度量

## 已完成能力

- 简单字体与 Type0/CID 字体的 `/ToUnicode`：`codespacerange`（多长度）、`bfchar`、`bfrange`（连续目标与数组目标）、多 rune 目标
- Type0 + CIDFontType0/2：`DescendantFonts`、`BaseFont`、`DW`、`W`、`DW2`/`W2` 记录
- Identity-H/V：双字节字符码、CID 与竖排码位
- 预定义简体中文 CMap：`UniGB-UCS2-H/V`、`UniGB-UTF16-H/V`、`GBK-EUC-H/V`、`GB-EUC-H/V`
- 预定义 Big5 CMap：`UniCNS-UCS2-H/V`、`UniCNS-UTF16-H/V`、`B5-H/V`、`B5pc-H/V`、`ETen-B5-H/V`、`ETenms-B5-H/V`
- 预定义日文 CMap：`UniJIS-UCS2-H/V`、`UniJIS-UTF16-H/V`、`90ms-RKSJ-H/V`、`90msp-RKSJ-H/V`、`90pv-RKSJ-H/V`、`EUC-H/V`、`H/V`
- 预定义韩文 CMap：`UniKS-UCS2-H/V`、`UniKS-UTF16-H/V`、`KSCms-UHC-H/V`、`KSCms-UHC-HW-H/V`、`KSC-EUC-H/V`、`KSCpc-EUC-H/V`、`KSC-Johab-H/V`
  - 1/2/4 字节 codespace 切分，`UniGB-UTF16` 支持代理对
  - 字符码到 CID 的预定义映射，`/W` 按 CID 取宽度
  - `UniGB-UCS2`/`UTF16` 直接由字符码得到 Unicode；GBK/GB 系通过 `GBK-EUC-UCS2` 映射
  - 日韩 legacy CMap 使用 code→Unicode 表（如 `90ms-RKSJ-UCS2`）或 CID→Unicode 表（`Adobe-Japan1-UCS2`/`Adobe-Korea1-UCS2`）与 code→CID 组合
  - 竖排 CMap 使用 V 变体覆盖标点 CID
- Unicode 解析优先级：简单字体与 Type0 均为 `/ToUnicode` 优先，再回退到 Encoding 或预定义 CMap
- Type3：`Widths` × `FontMatrix` 计算前进量，`/ToUnicode` 或 Differences 推导 Unicode
- Type3 CharProcs：每个字形展开为 `Path` 并标记 `FlagFromType3`，同时保留对应 `TextRun`
- Type3 CharProc 内的路径、图形状态与基础颜色复用内容解释器；嵌套裁剪写 `FNT-TYPE3-CLIP`
- 间接 `/Resources /Font` 字典解析修复：字体字典可再经一层引用
- 嵌入 CMap 流：解析 `codespacerange`、`cidrange`、`cidchar`、`notdefrange`、`WMode` 与 `usecmap`
  - `usecmap` 可继承内嵌预定义 CMap 或 `Identity-H/V`
  - 继承字符码长度、CID、Unicode 回退语义与 WMode，并在其上覆盖本地映射
- 预定义 CMap 数据以构建期生成的二进制嵌入，来源与许可见 `THIRD_PARTY.md`
- 嵌入 TrueType/OpenType（`FontFile2`/`FontFile3` OpenType）通过 `x/image/font/sfnt` 读取：
  - 填充 `FontMetrics.UnitsPerEm/Ascent/Descent/CapHeight/XHeight/AvgWidth/Advances`（AvgWidth 为字体单位，Advances 为千分单位）
  - `Advances` 为千分单位，简单字体以字符码为键，Type0 以 CID 为键
  - 支持 `/CIDToGIDMap` Identity 与流映射
  - 容错修复未对齐/未排序表、缺失或损坏的 `cmap`/`post`，并写诊断
  - 非嵌入、Type3、raw Type1/CFF 等标记 `FontMetrics.Substituted=true`
- 字形轮廓读取：`x/image/font/sfnt` `LoadGlyph` 输出 1/1000 文本空间、Y 向上的路径，二次曲线转换为三次曲线
  - 简单字体经 Encoding/ToUnicode 求 GID；Type0 优先使用 `/CIDToGIDMap`，CIDFontType0 回退 Unicode
  - 每个轮廓以 `SegmentClose` 结束；Type3 继续使用 `CharProcs` 路径，不走 sfnt
- 文档级字体缓存：跨页复用已加载字体，避免重复解析字体程序

## 本地语料结果

| 指标 | G1 | G2 |
| --- | --- | --- |
| 文件 / 页数 | 53 / 10,862 | 53 / 10,862 |
| 字符码/Glyph | 24,032,451（按字节） | 12,177,647（按字符码） |
| 可信 Unicode | 未统计（Type0 全部占位） | 12,177,647 |
| 路径 | 1,261,222 | 1,261,374（含 Type3 字形路径） |
| 唯一字体 / 有度量 / 有 Advances | 未统计 | 643 / 491 / 476 |
| 解释错误 | 0 | 0 |
| 耗时 | 15.4s | 14.4s |

所有字符码均已得到可信 Unicode。抽样
`H3C UniServer R5300 G6 产品彩页.pdf` 第 1 页可输出完整中文标题与正文；
`紫鸾UniCloudR5960G5机架服务器技术白皮书.pdf` 第 1 页在修复间接字体资源后
可输出完整中文文本。

## 已知限制

- Type3 CharProc 内的嵌套文本被忽略并写 `FNT-TYPE3-TEXT`；嵌套裁剪只写诊断，不做精确合成。
- Type3 字形路径来自 CharProc 内容流；复杂颜色空间、图案与图像仍按既有能力降级。
- 竖排（`-V`/`WMode 1`）已使用 `W2`/`DW2` 计算前进量与字形原点；复杂竖排标点替代与双向混排仍属后续。
- 多 rune 目标完整写入 `Glyph.Text`，但前进量仍按首字符宽度计算。
- 预定义 CMap 的多字符 Unicode 目标（当前数据 1,878 条，主要为兼容序列）未写入一维映射表；首字符加变体选择符会归并到首字符。
- 未包含日韩较少见的 CMap 变体（如 `Add-RKSJ`、`Ext-RKSJ`、`NWP`、`78-RKSJ`、`UniJIS2004` 等）。
- raw Type1、raw CFF（`Type1C`/`CIDFontType0C`）与 Type3 不提供 sfnt 度量或轮廓，`Substituted=true`。
- 简单字体的轮廓依赖 `cmap` 或 ToUnicode 映射；损坏且只能合成空 `cmap` 的字体可能缺少轮廓。
- 部分字体通过合成 `cmap`/`post` 获得度量，简单字体的 Unicode→GID Advances 可能缺失；CIDFontType2 仍可由 `/CIDToGIDMap` 填充。
- 区域渲染仍属后续增量；轮廓读取已就绪。
- 无 ToUnicode 且无预定义 Unicode 回退时输出 `U+FFFD`，不猜测编码。

## 后续输入

1. G3b 已提供区域渲染、EvenOdd、虚线/连接、色键遮罩与混合模式；剩余软蒙版/图案光栅化与 G5 语义见 `docs/g3-status.md`。
2. G3b 区域渲染（路径/图像/裁剪/透明/字体轮廓文字）。
3. `ActualText` 已支持（`TextRun.ActualText`）；剩余结构与标记内容见 `docs/g3-status.md`。
4. 复杂竖排标点替代与双向混排。
5. Type3 嵌套裁剪、复杂颜色与图案的精确处理。
6. 参考引擎文本位置 P95 统计。
7. 多字符 CMap Unicode 目标与少见日韩 CMap 变体（按语料驱动）。
