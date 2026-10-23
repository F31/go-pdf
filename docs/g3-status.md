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
  - 支持 DeviceGray/RGB/CMYK、ICCBased（1/3/4 分量）、1/2/4/8/16 bpc、Decode 数组、ImageMask
  - DCT 使用标准库 `image/jpeg`；CMYK JPEG 统一转换为 sRGB
  - 图像解码接入 `ColorManager`：DeviceCMYK 与 ICCBased 原始样本/JPEG 可注入外部 ICC 或 CMYK→sRGB 转换；未注入时 ICCBased 使用内置 ICC 变换
  - 新增 `displaylist.ImageSampleRef` / `ImageSamples`：保留归一化源样本、源颜色空间、ICC profile 与分量数；样本缓冲带可释放租约
  - CMYK/ICC 组内的图像在可用时直接使用源样本转换到组设备空间，不再先量化为 8 位 sRGB；无法提供源样本或转换失败时回退元素级 sRGB 路径
  - 同时带填充和描边的路径拆成两个绘制操作，分别进入组设备空间合成，避免填充/描边先合并成一个 sRGB 元素
  - CCITT Group 4 与 Group 3 1-D（含 EOL）通过 `x/image/ccitt` 解码；`BlackIs1` 与 `Decode` 已处理
  - JBIG2/JPX 通过不可变 `DecoderFactory` Registry 注入；`Engine` 可跨文档复用配置；无解码器时显式返回 `ErrUnsupported`
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
  - ICCBased 解析 matrix/TRC RGB/Gray、LUT8/LUT16 与 mAB CLUT（CMYK→Lab/XYZ）；mBA/B2A0 可解析并求值反向 LUT，并用于 ICC 组级合成；缺失/不支持的 profile 仍回退到 Device 空间并标记近似
  - `WithColorManager` 可注入 DeviceCMYK/ICCBased 外部转换，错误写诊断
  - PDF Function 类型 0（采样）、2（指数）、3（拼接）、4（PostScript 计算）
  - `Paint.Raw` 保留原始分量；`Paint.Space` 输出解析后的颜色空间名
- 公共字体 SPI：
  - `render.PageOptions.Fonts` / `render.FontProvider` 为无嵌入轮廓的字体提供公共扩展点
  - 嵌入 `GlyphSource` 优先，失败后回退 Provider；错误不被吞掉
  - 外部包测试覆盖 base-14 无轮廓字体的 Provider 回退
- 公共图像解码 SPI：
  - 不可变 `DecoderRegistry` 保存 `DecoderFactory`；`Engine` 复用配置，Document 保存快照
  - Factory 默认按图像任务创建解码会话；`WithOverride` 派生新 Registry；不再有进程级注册
  - 外部包通过 `external_api_test.go` 验证公共 API 可编译使用
- Pattern/SoftMask/Form 解码缓存：
  - 同一内容流只解码一次，嵌套 Form/Pattern/SoftMask 共享缓存
  - 彩色 Pattern 与 SoftMask 的展开结果按 `(stream, CTM)` 缓存
- 可选内容：
  - 解析 `/OCProperties` 的 `BaseState`、`ON`、`OFF`
  - 支持 `BDC` 属性名与 `/OC`，OCMD 的 `/P`（AllOn/AnyOn/AnyOff/AllOff）与 `/VE`（And/Or/Not），隐藏内容在 `EMC` 前整体跳过
  - `WithHiddenLayers(true)` 可包含隐藏图层；默认遵循文档配置；OCG `/Name` 写入 `Common.Layer`
  - 未闭合的可选内容标记写 `CNT-OC-UNBALANCED`
- 显示列表扩展字段：
  - `Path.Hint` 识别矩形、圆角矩形、椭圆、直线、折线；不匹配时保持 Generic
  - `Common.Source.OpIndex` 记录页内容操作符序号；Form 内序号会加上外层偏移
  - `Common.Sig` 按类型、样式与相对几何计算，可在不同位置识别重复元素
  - `Page.Scanned`：整页图像覆盖 ≥90% 且无可信 Unicode 文本时置位
  - `TextRun.ActualText`：标记内容 `/ActualText` 优先输出；与字形映射冲突写 `CNT-ACTUALTEXT-CONFLICT`
  - `displaylist.Version` 升至 1.18（仅字段追加；`PaintState`、`TextSpan`、`TextRun.Line`、组 Backdrop/ICC BlendProfile、软蒙版 Backdrop、`Clip.Intersect`）
