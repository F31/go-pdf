# ADR-0012: Core / Preview / Extended 产品边界

日期：2026-10-23｜状态：Accepted
更新：2026-10-23 范围由 ADR-0014 收窄；项目不实现写入、签名、打印色彩，Extended 仅按需接入外部 provider。

## 背景

V1.6 的 Native Complete 目标要求原生覆盖 JBIG2、JPX、完整字体、ICC、印刷色彩、
写入、签名和多种输出，已超出轻量、单一职责内核的合理范围。继续把所有能力都作为
核心必达项，会导致范围失控、依赖策略自相矛盾，也无法清楚说明“支持”与“近似”的
边界。

Poppler 的广度来自多年积累和 FreeType、lcms2、OpenJPEG、jbig2dec、Cairo 等成熟
依赖，不是单一代码库从零实现。go-pdf 需要保留 Go 原生内核的差异化，同时允许成熟
实现通过公共接口接入，而不是把项目做成 Poppler 的逐项重写。

## 决策

### 1. 使用三个产品档位，不维护三套内核

| 档位 | 目标 | 默认实现 | 不承诺 |
| --- | --- | --- | --- |
| **Core** | PDF 读取、解密、解释、IR、文本与结构提取 | 纯 Go；标准库 + `x/*` | 像素级渲染、印刷色彩、写入、签名 |
| **Preview** | 常用页面屏幕预览与基础栅格 | Core + Go 预览渲染器；可选 adapter | 印刷/分色/叠印、完整原生 codec、编辑、签名 |
| **Extended** | 高保真、原生 codec、编辑、签名、矢量输出 | 可插拔纯 Go/CGO/独立进程后端 | 默认纯 Go 可部署、统一像素确定性 |

三个档位共享同一对象模型、解释器和 IR。差异来自 provider 组合、构建标签和能力
声明，不来自分叉的解析或渲染实现。

### 2. Core 边界

Core 必须提供：

- PDF 对象模型、xref/xref 流、对象流、增量更新、重建与页面树
- Standard Security R2–R6 解密
- 读取内容所需的过滤器、Predictor、页面资源与元数据
- 内容解释、图形状态、路径/文本/图像/裁剪的 IR 投影
- 文本、链接、大纲、页标签、注释/Widget 元数据、表单字段树、附件、结构树
- 预算、取消、诊断、错误分类、能力报告
- 无 CGO 构建；不依赖具体图像/字体/色彩实现

Core 不承诺：

- 完整页面像素输出
- JBIG2/JPX 原生解码
- 印刷色彩、专色、叠印、半色调
- PDF 写入、页面拆合、表单保存、签名
- OCR、版面分析、完整无障碍验证

Core 遇到缺失能力必须返回 `ErrUnsupported` 或记录 `NotScanned`/`Unsupported`
能力状态，不能静默输出空白或近似结果。

### 3. Preview 边界

Preview 在 Core 之上提供：

- CropBox/Rotate/UserUnit 页面空间
- 路径填充/描边、裁剪、EvenOdd、虚线、端点/连接
- 嵌入字体轮廓文字、基础文本绘制模式
- 常见图像、DCT、CCITT、ImageMask、色键 Mask
- 轴向/径向渐变、基础 Tiling Pattern
- 透明组、软蒙版、混合模式、alpha 的屏幕近似
- 注释 `/AP` 叠加
- 固定/自适应 tile、页级并发、内存预算

Preview 的质量声明限定为：

- sRGB 屏幕预览，通常 96–220 DPI
- 不声明印刷色彩、专色、叠印、半色调或输出意图正确
- 不声明所有 JBIG2/JPX、Type1/CFF、复杂字体或复杂合成完整
- 缺少 provider 时明确失败或标记 `NotScanned`，不得静默缺图/缺字

Preview 可以使用纯 Go 自研组件，也可以使用可选 adapter；但默认构建必须保持
`CGO_ENABLED=0`。

### 4. Extended 边界

Extended 通过 `extension` 接口接入：

