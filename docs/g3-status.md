# G3 内容对象状态与 G3b 输入

日期：2026-10-21｜范围：G3 内容对象（Form XObject、图像、颜色、透明度）

## 已完成

- Form XObject（`/Subtype /Form`）展开：
  - `/Matrix` 与当前 CTM 组合，页面空间坐标正确
  - `/Resources` 独立解析；缺失时继承调用方资源
  - `/BBox` 转换为页面空间裁剪，并加入显示列表 `Clips`
  - `MaxFormDepth` 预算、递归与循环检测，写 `BUD-FORM-DEPTH`/`CNT-FORM-CYCLE`
  - 嵌套项裁剪索引重映射；与父裁剪通过 `Clip.Intersect` 合成
  - `WithPreserveForms(true)` 时保留 `/Group` 透明组为 `displaylist.Group`，区域渲染离屏合成
  - 默认扁平化非透明组；当 Form 处于非 1 alpha、非 Normal blend 或 SMask 下时自动保留透明组，避免丢失组级透明度
  - 支持间接 `/Group` 字典；组级 alpha 在合成时应用，组内初始 alpha 为 1
- 显示列表继续输出 Form 内的 `TextRun`、`Path`、裁剪与 Bounds；字体缓存跨 Form 复用
- Image XObject 与内联图像输出 `displaylist.Image`：
  - `ImageRef.Raw()` 保留原始编码，`ImageRef.Decode(ctx)` 输出 `image.Image`
  - 支持 DeviceGray/RGB/CMYK、ICCBased（1/3/4 分量）、Indexed（DeviceGray/RGB/CMYK、ICCBased 基色）、间接 `/ColorSpace`、1/2/4/8/16 bpc、Decode 数组、ImageMask
  - 图像流使用按图像尺寸推导的解码字节上限，避免高压缩纯色图误触通用 inflate ratio 预算；内容流仍保留通用比例保护
  - DCT 使用标准库 `image/jpeg`；CMYK JPEG 统一转换为 sRGB
  - 图像解码接入 `ColorManager`：DeviceCMYK 与 ICCBased 原始样本/JPEG 可注入外部 ICC 或 CMYK→sRGB 转换；未注入时 ICCBased 使用内置 ICC 变换
  - 新增 `displaylist.ImageSampleRef` / `ImageSamples`：保留归一化源样本、源颜色空间、ICC profile 与分量数；样本缓冲带可释放租约
  - 透明组按 sRGB 扁平化；不执行 CMYK/ICC 设备空间合成
  - 同时带填充和描边的路径拆成两个绘制操作，各自应用 alpha 后合成
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
  - ICCBased 解析 matrix/TRC RGB/Gray、LUT8/LUT16 与 mAB CLUT（CMYK→Lab/XYZ）到 sRGB；PCS D50 XYZ 经 Bradford 适配 D65；缺失/不支持的 profile 回退到 Device 空间并标记近似
  - `WithColorManager` 可注入 DeviceCMYK/ICCBased 外部转换，错误写诊断；Indexed、Separation、DeviceN 的基色/替代空间也会传递同一 manager
  - 新增 `pdf.NewColorManagerFromEngine`：将 `extension.ColorEngine` 适配为 `ColorManager`，按源空间/profile 缓存 transform，目标固定 DeviceRGB/sRGB；返回对象实现 `io.Closer`
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
  - `displaylist.Version` 升至 2.0（删除打印色彩/设备组字段；保留 `PaintState` 填充/描边 alpha、`TextSpan`、`TextRun.Line`、组 Backdrop、软蒙版 Backdrop、`Clip.Intersect`）
- 文本绘制模式：
  - `TextRun` 新增 `Line`，保留文字描边线宽、端点、连接、斜接与虚线
  - `RenderMode 0/1/2` 分别执行填充、描边、填充+描边；`3` 不可见；`4–7` 裁剪模式仍显式跳过
  - 文本按字形拆分为小 tile；`RenderMode 2` 再按 fill/stroke 拆分
  - 文字渲染模式 4–7：将字形轮廓加入裁剪路径，不进行填充/描边；`Clip.Intersect` 表示与已有裁剪的交集
  - Type3 文字裁剪优先解释 CharProc，收集其填充/描边路径作为实际裁剪轮廓
  - 无可用字形轮廓时（部分未嵌入 base14、无路径 Type3）使用保守 em box 作为替代裁剪路径，并记录 `CNT-TEXT-CLIP-BBOX`；空白 glyph 不生成裁剪
  - `q/Q` 恢复裁剪状态；路径裁剪与文字裁剪采用交集而非替换

