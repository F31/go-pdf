# G2 首个增量：ToUnicode 与 Type0/CID

日期：2026-10-21｜范围：Identity-H/V + ToUnicode + CID 宽度

## 已完成能力

- ToUnicode CMap：`codespacerange`、`bfchar`、`bfrange`（连续目标与数组目标）、多 rune 目标
- Type0 + CIDFontType0/2：`DescendantFonts`、`BaseFont`、`DW`、`W`、`DW2`/`W2` 记录
- Identity-H/V：双字节字符码、CID、竖排标记与降级诊断
- 统一 `Codes()`/`WidthCode()`/`DecodeCode()`，简单字体路径保持 G1 金样不变
- 内容解释器输出 `Glyph.Code/CID/Text/UnicodeOK`
- 非 Identity 预定义 CMap 与缺失 ToUnicode 的显式诊断

## 本地语料结果

| 指标 | G1 | G2 |
| --- | --- | --- |
| 文件 / 页数 | 53 / 10,862 | 53 / 10,862 |
| 字符码/Glyph | 24,032,451（按字节） | 12,177,646（按字符码） |
| 可信 Unicode | 未统计（Type0 全部占位） | 1,401,248 |
| 解释错误 | 0 | 0 |
| 耗时 | 15.4s | 31.1s |

抽样：`H3C UniServer R5300 G6 产品彩页.pdf` 第 1 页已输出完整中文标题与正文（“产品概览”“多元算力引擎 H3C UniServer R5300 G6”等）。中文可信率仍受限于未实现预定义 CMap 与无 ToUnicode 的字体。

## 已知限制

- 预定义 CJK CMap（GBK-EUC、UniGB-UCS2/UTF16、Big5、Shift-JIS 等）未内置；命中时按 2 字节切分并降级。
- 嵌入 CMap 流未解析。
- 竖排（Identity-V）只标记，不承诺位置精度。
- 多 rune 目标完整写入 `Glyph.Text`，但前进量仍按首字符宽度计算。
- 无 ToUnicode 时输出 `U+FFFD`，不猜测编码。
- 嵌入字体轮廓与 `FontMetrics.Advances` 仍属后续增量。

## 后续输入

1. 预定义 CMap 数据（Adobe CMap 资源，需许可登记与构建期生成）。
2. 嵌入 CMap 流解析。
3. `x/image/font/sfnt` 字体度量与轮廓，填充 `FontMetrics`。
4. `ActualText` 与标记内容优先级。
5. 竖排 `W2/DW2` 定位（G5）。
6. 参考引擎文本位置 P95 统计。
