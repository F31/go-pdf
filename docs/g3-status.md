# G3 内容对象状态与 G3b 输入

日期：2026-10-21｜范围：G3 内容对象（Form XObject、图像、颜色、透明度）

## 已完成

- Form XObject（`/Subtype /Form`）展开：
  - `/Matrix` 与当前 CTM 组合，页面空间坐标正确
  - `/Resources` 独立解析；缺失时继承调用方资源
  - `/BBox` 转换为页面空间裁剪，并加入显示列表 `Clips`
  - `MaxFormDepth` 预算、递归与循环检测，写 `BUD-FORM-DEPTH`/`CNT-FORM-CYCLE`
  - 嵌套项裁剪索引重映射；与父裁剪交集不精确时写 `CNT-FORM-CLIP-INTERSECT`
  - `WithPreserveForms(true)` 时保留 `/Group` 透明组为 `displaylist.Group`，区域渲染离屏合成；默认扁平化并写诊断
- 显示列表继续输出 Form 内的 `TextRun`、`Path`、裁剪与 Bounds；字体缓存跨 Form 复用
- Image XObject 与内联图像输出 `displaylist.Image`：
  - `ImageRef.Raw()` 保留原始编码，`ImageRef.Decode(ctx)` 输出 `image.Image`
  - 支持 DeviceGray/RGB/CMYK、1/2/4/8/16 bpc、Decode 数组、ImageMask
  - DCT 使用标准库 `image/jpeg`；CMYK JPEG 统一转换为 sRGB
  - CCITT Group 4 与 Group 3 1-D（含 EOL）通过 `x/image/ccitt` 解码；`BlackIs1` 与 `Decode` 已处理
  - JBIG2/JPX 通过 `pdf.RegisterExternalDecoder` 注册外部解码器；无解码器时显式返回 `ErrUnsupported`
  - 解析色键 Mask、显式 Mask 与 SMask；SMask 只关联，不在此层合成
  - `Page.Images` 与 `Page.Fonts` 已从显示列表返回清单
- 修正 `cm` 与 CTM 的乘法顺序，图像/路径在缩放平移后的位置正确
- ExtGState 常量透明度与线型状态：
  - `ca`/`CA` 映射到显示项 `Common.Alpha`，`BM` 记录到 `Common.Blend`
  - 支持 `LW/LC/LJ/ML/D` 的线宽、端点、连接、斜接与虚线状态
  - 填充与描边 alpha 不同时取较低值并写 `CNT-GSTATE-ALPHA`
  - `/SMask`、`/Font` 等未实现项写诊断，不静默忽略
- 颜色空间与 PDF Function：
  - DeviceGray/RGB/CMYK、CalGray/CalRGB/Lab、Indexed、Separation/DeviceN
  - ICCBased 按 `/N` 回退到 Device 空间，不做 ICC 变换并标记近似
  - PDF Function 类型 0（采样）、2（指数）、3（拼接）、4（PostScript 计算）
  - `Paint.Raw` 保留原始分量；`Paint.Space` 输出解析后的颜色空间名
- 可选内容：
  - 解析 `/OCProperties` 的 `BaseState`、`ON`、`OFF`
  - 支持 `BDC` 属性名与 `/OC`，隐藏内容在 `EMC` 前整体跳过
  - `WithHiddenLayers(true)` 可包含隐藏图层；默认遵循文档配置
  - 未闭合的可选内容标记写 `CNT-OC-UNBALANCED`
- 显示列表扩展字段：
  - `Path.Hint` 识别矩形、圆角矩形、椭圆、直线、折线；不匹配时保持 Generic
  - `Common.Source.OpIndex` 记录页内容操作符序号；Form 内序号会加上外层偏移
  - `Common.Sig` 按类型、样式与相对几何计算，可在不同位置识别重复元素
  - `Page.Scanned`：整页图像覆盖 ≥90% 且无可信 Unicode 文本时置位
  - `TextRun.ActualText`：标记内容 `/ActualText` 优先输出；与字形映射冲突写 `CNT-ACTUALTEXT-CONFLICT`
  - `displaylist.Version` 升至 1.11（仅字段追加）
- Tiling 图案与剩余渐变类型：
  - `PatternType 1` 记录 BBox/XStep/YStep/Matrix/PaintType/TilingType、原始内容流与 uncolored 基色
  - 图案内容已展开为 `Pattern.Items`，区域渲染支持彩色/未着色图案基础平铺
  - `ShadingType 1` 记录为 `ShadingFunction`，`4–7` 记录为 `ShadingMesh` 并保留原始码流
  - 这些类型按设计 R/P 档处理，不做静默近似
- 渐变：
  - 支持 `ShadingType 2`（轴向）与 `3`（径向），输出 `displaylist.Shading`
  - 函数结果通过颜色空间转为 64 个色标，保留 `Extend` 与 CTM 变换
  - 非均匀缩放的径向渐变按圆近似并写 `CLR-SHADING-CTM`
  - `ShadingType 1` 记录为 `ShadingFunction`；`4–7` 记录为 `ShadingMesh` 并透传原始码流

- G3b 区域渲染：
  - 新增公共 `render.RenderRegion`，默认 220 DPI、上限 300 DPI
  - 支持路径填充与 EvenOdd、虚线、butt/round/projecting 端点、miter/bevel/round 连接、图像、色键遮罩与 ImageMask 模板、裁剪、常量 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字
  - `Exclude`、`IncludeText` 与 `MaxPixels` 预算生效
  - 未注册解码器的图像显式返回 `ErrUnsupported`，不输出缺图区域
  - Alpha/Luminosity 软蒙版与透明组离屏合成已进入区域渲染；非隔离组使用 backdrop 拷贝/替换，knockout 仍为近似

## 本地语料结果

`docs/pdf/` 私有语料，对全部页面执行 `DisplayList`：

| 指标 | G2 阶段 | 当前（含 Form/Image/Shading/Pattern） |
| --- | --- | --- |
| 文件 / 页数 | 53 / 10,862 | 53 / 10,862 |
| TextRun | 1,078,860 | 1,087,974 |
| Glyph / 可信 Unicode | 12,177,647 | 12,210,639 |
| Path | 1,261,374 | 1,268,331 |
| Image | — | 9,558 |
| Shading | — | 3,339 |
| Scanned 页 | — | 28 |
| Pattern 填充 | — | 689 |
| 唯一字体 / 有度量 / 有 Advances | 643 / 491 / 476 | 657 / 500 / 483 |
| 解释错误 | 0 | 0 |
| 耗时 | 约 13.0s | 约 16.0s |

本地语料中的 24 个 CCITT 图像全部解码成功；DCT 图像通过标准库解码。启用可选内容过滤后，1 个隐藏图像未输出。

## 未完成

- CCITT Group 3 2-D（`K > 0`）与无 EOL 的 G3 变体尚未支持，返回显式错误；JBIG2/JPX 核心不提供解码，仅支持外部注册。
- type 4–7 mesh 与 function shading 已渲染。
- 色键遮罩、16 种混合模式、Alpha/Luminosity 软蒙版与透明组离屏合成已进入区域渲染。
- Form BBox 与父裁剪的交集为近似；knockout 透明组细节未完整。

## 下一步

1. knockout 透明组细节与 SMask PSNR 金样。
2. SMask PSNR 金样与混合模式回归。
3. 按语料需要补 CCITT G3 2-D/无 EOL 变体。
4. G5 结构树/链接/注释与 G8 整页光栅化。
