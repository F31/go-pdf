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

## 已知限制

- 标准 14 宽度表来自 Liberation 临时度量，非 Adobe AFM 原表；文本位置尚无参考引擎 P95 统计。
- Type0/CID、CMap、ToUnicode、Type3、嵌入字体轮廓未实现；只做回退宽度与诊断。
- Form XObject、内联图像、图像 XObject、渐变、图案、软蒙版、透明组未实现，写诊断并跳过。
- 路径控制点包围盒可能略大于真实曲线包围盒；线宽按 CTM 平均缩放近似。
- 裁剪交集未精确表达，当前用新裁剪切替换旧裁剪并写诊断。
- 标记内容 `ActualText`、结构树关联未实现。
- `Walk` 已按项流式输出，但文本运行在 flush 前会短暂缓存。

## G2 输入

1. Type0/CIDFont + CMap + ToUnicode（含预定义 CMap 与中文）。
2. 字体程序读取（`x/image/font/sfnt`）与 `FontMetrics`/`Advances`。
3. `ActualText` 优先级与冲突诊断。
4. Type3 字形路径输出。
5. 基于字宽补偿的文本位置校准与 P95 统计。
6. 将 `FNT-TYPE0-DEFERRED`/`FNT-TYPE3-DEFERRED` 替换为真实字形输出，并更新显示列表金样哈希。