- 图案与剩余渐变类型：
  - `PatternType 1` 记录 BBox/XStep/YStep/Matrix/PaintType/TilingType、原始内容流与 uncolored 基色；图案内容已展开为 `Pattern.Items`，区域渲染支持彩色/未着色平铺并按 X/Y 步长双向重复
  - Pattern 内容继承当前 ExtGState，`/SMask` 对 tiling/shading pattern 均生效
  - `PatternType 2` 解析 `/Shading`，支持 axial/radial shading pattern；按路径 mask 合成。mesh/function shading pattern 记录诊断并按设计降级
  - `ShadingType 1` 记录为 `ShadingFunction`；`4–7` 记录为 `ShadingMesh` 并保留原始码流
  - 渲染支持 type 4–7：type 4/5 顶点网格与 type 6/7 patch；支持 `/Function` 颜色、32 MiB 流上限；超限或空流记录诊断，不静默近似
  - Pattern 子渲染器的临时租约在 tile 完成后释放，避免跨页累计导致 `BUD-LIVE-MEMORY`
- 渐变：
  - 支持 `ShadingType 2`（轴向）与 `3`（径向），输出 `displaylist.Shading`
  - 函数结果通过颜色空间转为 64 个色标，保留 `Extend` 与 CTM 变换
  - 非均匀缩放的径向渐变按圆近似并写 `CLR-SHADING-CTM`
  - `ShadingType 1` 记录为 `ShadingFunction`；`4–7` 记录为 `ShadingMesh` 并透传原始码流；渲染覆盖见上一节

- 加密：
  - Standard R2–R6：RC4-40/128、AES-128、AES-256
  - 支持用户密码与所有者密码；Info 字符串、内容流、对象流解密
  - R6 Algorithm 2.B；SASLprep 常用映射/NFKC/禁止字符检查；完整双向规则未实现
- 能力报告：
  - `pdf.Capabilities()` 按 parse/text/render/document/security 列出 supported/approximate/external/unsupported
  - `pdf.CapabilitiesFor(ProfileCore|Preview|Extended)` 与 `pdf capabilities -profile` 按 ADR-0012/0014 分档输出
  - `extension.Registry` 提供不可变 provider 目录，按编码/名称显式选择，拒绝重复和顺序依赖
  - `extension/legacy` 将旧 `pdf.DecoderFactory`、`pdf.ColorManager`、`render.FontProvider` 适配到新端口，旧 API 保持兼容
  - `extension/poppler` 通过 `pdftoppm` 独立进程提供可选 Extended 高保真渲染；`RenderPage` 单页、`RenderEach` 单进程分页回调、`RenderPages` 任意页集，不链接 GPL
  - `extension/cairo` 通过 `pdftocairo` 独立进程输出单页 SVG，不链接 Cairo/Poppler；CLI `pdf svg` 暴露该能力
  - `extension/openjpeg` 通过 `opj_decompress` 独立进程解码 JPX，注册为可选图像 provider；命令路径用 fake binary 测试，真实 OpenJPEG 端到端待安装
  - `displaylist.FontInfo.Source` 暴露嵌入字体程序，`Paint.Profile` 保留 ICCBased 原始 profile
  - `render.Options` / `PageOptions.FontEngine` 通过 FontSource 调用 extension 字体 provider
  - `pdf.WithExtensionRegistry` 将 Registry 的图像 provider 接入解码路径；`DecodeSamples` 保留源样本，`Decode` 兼容为 image.Image；与旧 `DecoderRegistry` 同编码重复配置返回参数错误
  - `Document.Preflight` 扫描页面并输出 Parse/Extract/Render 的 Status/Outcome
  - `Budget.MaxLiveBytes` 与 `Document.ResourceUsage` 提供活跃文件内存与峰值
- CLI：
  - `go run ./cmd/pdf info|text|search|render|links|outlines|forms|attachments|preflight`
  - `render` 支持 DPI、页面选择、背景与注释外观开关
  - `-password` 可用于所有读取 PDF 的命令
  - `attachments` 列出嵌入文件