- 文本绘制模式：
  - `TextRun` 新增 `Line`，保留文字描边线宽、端点、连接、斜接与虚线
  - `RenderMode 0/1/2` 分别执行填充、描边、填充+描边；`3` 不可见；`4–7` 裁剪模式仍显式跳过
  - CMYK/ICC 组内的填充+描边文本拆成两个设备空间绘制操作
  - 文本按字形拆分为小 tile：每个 glyph 独立渲染并进入组设备空间，避免整段文字先扁平化为一个 sRGB 元素；`RenderMode 2` 再按 fill/stroke 拆分
  - 文字渲染模式 4–7：将字形轮廓加入裁剪路径，不进行填充/描边；`Clip.Intersect` 表示与已有裁剪的交集
  - Type3 文字裁剪优先解释 CharProc，收集其填充/描边路径作为实际裁剪轮廓
  - 无可用字形轮廓时（部分未嵌入 base14、无路径 Type3）使用保守 em box 作为替代裁剪路径，并记录 `CNT-TEXT-CLIP-BBOX`；空白 glyph 不生成裁剪
  - `q/Q` 恢复裁剪状态；路径裁剪与文字裁剪采用交集而非替换

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

- 加密：
  - Standard R2–R6：RC4-40/128、AES-128、AES-256
  - 支持用户密码与所有者密码；Info 字符串、内容流、对象流解密
  - R6 Algorithm 2.B；SASLprep 常用映射/NFKC/禁止字符检查；完整双向规则未实现
- 能力报告：
  - `pdf.Capabilities()` 按 parse/text/render/document/security/edit 列出 supported/approximate/external/unsupported
  - `Document.Preflight` 扫描页面并输出 Parse/Extract/Render/Edit/Export/Verify 的 Status/Outcome
  - `Budget.MaxLiveBytes` 与 `Document.ResourceUsage` 提供活跃文件内存与峰值
- CLI：
  - `go run ./cmd/pdf info|text|search|render|links|outlines|forms|attachments|preflight`
  - `render` 支持 DPI、页面选择、背景与注释外观开关
  - `-password` 可用于所有读取 PDF 的命令
  - `attachments` 列出嵌入文件
- G3b 区域渲染与输出：
  - `render.RenderPage` 按 CropBox 输出整页
  - `render.EncodePNG` / `EncodeJPEG` 提供基础图像编码
  - 新增公共 `render.RenderRegion`，默认 220 DPI、上限 300 DPI
  - 支持路径填充与 EvenOdd、虚线、butt/round/projecting 端点、miter/bevel/round 连接、图像、色键遮罩与 ImageMask 模板、裁剪、`PaintState` 分离填充/描边 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字
  - `Exclude`、`IncludeText` 与 `MaxPixels` 预算生效
  - 未注册解码器的图像显式返回 `ErrUnsupported`，不输出缺图区域
  - Alpha/Luminosity 软蒙版（含 /BC backdrop）与透明组离屏合成已进入区域渲染；非隔离组使用 backdrop 拷贝/替换；`Group.BlendColorSpace`、ICC `BlendProfile` 与可选 Backdrop 已保留；隔离/非隔离组 DeviceGray 混合空间生效（实心绘制与图像先转灰度），RGB 保持 sRGB，CMYK/ICC 执行元素级设备空间合成；knockout 组按元素独立合成，非隔离组元素从当前 backdrop 初始化；逐元素设备空间内部绘制和完整 alpha 语义仍为近似

## 文本与链接

