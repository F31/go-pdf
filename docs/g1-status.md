# G1 内容解释状态与 G2 输入

日期：2026-10-21｜范围：G1 内容流解释（基础文本/路径）

## 已完成能力

- 内容流 tokenizer、操作数栈、未知操作符、`BX/EX`、`BI/ID/EI` 跳过
- 图形状态 `q/Q`、`cm`、线型、Gray/RGB/CMYK 基础颜色
- 路径 `m/l/c/v/y/h/re`、`S/s/f/F/f*/B/B*/b/b*/n`、`W/W*` 裁剪池
- 文本状态 `Tc/Tw/Tz/TL/Tf/Tr/Ts`、`BT/ET/Td/TD/Tm/T*`、`Tj/TJ/'/"`
- 简单字体 `/Widths`、`/MissingWidth`、StandardEncoding/WinAnsi、`/Differences`
- 标准 14 字体宽度表（Liberation 度量生成的临时数据，SIL OFL，见 `THIRD_PARTY.md`）
- `Page.Walk` 流式输出与 `Page.DisplayList` 物化输出共用解释器
- 路径/文本编号、Z 顺序、裁剪索引、Bounds、OutOfBounds、Approx 标记

## 本地语料结果

`docs/pdf/` 私有语料（不提交），对全部页面执行 `DisplayList`：

| 指标 | 结果 |
| --- | --- |
| 文件 / 页数 | 53 / 10,862 |
| TextRun | 1,078,860 |
| Glyph | 24,032,451 |
| Path | 1,261,222 |
| 解释错误 | 0 |
| 耗时 | 约 15.4s（单进程） |

说明：语料以中文 Type0/CID 字体为主，G1 按设计输出 `UnicodeOK=false` 的占位字形；简单字体页面（如 H3C、HuaKun、紫光云认证证书等）已能提取可信英文/数字文本。Type0/CMap/ToUnicode 是 G2 的主要任务。

## 英文文本位置抽样（2026-10-25）

语料：15 份美国政府公有领域 PDF（8 Congressional Record、5 Supreme Court、1 Public Law、1 NIST），共 2,233 页；文件不提交仓库，使用 `scripts/fetch-english-corpus.sh` 下载。

方法：每份取前 3 页，Core 词框与 Poppler `pdftotext -bbox` 同名词框做最近匹配，统计词框边缘最大误差和中心误差。

| 指标 | P50 | P95 | P99 | 备注 |
| --- | ---: | ---: | ---: | --- |
| 词框边缘误差 | 0.000 pt | 0.012 pt | 3.590 pt | 96.93% ≤ 0.3 pt |
| 词框中心误差 | 0.000 pt | 0.006 pt | 3.085 pt | |

抽样：45 页，34,374 个匹配词。P95 满足设计初值 0.3 pt；长尾来自重复词匹配和个别异常页。

为达到该结果修复了 `text.Extract`：

- 字形页空间缩放取 `Glyph.Matrix`，不再直接使用 `TextRun.Size`；
- 字宽按 CTM/字体缩放换算；
- 词框上下界使用 `FontInfo.Ascent/Descent`，与 Poppler 字体包围盒一致。

命令：

```bash
./scripts/fetch-english-corpus.sh /tmp/english-corpus
./scripts/english-text-position.sh /tmp/english-corpus
```

## 已知限制

- 标准 14 宽度表来自 Liberation 临时度量，非 Adobe AFM 原表；英文位置 P95 已用 Poppler `-bbox` 抽样统计，见下节。
- G1 未实现 Type0/CID、CMap、ToUnicode、Type3、嵌入字体轮廓；G2 增量已覆盖 Identity-H、简单字体与 Type3 ToUnicode、预定义与嵌入 CMap、多字节 codespace、Type3 CharProcs 路径、sfnt `FontMetrics` 与字形轮廓，见 `docs/g2-status.md`。
- Form XObject 已在 G3 展开；基础 Image XObject/内联图像、DCT 与 Device 色彩空间已在 G3 支持，见 `docs/g3-status.md`。渐变、图案、透明组与高级图像编码仍未实现。
- 路径控制点包围盒可能略大于真实曲线包围盒；线宽按 CTM 平均缩放近似。
- 裁剪交集未精确表达，当前用新裁剪切替换旧裁剪并写诊断。
- 标记内容 `ActualText` 已在 G3 写入 `TextRun.ActualText`；结构树关联未实现。
- `Walk` 已按项流式输出，但文本运行在 flush 前会短暂缓存。

## G2 输入

1. Type0/CIDFont + CMap + ToUnicode（含预定义/嵌入 CMap 与中文）。
2. 字体程序读取（`x/image/font/sfnt`）与 `FontMetrics`/`Advances`（G2 已完成）。
3. `ActualText` 优先级与冲突诊断。
4. Type3 字形路径输出（G2 已完成基础 CharProcs 展开）。
5. 英文文本位置 P95 已完成；后续按字体/生成器分层跟踪长尾。
6. Type3 基础字形输出已完成；后续补充 CharProc 嵌套裁剪的精确合成与复杂颜色/图案。