- G3b 区域渲染与输出：
  - `render.RenderPage` 按 CropBox、旋转和 UserUnit 后的页面空间输出整页
  - `render.EncodePNG` / `EncodeJPEG` 提供基础图像编码
  - 新增公共 `render.RenderRegion`，默认 220 DPI、上限 300 DPI
  - 支持路径填充与 EvenOdd、虚线、butt/round/projecting 端点、miter/bevel/round 连接、图像、色键遮罩与 ImageMask 模板、裁剪、`PaintState` 分离填充/描边 alpha、16 种混合模式、轴向/径向渐变、嵌入字体轮廓文字
  - `Exclude`、`IncludeText` 与 `MaxPixels` 预算生效
  - 未注册解码器的图像显式返回 `ErrUnsupported`，不输出缺图区域
  - Alpha/Luminosity 软蒙版（含 /BC backdrop）与透明组离屏合成已进入区域渲染；非隔离组使用 backdrop 拷贝/替换；`Group.BlendColorSpace` 与可选 Backdrop 已保留；隔离/非隔离组按 sRGB 扁平化，DeviceGray 混合空间生效；CMYK/ICC 组不再做设备空间合成；knockout 组按元素独立合成，非隔离组元素从当前 backdrop 初始化

## 文本与链接

- 公共 `Page.Links`：Link Rect、URI、直接/命名 GoTo 页目标，并写入显示列表
- 链接矩形按页面 Matrix 转换到页空间
- `Document.Outlines`：标题、页目标、URI、子节点；支持 `/Dests` 与 `/Names /Dests` 名称树
- Outline 循环与深度有保护
- `Page.Label` / `Document.PageLabel` 支持 `/PageLabels` 的 D/R/r/A/a、前缀与 St
- `Page.Annotations` 输出通用注释与 Widget 元数据；`displaylist.Page.Annotations` 同步
- `/AP /N` 外观流按 BBox→Rect 映射展开；`RenderRegion` 的 `Annotations: View` 按 Invisible/Hidden/NoView 标志过滤
- `Document.FormFields` 输出 AcroForm 字段树（类型、值、Flags、页面、Widget Rect）
- `Document.StructureTree` 输出轻量 StructTreeRoot/StructElem/MCR 树与 MCID
- `Document.Attachments` 输出 EmbeddedFiles 名称树，`Attachment.Data` 读取嵌入文件
- 内容流 BDC 的 `/MCID` 透传到显示项 `Common.MCID`
- `DisplayList` 按页面结构树把 MCID 映射为 `StructRef`（结构元素对象号）
- `displaylist.Page.Label` 输出页标签

## 竖排文本

- Type0 `WMode 1`/`-V` 使用 `/W2`/`/DW2` 计算垂直前进量
- 字形原点按 `/W` 宽度水平居中，文本矩阵沿垂直方向推进
- `text.Extract` 按行段和列聚类；`Page.Columns`、`Line.Column` 暴露列结构，`Page.Vertical` 标记竖排页

## 文本索引

- 公共 `text` 包：字符、词、行、块、列感知阅读顺序、`StructOrder`、结构语义元数据、RoleMap/ClassMap/Classes、`/A` 与类属性树、BBox/Table/List 访问器与 AsTable/Page.Tables/Document.Tables（TR/TD/TH、行列位置、span、headers/HeaderRefs、段落、嵌套表格、跨页同 ID 合并、TablesMerged 启发式合并、重复表头去重、span 诊断）、`Page.Select` 选区、`Search.Hit.Quads` 逐行高亮、旋转文本按基线分行、竖排 RTL/LTR 分列与纵向 Quad、`Char.Quad`、Selection/Hit `CharQuads` 与整页文本
- `Page.Text(ctx)` 便捷入口
- ActualText 作为独立字符项，避免重复填入每个 glyph
- 水平多栏和全宽标题已按列/行段排序；tagged `StructOrder` 可优先于几何列顺序；纯 RTL 与常见 mixed 行按词/run 反转为逻辑序；支持 NFC/NFKC 正规化和 ligature 兼容分解；完整 UBA 级别、复杂竖向标点仍属后续

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

## 渲染质量门禁