- 公共 `Page.Links`：Link Rect、URI、直接/命名 GoTo 页目标，并写入显示列表
- 链接矩形按页面 Matrix 转换到页空间
- `Document.Outlines`：标题、页目标、URI、子节点；支持 `/Dests` 与 `/Names /Dests` 名称树
- Outline 循环与深度有保护
- `Page.Label` / `Document.PageLabel` 支持 `/PageLabels` 的 D/R/r/A/a、前缀与 St
- `Page.Annotations` 输出通用注释与 Widget 元数据；`displaylist.Page.Annotations` 同步
- `/AP /N` 外观流按 BBox→Rect 映射展开；`RenderRegion` 的 `Annotations: View/Print` 按 Invisible/Hidden/NoView/Print 标志过滤
- `Document.FormFields` 输出 AcroForm 字段树（类型、值、Flags、页面、Widget Rect）
- `Document.StructureTree` 输出轻量 StructTreeRoot/StructElem/MCR 树与 MCID
- `Document.Attachments` 输出 EmbeddedFiles 名称树，`Attachment.Data` 读取嵌入文件
- 内容流 BDC 的 `/MCID` 透传到显示项 `Common.MCID`
- `DisplayList` 按页面结构树把 MCID 映射为 `StructRef`（结构元素对象号）
- `displaylist.Page.Label` 输出页标签

## 竖排文本

- Type0 `WMode 1`/`-V` 使用 `/W2`/`/DW2` 计算垂直前进量
- 字形原点按 `/W` 宽度水平居中，文本矩阵沿垂直方向推进
- `text.Extract` 按列聚类，`Page.Vertical` 标记竖排页

## 文本索引

- 公共 `text` 包：字符、词、行、Bounds 与整页文本
- `Page.Text(ctx)` 便捷入口
- ActualText 作为独立字符项，避免重复填入每个 glyph
- 当前支持基础水平与竖排列聚类；双向、复杂竖排标点和 Tagged 阅读顺序仍属后续

## 本地语料结果

`docs/pdf/` 私有语料已恢复并扩充到 57 个文件，对全部页面执行 `DisplayList`：

| 指标 | G2 阶段 | 当前（含 Form/Image/Shading/Pattern） |
| --- | --- | --- |
| 文件 / 页数 | 53 / 10,862 | 57 / 10,866 |
| TextRun | 1,078,860 | 1,090,999 |
| Glyph / 可信 Unicode | 12,177,647 | 12,212,204 |
| Path | 1,261,374 | 1,268,399 |
| Image | — | 9,566 |
| Shading | — | 3,339 |
| Scanned 页 | — | 28 |
| Pattern 填充 | — | 689 |
| 唯一字体 / 有度量 / 有 Advances | 643 / 491 / 476 | 669 / 512 / 495 |
| 解释错误 | 0 | 0 |
| 耗时 | 约 13.0s | 约 18s |

本地语料中的 24 个 CCITT 图像全部解码成功；DCT 图像通过标准库解码。启用可选内容过滤后，1 个隐藏图像未输出。

## 未完成

- CCITT Group 3 2-D（`K > 0`）与无 EOL 的 G3 变体尚未支持。
- JBIG2/JPX 核心不提供解码，仅支持 Factory Registry 注入。
- Form BBox 与父裁剪的交集为近似；knockout 已按元素 alpha 覆盖并处理非 Normal Blend 的 backdrop 合成；完整外部参考金样仍未建立。
- SMask PSNR 金样、knockout 精确合成与 G8 生产级并行光栅化尚未完成。
- 完整 `scene` IR 与场景缓存租约仍未完成。文件映射读取已加入 `OpenFile` 与 `Open(*os.File)`（Unix mmap，其他平台回退读取）；流式 ReaderAt 解析仍未完成。
- 已落地 `PaintState`、`TextSpan`、Preflight、文件/临时位图/图像解码/输出栅格租约、有界页级 Executor、`RenderTiles`/`RenderPageTiles`、`PlanTiles`/`PlanTilesAdaptive` 与 `RenderPageTilesTo`；tile 复用单页显示列表，避免逐 tile 重解释。

## 下一步

1. knockout 透明组细节与 SMask PSNR 金样。
2. 按语料需要补 CCITT G3 2-D/无 EOL 变体。
3. G8 生产级并行整页光栅化与质量/性能门禁。