- 原生/第三方 JBIG2、JPX、CCITT G3 2-D
- 完整 CFF/Type1、字体替代、shaping
- ICC v2/v4、CMYK、DeviceN、专色、叠印、输出意图
- CGO Cairo/Skia 等高保真光栅或矢量后端
- Poppler/其他引擎的独立进程后端
- 写入、页面拆合、附件、表单 AP、签名
- SVG/HTML/PDF/PS/EPS/TIFF/PPM 输出

Extended 必须：

- 使用公共类型实现 provider，不依赖 `internal/*`
- 报告 provider 名称、版本、许可、确定性和能力
- 在能力报告中区分 `Supported`、`Approximate`、`External`、`Unsupported`、
  `Unknown`、`NotScanned`
- 在缺失、失败或预算超限时给出可操作错误，不伪装成成功
- 单独构建、单独测试；核心 CI 不依赖 Extended provider

### 5. Native Complete 不再是当前发布档位

“Native Complete”容易把“没有外部依赖”和“与 Poppler 全面对等”混为一谈。
当前不再使用该名称作为发布承诺。若未来重启，必须新增 ADR，重新定义：

- 冻结功能表与 Poppler 版本
- 必需原生组件与外部组件清单
- 质量、性能、隐藏集和退役条件
- 许可与专利审查结论

在此之前，对外只声明 Core、Preview 和具体 Extended provider 的能力。

### 6. 能力报告与构建

- 静态 `Capabilities()` 描述发行物；`Document.Preflight` 描述具体文档；
  `Renderer.Capabilities` 描述具体页面。
- 任何 `Approximate`/`External`/`NotScanned` 都必须带原因，不能只给布尔值。
- provider 选择必须显式：配置 > profile 默认 > 无；不得静默跨 provider 回退。
- 核心构建：`CGO_ENABLED=0 go build ./...`。
- Extended 构建：独立 tags/module 或独立进程，不能要求核心用户安装 CGO 依赖。
- 同一 provider/版本/参数/字体/ICC 才可声明确定性；跨 provider 只比较质量指标，
  不承诺 bit-exact。

## 影响

- 现有 `render`、`displaylist` 和 CLI 功能继续保留，不因本 ADR 删除。
- README 和状态文档必须按档位描述能力，不能把 Preview 近似写成完整支持。
- 写入、签名、矢量输出从 Core 必达项中移出，作为 Extended 或独立产品线。
- 自研组件仍然可以做，但必须通过 provider 接口接入，并接受同一套质量门禁。
- 依赖策略由 ADR-0013 约束；本 ADR 不新增任何依赖。

## 迁移

1. 先抽 `extension` 公共接口，不改变现有 API 行为。
2. 现有 `pdf.DecoderFactory`、`pdf.ColorManager`、`render.FontProvider` 通过
   `extension/legacy` 映射到统一 provider 模型；旧 API 在一个 minor 周期内保留。
3. `displaylist` 继续作为实验性 IR v0；独立 `scene` 契约稳定前不冻结更多字段。
4. 文档和 CLI 增加 `--profile`/能力输出，明确当前档位与 provider。
5. 任何把 Preview 能力提升为 Core/Extended 的变更，必须附带测试与能力表更新。

## 验收

Core：

- 关闭所有 Extended provider 后仍可构建、打开、解释和提取语料
- 缺失 codec 时 `ErrUnsupported`/`NotScanned` 可预测
- 无 CGO，预算/取消/fuzz 门禁通过

Preview：

- 固定语料上路径、文字、图像、裁剪、透明组的屏幕质量达标
- 缺字体/缺 codec 不静默丢失
- 质量报告明确 sRGB 和 DPI，不宣称印刷正确

Extended：

- 每个 provider 有版本、许可、能力、差分和资源报告
- 删除 provider 后核心仍可构建
- provider 失败不会导致核心 panic 或静默降级

## 未覆盖

- Core 的具体 API 冻结版本
- `scene` 独立包和对象快照
- Extended provider 的具体商业/发布形态
- JPEG2000/JBIG2 专利结论