- `render/quality_gate_test.go` 使用独立预期图校验 SMask Alpha、SMask Luminosity 与 knockout 非隔离组合成，门槛为 PSNR ≥45 dB、SSIM ≥0.999。
- `render/poppler_quality_test.go` 在 `POPPLER_QUALITY=1` 时对纯色、常量透明度、Luminosity SMask 与 `pdftoppm` 参考图比较，门槛为 PSNR ≥40 dB、SSIM ≥0.99、mean ΔE ≤1、p95 ΔE ≤2。
- 当前 knockout 仍用解析期望门禁，不用本地 Poppler Splash 作预言机。
- 已加入私有语料首屏 Poppler 差分：57/57 页可比较，渲染/Poppler/尺寸错误均为 0；在 PSNR 20 dB、SSIM 0.85 阈值下 fallback 21/57（36.8%）；PSNR p50/p90 为 25.74/33.82 dB，SSIM p50/p90 为 0.903/0.981。这是 Preview 基线，不是 G8 达标。详见 `docs/perf/poppler-corpus-baseline.md`。
- 已修复旋转文本 advance、`Td`/`T*` 文本空间顺序和 glyph-matrix 文本边界；新增 `TestRotatedTextAdvanceAndTd`、`TestRotate90CornerMapping`。
- 简单嵌入字体若缺少 Unicode cmap（例如仅 Mac Roman cmap 的子集），不再用错误 cmap 生成方框轮廓，而是拒绝轮廓并进入系统/内置回退；新增 `FNT-OUTLINE-CMAP` 诊断和 `TestHasUnicodeCMap`。
- 语料 harness 支持 `POPPLER_CORPUS_PAGES=0` 全页扫描、按文件批量调用 Poppler、单页失败回退、逐页 CSV 报告；多页批量路径已在 5 页样本验证，0 渲染/参考错误。全量 10,866 页尚未实际跑完。
- `IncludeText` 在未配置 FontProvider/FontEngine 时使用内置 Go 字体轮廓回退；支持 sans/mono 及 bold/italic/bolditalic 变体，base-14 与无嵌入程序文本不再直接跳过。新增 `render.ChainFonts` 组合多个 provider。
- 新增 `render.SystemFonts()`：扫描平台 TTF/OTF/TTC/OTC 目录（Linux/macOS/Windows），解析 OpenType name 表，按 Family/Style/Mono/Serif 匹配；逐候选检查 glyph 覆盖，缺字可继续回退。支持 Helvetica/Arial、Times、Courier 常见别名。CJK 字符会优先尝试 CJK family（Noto CJK、WenQuanYi、SimSun、YaHei、Mincho、PingFang 等），并支持 TTC/OTC collection index。候选按 `zh-Hans`/`zh-Hant`/`ja`/`ko` locale 评分：字体名（如 SimSun、MingLiU、Mincho、Malgun、Noto CJK SC/TC/JP/KR）和字符脚本（假名/谚文/注音）参与选择。GSUB `locl`/`vert` shaping 尚未实现；`IncludeText` 默认使用系统字体链，显式设置 `Fonts` 可获得主机无关输出。

## 未完成

- CCITT Group 3 2-D（`K > 0`）与无 EOL 的 G3 变体核心尚未支持；`ImageCCITTG32D` provider 路径已验证可接管，需部署方注入实现。
- JBIG2/JPX 核心不提供解码，仅支持 Factory Registry 注入；`extension/jbig2` 和 `extension/openjpeg` 提供可选进程 provider。
- Form BBox 与父裁剪已按 clip Intersect 合成；knockout 已按元素 alpha 覆盖并处理非 Normal Blend 的 backdrop 合成；完整外部参考金样仍未建立。
- 已新增 SMask Alpha/Luminosity 与 knockout 非隔离组的解析期望质量门禁（PSNR ≥45 dB、SSIM ≥0.999）；外部参考图金样与 G8 生产级并行光栅化尚未完成。
- 完整 `scene` IR 与场景缓存租约仍未完成。文件映射读取已加入 `OpenFile` 与 `Open(*os.File)`（Unix mmap，其他平台回退读取）；大 ReaderAt 的临时文件分块复制支持 context 取消；流式 ReaderAt 解析仍未完成。
- 已落地 `PaintState`、`TextSpan`、Preflight、文件/临时位图/图像解码/输出栅格租约、有界页级 Executor、`RenderTiles`/`RenderPageTiles`、`PlanTiles`/`PlanTilesAdaptive` 与 `RenderPageTilesTo`；tile 复用单页显示列表，避免逐 tile 重解释。

## 下一步

1. knockout 透明组细节与 SMask PSNR 金样。
2. 按语料需要补 CCITT G3 2-D/无 EOL 变体。
3. G8 生产级并行整页光栅化与质量/性能门禁。
