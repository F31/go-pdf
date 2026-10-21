# go-pdf 组件完整设计方案 V1.2

> 文档类型：开发实施基线（草案）｜日期：2026-10-20｜版本：V1.4（V1.4 = V1.3 + 保真度专项整改：图像解码契约[禁缺图]、CMYK 统一转换、渐变采样规则、区域渲染 DPI 默认值、SMask 合成验证、元素级保真门槛，见附录 A） 核心语言：Go｜核心约束：纯 Go、CGO\_ENABLED=0、无外部运行时强依赖 适用范围：通用 PDF 解析与内容提取组件；首要下游为 pdf2pptx 转换组件，同时可独立用于文本提取、版面分析、资源提取和合规检查。 本文定义目标架构、开发契约与验收要求，不表示相关功能已经实现或完成兼容测试。

**阅读导航：** 1–3 章定义定位、范围与总体架构（2.3 为特性能力矩阵）；4–12 章定义各层能力与显示列表契约（第 10 章是对下游的核心契约）；13–16 章定义公共 API、诊断、资源预算与性能；17–19 章定义验收、阶段与决策记录；20–24 章提供工作包、对接约定、CLI、工具链与参考资料。

## 1. 项目定位与总体目标

go-pdf 是面向服务端和工具开发者的 Go PDF 解析与处理组件。它把 PDF 文件读取为文档对象模型，并把页面内容流解释为稳定的**显示列表（Display List）**：带坐标的文字、矢量路径、图片、裁剪、颜色和透明度信息。显示列表是 go-pdf 对外的核心契约，也是 pdf2pptx 的唯一输入。

项目以六项能力构成产品主线：

1. **健壮读取**：经典与流式 xref、对象流、增量更新、线性化、损坏文件重建、加密文件解密。
2. **内容流解释**：图形状态、文本状态、路径、裁剪、XObject 递归、内联图片、标记内容与可选内容（图层）。
3. **字体与文字还原**：Type1、TrueType、Type0/CID、Type3、CMap、ToUnicode，输出可信的 Unicode、字宽和字形位置，并恢复词间空格。
4. **资源提取**：图片（保留原始编码）、字体程序、元数据、大纲、链接、附件。
5. **工程可靠性**：惰性解析、统一资源预算、取消与超时、显式失败、真实文档回归与原生 fuzz。
6. **可选扩展**：文字行与词的基础切分、有限光栅化（评测与缩略图）、CLI，保持核心依赖简单。

### 1.1 部署与职责

核心 SDK 编译为 Go 应用的静态依赖，不调用 Python、MuPDF、pdfium、Poppler 或 Office。OCR、版面检测模型、重型渲染由上层服务或可选适配器负责；使用这些能力的完整产品可能依赖外部进程，不能据此宣称整条转换链路零依赖。

三个组件的依赖关系：go-pdf 与 go-pptx 互不依赖；pdf2pptx 同时依赖二者，承担段落合并、表格还原、阅读顺序和元素映射等语义工作。

**生态位（V1.2 明确）**：go-pdf 的生态位是——"pdfcpu（文件操作与校验）× MuPDF/pdfium（内容流解释与显示列表）× 商业友好许可（Apache/MIT 路线）"的组合，该组合目前没有现成产品覆盖。与 pdfcpu 的关系：pdfcpu 的重心在文件结构处理、校验、装配与加密，不提供内容流→显示列表的解释层；二者能力互补而非替代，go-pdf 在文件层可参考 pdfcpu 的 validate/relaxed 设计思路（Apache-2.0，思路级参考并保留归属），在解释层以 MuPDF/pdfium/pdf.js 的文本层与显示列表架构为参照系。

### 1.2 开发约束

- 公共入口为 module 根包 `pdf`，正式 module path 在建仓时确定；文档中的示例域名不用于生产发布。
- 解析与解释逻辑放在 internal 包，公共 API 仅暴露稳定类型。
- 普通错误以 `error` 返回，不使用 panic 传播；内部若使用 recover，仅用于把解析层 panic 转成带位置的错误。
- 不通过返回空结果掩盖未支持能力；降级必须写入诊断。
- 所有读取接口接受 `context.Context`，长操作可取消。
- 第三方依赖只允许 Go 标准库和 `golang.org/x/*`；新增任何依赖必须走 ADR 并核对许可证。
- 不借鉴 AGPL/GPL 项目的源码；参考 Apache-2.0、MIT、BSD 项目的思路时保留归属并遵守其条款（见第 24 章）。
- 首版优先形成"打开→遍历页面→导出显示列表→提取文字/图片"闭环，写入与编辑能力后续立项。

## 2. 产品范围与边界

### 2.1 核心使用场景

| 编号 | 场景 | 关键输出 |
| --- | --- | --- |
| 1 | PDF 转 PPTX（pdf2pptx） | 显示列表、字体信息、图片原始码流 |
| 2 | 文本与布局提取 | 带坐标的文字、行、词、阅读顺序基础信息 |
| 3 | 资源提取 | 图片、嵌入字体、附件、元数据 |
| 4 | 文档体检 | 加密状态、字体嵌入、Tagged 结构、生成器信息、损坏诊断 |
| 5 | 评测与回归 | 能力报告、与基准引擎的输出对比（仅测试环境） |

### 2.2 文件和运行时边界

| 对象 | 首版策略 |
| --- | --- |
| PDF 版本 | 1.0–1.7 与 2.0 的常见子集 |
| 损坏文件 | 默认严格模式；`Repair` 选项开启重建，并在诊断中标注 |
| 加密 | 支持空用户密码与调用方提供密码；公钥加密显式失败 |
| 写入/编辑/合并 | 首版不做，G7 阶段按需立项 |
| 渲染 | 首版不做；G6 仅提供有限光栅化，用于评测与缩略图 |
| 表单与签名 | 识别并只读暴露字段；签名透传，不校验 |
| 复杂图像码流 | CCITT G3/G4 首版自研解码（V1.2 调整）；JBIG2、JPEG2000 首版透传原始码流并定义外部解码器接口，解码后续立项 |
| 扫描件 | 识别"整页图片、无可选文字"并标注；OCR 不在核心范围 |
| 文件体积与页数 | 由资源预算控制，不设硬编码上限（见第 15 章） |
| 并发 | 同一 `Document` 的页面解析可并发，对象缓存并发安全；单个页面句柄不保证并发安全 |

### 2.3 特性能力矩阵

本矩阵声明各特性的**目标档位**，不表达已验证状态；发布声明以第 17 章测试证据为准。档位调整属于能力范围变更，须按 ADR 流程记录。

| 档位 | 含义 |
| --- | --- |
| P 透传 | 识别并保留原始字节或码流，不提供语义视图 |
| R 只读识别 | 解析并输出语义与诊断，允许近似或降级 |
| F 完整读取 | 全量读取并纳入金样验收 |
| W 写入 | 提供写入 API（G7 之后立项） |

| 能力组 | 特性 | 关键结构 | 目标档位 | 目标阶段 |
| --- | --- | --- | --- | --- |
| 文件结构 | 经典 xref 表、trailer、增量更新 | `xref`、`Prev` | F | G0 |
| 文件结构 | xref 流与对象流 | `/Type /XRef`、`/ObjStm` | F | G0 |
| 文件结构 | 线性化文件 | `/Linearized` | R（按普通文件读取） | G0 |
| 文件结构 | 损坏重建 | 扫描 `obj/endobj`、`startxref` | F | G0 |
| 过滤器 | Flate、LZW、ASCII85、ASCIIHex、RunLength、Predictor | `/Filter`、`/DecodeParms` | F | G0 |
| 过滤器 | DCT（JPEG） | `/DCTDecode` | R（基于 `image/jpeg`，CMYK/Adobe 变体按诊断降级） | G3 |
| 过滤器 | CCITTFax（G3/G4） | `/CCITTFaxDecode` | F（V1.2：纯 Go 自研解码，见 ADR-018） | G3 |
| 过滤器 | JBIG2、JPX | `/JBIG2Decode`、`/JPXDecode` | P（透传 + 外部解码器接口） | G3；解码后续立项 |
| 加密 | RC4-40/128、AESV2（AES-128） | `/Encrypt`、`/V 1-4` | F | G4 |
| 加密 | AESV3（AES-256，R5/R6） | `/V 5` | F（V1.2：前移至 G2 末，见 18.2） | G2 末–G3 |
| 加密 | 公钥安全处理器 | `/Filter /Adobe.PubSec` | 显式失败 | — |
| 文档层 | 页面树及继承属性 | `/Pages`、`/Resources`、`/MediaBox`、`/CropBox`、`/Rotate` | F | G0 |
| 文档层 | 页面标签、大纲、命名目标 | `/PageLabels`、`/Outlines`、`/Dests` | R | G5 |
| 文档层 | 可选内容（图层）可见性 | `/OCProperties`、`/OC` | F（隐藏内容不输出） | G3 |
| 内容流 | 图形状态、路径、裁剪、文字、XObject、内联图片 | 内容流操作符 | F | G1–G3 |
| 内容流 | 轴向/径向渐变 | `sh`、Shading 类型 2/3 | R（输出渐变描述） | G3 |
| 内容流 | 函数式、网格渐变 | Shading 类型 1、4–7 | P（标记降级） | G3 |
| 内容流 | Tiling 图案 | `/PatternType 1` | R | G5 |
| 透明 | 常量透明度 | `ca`、`CA` | F | G3 |
| 透明 | 混合模式、软蒙版、透明组 | `BM`、`SMask`、`/Group` | R（记录，不精确合成） | G5 |
| 字体 | Type1（标准 14 字体、内嵌 FontFile） | `/Type1`、`/FontFile` | F | G1–G2 |
| 字体 | TrueType | `/TrueType`、`/FontFile2` | F | G2 |
| 字体 | Type0/CIDFontType2/CIDFontType0（CFF） | `/Type0`、`/FontFile3` | F | G2 |
| 字体 | Type3 | `/Type3`、`/CharProcs` | R（字形作为路径输出） | G2 |
| 字体 | 预定义与嵌入 CMap、ToUnicode | `/Encoding`、`/ToUnicode` | F | G2 |
| 字体 | 竖排书写 | `WMode 1`、`W2`、`DW2` | R | G5 |
| 颜色 | DeviceGray/RGB/CMYK、CalGray/CalRGB、Lab、Indexed | 颜色空间 | F（统一到 sRGB，近似项写诊断） | G3 |
| 颜色 | ICCBased | `/ICCBased` | R（按 `N` 分量使用备用空间，不做 ICC 变换） | G3 |
| 颜色 | Separation、DeviceN 与 PDF Function（类型 0/2/3/4） | `/Function` | R | G3 |
| 图像 | 图像、掩码、SMask、Decode 数组 | `/Image` | F（保留原编码） | G3 |
| 结构 | Tagged PDF 结构树、角色映射 | `/StructTreeRoot` | R（V1.2：轻量读取[角色+MCID+ActualText]前移 G3，完整版 G5） | G3/G5 |
| 元数据 | Info、XMP | `/Info`、`/Metadata` | R | G0 |
| 注释 | 链接 | `/Link` | R | G5 |
| 注释 | 其他注释、AcroForm、签名、附件 | `/Annots`、`/AcroForm` | P / R（字段只读） | G5 |
| 渲染 | 有限光栅化 | — | R | G6 |
| 写入 | 创建、增量更新、合并、覆盖编辑 | — | W | G7 |

### 2.4 不做清单与立项过滤器

首版明确**不做**：完整渲染器、PDF 表单填写、数字签名校验与签署、PDF/A 与 PDF/UA 合规校验、OCR、JavaScript 动作执行、3D 与多媒体注释。

新增能力立项需同时满足：(1) 有下游真实需求或语料证据；(2) 能在显示列表或诊断中表达，不破坏公共契约；(3) 有可复现的金样或基准对比；(4) 不引入新的外部依赖。

### 2.5 PDF 规范版本策略（V1.1 新增）

方案以 ISO 32000-1:2008（PDF 1.7）为主体，叠加 ISO 32000-2:2020（PDF 2.0）的新增与变更；对在 2.0 中被弃用的特性，读取层仍保持兼容，因为存量文件大量使用。逐条款的对应关系见附录 C。

| 情形 | 策略 |
| --- | --- |
| PDF 1.0–1.6 | 按 1.7 语义读取，文件头版本仅用于诊断 |
| PDF 1.7 | 主体依据 |
| PDF 2.0 | 在 1.7 基础上叠加差异；差异项在附录 C 的"PDF 2.0 差异"列逐项记录 |
| 文件头版本与 Catalog 的 `Version` 不一致 | 取较高者，诊断 `DOC-VERSION-MISMATCH` |
| 未知更高版本 | 按已知子集读取，未识别特性写入诊断，不拒绝打开 |
| 扩展级别（`Catalog/Extensions`） | 解析并暴露；对应 AES-256 的历史扩展仍兼容读取 |
| PDF/A、PDF/UA、PDF/X | 视为核心规范的子集，不单独实现；其 XMP 标识只读暴露，仅用作语料标签与线索 |

引用规范时只记录条款号与我方理解，不复制规范原文；条款号在建仓时须对照正式文本核对。

## 3. 总体架构与模块职责

### 3.1 分层

| 层 | 职责 | 关键约束 |
| --- | --- | --- |
| 语法层 | 词法、对象模型、xref、对象流、增量更新、重建 | 惰性解析；每个对象与流受预算约束 |
| 过滤器层 | 流解码与 Predictor | 输出长度与压缩比上限，防压缩炸弹 |
| 安全层 | RC4/AES 解密、密钥派生、权限位 | 与语法层解耦，通过接口注入 |
| 文档层 | Catalog、页面树、继承属性、元数据、大纲 | 页面句柄惰性，关闭后访问返回稳定错误 |
| 内容流层 | 操作符解释器、图形状态、文本状态、路径、裁剪、XObject 递归 | 递归深度与操作符数量设上限 |
| 字体层 | 字体加载、编码、CMap、字宽、字体程序读取、ToUnicode | 最大长尾；全部降级写诊断；解析基于 `x/image/font/sfnt`（V1.2，ADR-019） |
| 颜色与图像层 | 颜色空间转换、Function 求值、图像对象与掩码 | 近似转换必须标注 |
| 显示列表层 | 把内容流层结果投影为稳定公共类型 | 对外主 API，遵循第 10 章规格 |
| 文字辅助层 | 字形→词→行切分、空格恢复（可选子包 `pdf/text`） | 仅做几何层面处理，不做语义重排 |
| 诊断层 | 能力报告、错误定位、降级记录 | 全层共用，格式见第 14 章 |

### 3.2 目录

```text
go-pdf/
  pdf.go  document.go  page.go  options.go  errors.go     # 公共入口
  displaylist/        # 显示列表公共类型与消费接口
  text/               # 可选：词、行切分与空格恢复
  raster/             # 可选：有限光栅化（G6）
  cmd/pdf/            # CLI
  internal/syntax/    # 词法、对象、xref、重建
  internal/filter/    # 过滤器与 Predictor（含 CCITT，V1.2）
  internal/crypt/     # 加解密
  internal/doc/       # 页面树、元数据、大纲
  internal/content/   # 内容流解释器
  internal/font/      # 字体、CMap、字宽、字体程序（度量与轮廓基于 x/image/font/sfnt）
  internal/color/     # 颜色空间与 Function
  internal/image/     # 图像对象与掩码
  internal/budget/    # 统一资源预算
  internal/testkit/   # 语料、金样、对照工具
  testdata/           # 语料与金样（按生成器×特性标签组织）
```

### 3.3 数据流

```text
字节源 → 语法层(对象/xref) → [安全层解密] → 过滤器层(流解码)
      → 文档层(页面树/资源) → 内容流层(解释器 + 字体/颜色/图像层)
      → 显示列表层 → 消费者(pdf2pptx / 文字辅助 / CLI)
```

所有层通过 `budget.Budget` 共享资源计数，通过 `diag.Collector` 共享诊断收集，二者随 `context.Context` 传入。

## 4. 语法层与重建机制

### 4.1 对象模型

语法层输出的对象类型是内部类型，不对外暴露：`Null`、`Bool`、`Int`、`Real`、`String`（原始字节）、`Name`、`Array`、`Dict`、`Stream`（字典加原始流位置）、`Ref{Num, Gen}`。外部只能通过 `Document`、`Page`、`FontInfo` 等稳定类型访问。

### 4.2 读取顺序

1. 从文件尾向前查找 `startxref` 与 `%%EOF`，容忍尾部垃圾字节和 `%PDF-` 之前的前导垃圾。
2. 解析 xref：经典表、xref 流、混合引用文件（`XRefStm`）均支持；沿 `Prev` 链合并，循环与过深链路报错。
3. 对 `/Type /ObjStm` 的对象流按需展开，展开后的对象缓存入口使用 `(objstm, index)` 标识。
4. 解析 `Encrypt`，建立解密器（第 5 章）。
5. 以 `Root` 为入口读取 Catalog；任何一步失败且启用 `Repair` 时进入重建模式。

### 4.3 重建模式

- 线性扫描全文件，匹配 `N G obj … endobj`，同一编号取**最后一次**出现。
- 对象流内的对象通过逐个展开所有 `ObjStm` 补全。
- 从找到的 `trailer` 或 `/Type /Catalog`、`/Type /XRef` 对象恢复 `Root`、`Info`、`Encrypt`、`ID`。
- 重建后文档标记 `Repaired=true`，诊断记录原因、扫描范围和丢失对象数量。
- 默认严格模式下，任何需要重建的情形返回 `ErrCorrupt` 并携带诊断；仅 `WithRepair(true)` 时才继续。

### 4.4 容错规则

| 异常 | 严格模式 | 宽松（Repair）模式 |
| --- | --- | --- |
| 流 `Length` 错误或为间接引用失败 | 错误 | 向后搜索 `endstream`，诊断记录 |
| 缺少 `endobj` | 错误 | 以下一个 `obj` 或文件尾结束 |
| 数字格式异常（`+-3`、`1.2.3`） | 错误 | 按最长合法前缀解析，诊断记录 |
| 字符串转义非法 | 错误 | 保留字面字节，诊断记录 |
| `Count` 与实际页数不符 | 以实际遍历为准 | 同左，诊断记录 |
| 页面树循环引用 | 错误 | 跳过已访问节点，诊断记录 |
| 对象/数组嵌套深度超限（V1.2 新增） | 错误 | 截断并诊断记录 |

### 4.5 惰性与缓存

对象按需解析，使用带容量上限的 LRU 缓存；缓存容量与对象总数计入资源预算。已关闭的 `Document` 再访问页面或对象返回稳定的 `ErrClosed`。

## 5. 过滤器与安全

### 5.1 过滤器链

流解码按 `Filter` 数组顺序依次执行，每一步输出都受**输出长度**与**压缩比**两个上限约束。解码接口以 `io.Reader` 形式暴露，避免一次性整块载入内存。图像类过滤器中 DCT 在 G3 基于标准库解码；CCITT G3/G4 在 G3 自研解码（V1.2，ADR-018）；JBIG2、JPX 在流层面不解码，透传并由图像层通过外部解码器接口决定（见 9.3）。

### 5.2 Predictor

支持 TIFF Predictor 2 与 PNG Predictor 10–15，参数 `Columns`、`Colors`、`BitsPerComponent` 必须经范围校验（防止整数溢出和超大行缓冲）。

### 5.3 压缩炸弹防护

单个流解码输出超过 `MaxStreamBytes`、压缩比超过 `MaxInflateRatio`、或累计解码字节超过 `MaxTotalDecoded` 时立即中止，返回 `ErrBudget` 并写入诊断。

### 5.4 加密

| 项 | 规则 |
| --- | --- |
| 处理器 | 仅 `Standard`；`Adobe.PubSec` 显式失败 |
| 算法 | V1/V2（RC4）、V4（RC4 或 AESV2，经 CF/StmF/StrF）、V5（AESV3，R5/R6） |
| 密钥派生 | R2–R4 按 MD5 流程；R5 使用 SHA-256；R6 使用规范规定的哈希迭代（2.B 算法） |
| 对象密钥 | RC4 与 AESV2 按对象号/世代号派生；AESV3 直接使用文件密钥 |
| 不解密对象 | `Encrypt` 字典本身、`XRef` 流、对象流内部的字符串（由对象流整体解密） |
| 密码 | 先试空用户密码；失败后使用 `WithPassword`；仍失败返回 `ErrPassword` |
| 元数据 | 遵守 `EncryptMetadata=false` |
| 权限位 | 解析并暴露，不强制拒绝读取，由调用方决定处理策略 |

阶段安排（V1.2 调整）：RC4/AESV2 随 G4 完成前的解释器开发同步验证；AESV3（R5/R6）前移至 G2 末–G3（见 18.2），理由是 R5/R6 自 Acrobat X 起即为加密文件主流，前移可让 G0–G3 语料与联调金样覆盖加密件。加密实现仅依赖标准库 `crypto/*`，不引入第三方密码学库。

### 5.5 密码规范化与规范版本差异（V1.1）

以下内容按我对规范的理解给出，实现前须对照 ISO 32000-1/-2 核对。

- R6（AES-256）以 2.0 为准；R5 是 2.0 之前的扩展修订，仍兼容读取。
- R5/R6 的密码按 UTF-8 编码，先做 Unicode 规范化（SASLprep 类处理）并按规范截断；R2–R4 使用 PDFDocEncoding 或 Latin-1。库内部按版本转换，调用方通过 `WithPassword` 传入普通字符串；另提供 `WithPasswordBytes` 绕过转换，用于疑难文件排查。
- RC4 与 AES-128 在 2.0 中被弃用（按我的理解）：读取保持兼容，不提供写入；使用旧算法的文件产生 Info 级诊断 `CRY-DEPRECATED-ALG`。
- 增加含非 ASCII 密码的语料，覆盖各修订版本。

## 6. 文档层

### 6.1 页面树

页面树遍历使用显式栈、访问集合与深度上限；`Count` 仅作提示，不作依据。继承属性（`Resources`、`MediaBox`、`CropBox`、`Rotate`）沿父节点逐级解析，子节点覆盖父节点。

### 6.2 页面空间约定（核心约定）

显示列表中所有坐标统一为**页面空间**：

- 单位为 pt（已乘 `UserUnit`）；
- 原点在页面左上角，Y 轴向下；
- 已应用 `Rotate`，页面宽高为旋转后的可见尺寸；
- 以 `CropBox`（缺省回退 `MediaBox`）为可见区域，超出区域的元素保留并标记 `OutOfBounds`。

同时返回原始 `MediaBox`、`CropBox`、`Rotate` 和 PDF 原始坐标变换矩阵，供需要逆向计算的调用方使用。

### 6.3 可选内容（图层）

读取 `OCProperties` 的默认配置（`D`，含 `BaseState`、`ON`、`OFF`、`AS`），内容流中 `BDC /OC` 与 XObject 的 `/OC` 属性按成员关系判定可见性。**默认不输出不可见内容**；`WithHiddenLayers(true)` 时输出并带 `Layer` 标记。

### 6.4 元数据

读取 `Info` 字典（编码按 PDFDocEncoding 与 UTF-16BE 判定）和 XMP（原始 XML 字节透传，并提取常用字段）；`Producer` 与 `Creator` 作为生成器标签用于语料统计和兼容率分析。

### 6.5 元数据优先级（V1.1）

文档信息字典的多数字段在 2.0 中被弃用（按我的理解），推荐使用 XMP。因此：生成器标签与标题等字段**优先取 XMP**（如 `pdf:Producer`、`xmp:CreatorTool`），XMP 缺失时回退 `Info`；两者冲突时同时暴露并标注。语料的"生成器"标签取最终采用的值。

### 6.6 Tagged 结构读取（V1.2 拆分）

结构树读取拆为两级：

- **轻量读取（G3）**：提取角色名（含 RoleMap 解析）、`MCID`→显示项映射、`ActualText`，供 pdf2pptx 在联调期即获得标题/段落/表格线索；
- **完整读取（G5）**：完整结构树、命名空间、属性与全部结构类型。

轻量读取的输出格式与完整版兼容（只读子集），后续升级不破坏下游。

## 7. 内容流解释器

### 7.1 操作符覆盖

| 分组 | 操作符 |
| --- | --- |
| 图形状态 | `q Q cm w J j M d ri i gs` |
| 路径构造 | `m l c v y h re` |
| 路径绘制 | `S s f F f* B B* b b* n` |
| 裁剪 | `W W*` |
| 文本对象与状态 | `BT ET Tc Tw Tz TL Tf Tr Ts` |
| 文本定位与显示 | `Td TD Tm T* Tj TJ ' "` |
| 颜色 | `CS cs SC SCN sc scn G g RG rg K k` |
| 着色 | `sh` |
| 外部对象 | `Do`（Image、Form） |
| 内联图像 | `BI ID EI` |
| 标记内容 | `MP DP BMC BDC EMC` |
| Type3 | `d0 d1` |
| 兼容 | `BX EX`（范围内未知操作符静默忽略） |

### 7.2 解释规则

- 文本渲染矩阵：`Trm = [Tfs·Th 0 0 Tfs 0 Trise] × Tm × CTM`；字符前进量按字宽、`Tc`、`Tw`（仅单字节码 32）、`Tz` 计算，`TJ` 数组中的调整量以千分之一文本空间单位计。
- 未知操作符与操作数不足：忽略该操作符，诊断记录；不终止整页。
- `q/Q` 失衡：页面结束时自动补齐，诊断记录。
- 裁剪路径在 `W/W*` 后随下一个路径绘制操作符（含 `n`）生效，入栈保存；每个显示项引用其当前裁剪。
- `EI` 终止判定使用「后随空白、后续合法操作符」的启发式，避免误截断二进制数据。

### 7.3 Form XObject 递归

展开 Form 时依次应用 `Matrix`、按 `BBox` 生成裁剪、合并 `Resources`（缺省继承页面资源）；设置递归深度上限与访问路径集合，循环引用立即中止并写诊断。Form 可选择**展开**（默认）或**保留为分组**（`WithPreserveForms`），后者在显示列表中产生 `Group` 项，便于下游保留重复元素（页眉页脚、水印）。

## 8. 字体与 CMap

字体层是整个项目中最大的长尾，必须首先建立语料和诊断，再逐步扩展。字体程序解析与度量基于 `golang.org/x/image/font/sfnt`（V1.2，ADR-019），仅当 sfnt 不满足需求（如 CFF 逐字符轮廓、Type3 场景）时自研差异部分，避免低价值重复劳动并压缩 fuzz 面。

### 8.1 字符码到 Unicode 的解析优先级

1. 标记内容的 `ActualText`（V1.2 调整，ADR-017）：`BDC/EMC` 范围内若存在 `ActualText`，优先作为该区域文字的 Unicode 来源；与字形映射结果冲突时以 `ActualText` 输出并写入诊断 `CNT-ACTUALTEXT-CONFLICT`；
2. 字体字典的 `ToUnicode` CMap；
3. 预定义 CMap 与 `UCS2`/`UTF16` 对应表（Type0 字体）；
4. 简单字体：`Encoding` 与 `Differences` 加字形名，通过 Adobe Glyph List（含 `uniXXXX`、`uXXXX` 规则）映射；
5. 嵌入 TrueType 的 `cmap`、`post` 表反查；
6. 以上均失败：输出 `U+FFFD` 并置 `UnicodeOK=false`，同时记录字体与字符码，供下游决定走 OCR 或以图形保留。

### 8.2 字宽

| 字体类型 | 字宽来源 |
| --- | --- |
| 简单字体 | `FirstChar`、`Widths`；缺失回退 `MissingWidth`；标准 14 字体回退内置度量 |
| Type0/CID | `W`、`DW`；竖排使用 `W2`、`DW2` |
| Type3 | `Widths` 与 `FontMatrix` |

标准 14 字体的内置度量表随库编译进二进制，体积需在 ADR 中评估并仅保留宽度，不保留字距对。

### 8.3 CJK 与预定义 CMap

- 需内置的预定义 CMap 按语料驱动分批：先覆盖简体中文（Adobe-GB1 系列，含 `GBK-EUC-H/V`、`UniGB-UCS2-H/V`、`UniGB-UTF16-H/V`、`GB-EUC-H/V`），其次繁体、日文、韩文。
- 资源采用构建期生成的紧凑二进制并以 `embed` 引入；数据来源与许可必须在建仓前核对并记录在 `THIRD_PARTY.md`。
- 非嵌入的 CJK 字体（仅引用字体名）仍可提取 Unicode 与位置，下游自行选择替代字体。

### 8.4 字体程序读取

基于 `x/image/font/sfnt` 读取转换所需信息，不做字体渲染：

| 格式 | 读取内容 |
| --- | --- |
| TrueType/OpenType（`FontFile2`、`FontFile3`） | `head`、`hhea`、`hmtx`、`cmap`、`post`、`name`、`OS/2`（粗斜体、字族、权重）、`glyf`/`CFF ` 轮廓（经 sfnt） |
| CFF（`FontFile3`） | 字体名、字符集、CID 到 GID 映射；sfnt 不覆盖的逐字符 CFF 轮廓提取列为自研差异项 |
| Type1（`FontFile`） | 字体名、编码向量、字形名 |

输出的 `FontInfo` 包含：`BaseFont`（去除子集前缀 `ABCDEF+`）、`Family`、`Bold`、`Italic`、`Serif`、`Mono`、`Embedded`、`Subset`、`Kind`、上升下降高度、`ItalicAngle`，以及可选的字体程序原始字节引用，供下游决定是否嵌入或替换。

### 8.5 Type3

Type3 字形是内容流，默认把每个字形展开为 `Path`（带 `FromType3` 标记）并同时输出对应的 `TextRun`（若可推导 Unicode），由下游选择。

## 9. 颜色与图像

### 9.1 颜色

| 颜色空间 | 转换策略 |
| --- | --- |
| DeviceGray、DeviceRGB | 直接转换 |
| DeviceCMYK | 使用固定的近似公式转换到 sRGB，诊断标记 `ColorApprox`；公式在 ADR 中固定，避免版本漂移。**原始 CMYK 分量始终透传**（V1.2，见 `Paint.RawComponents`），供下游对专色/品牌色做二次精确转换 |
| CalGray、CalRGB、Lab | 按规范公式转换到 sRGB |
| ICCBased | 按 `N` 回退对应 Device 空间，不做 ICC 变换 |
| Indexed | 查表后按基础空间转换 |
| Separation、DeviceN | 通过 `tintTransform`（PDF Function）求值到备用空间 |
| Pattern | 渐变输出 `Shading` 项；平铺图案先标记降级 |

PDF Function 支持类型 0（采样）、2（指数插值）、3（拼接）、4（PostScript 计算，限制指令数和栈深）。

### 9.2 图像

- 输出项始终携带原始字典信息（宽高、位深、颜色空间、`Decode`）与原始编码标识。
- `ImageRef` 提供两种读取方式：`Raw()` 返回原始码流（保留 DCT 等原编码），`Decode()` 返回 `image.Image`（支持的编码范围内：G3 起含自研 CCITT 解码）。
- `ImageMask`（1 位模板）、`Mask`（色键与显式掩码）、`SMask`（软蒙版）作为关联对象输出，由下游合成；go-pdf 不预先合成。**SMask 合成验证（V1.4 新增）**：下游合成结果的 alpha 通道以 PSNR 金样验证（初始门槛 ≥ 45 dB，首轮固化），预乘 alpha 与插值语义纳入金样测试。
- 内联图像与 XObject 图像输出同一类型，标记 `Inline`。
- 解码前先按宽高与位深估算内存并经预算检查。
- **CMYK 统一转换（V1.4 调整）**：图像解码层对 CMYK JPEG 等统一执行 CMYK→sRGB 转换，转换公式版本固化并随 `THIRD_PARTY.md` 记录（与 ADR-008 同一近似体系）；转换结果以 ΔE 指标进入闭环校验，不再停留在「标记降级」。原始码流仍经 `Raw()` 透传，供下游按需自行处理。

### 9.3 外部解码器接口（V1.2 新增）

JBIG2、JPX 首版透传，但 G3 即定义解码器注册接口，避免下游联调时才发现契约缺口：

```go
package image // internal/image 定义，公共出口通过 ImageRef.Decode 暴露

type ExternalDecoder interface {
    CanDecode(enc ImageEncoding) bool
    Decode(ctx context.Context, raw []byte, info ImageInfo, budget *budget.Budget) (image.Image, error)
}

// 库内注册点：RegisterExternalDecoder(d ExternalDecoder)
// 默认无注册实现时 Decode() 返回 ErrUnsupported 并写诊断 IMG-NO-DECODER
```

外部解码器（如 cgo 封装的 JBIG2/JPX 库）由调用方以独立 module 提供，不进入 go-pdf 核心依赖。

**解码契约（V1.4 新增，ADR-024）**：页面含透传编码图像（JBIG2/JPX）且未注册外部解码器时，`Decode()` 返回 `ErrUnsupported` 并写诊断 `IMG-NO-DECODER`；区域渲染（`RenderRegion`）遇到此类元素**必须显式失败并携带该诊断，禁止静默输出缺图页面**——下游（pdf2pptx）据此改走 sidecar/外部引擎路径。

### 9.4 图像转码策略约定（V1.2 新增）

`Raw()` 原码流优先；当调用方需要目标格式而原编码不适用（如 Flate 编码需转 JPEG）时，转码由调用方或下游负责，go-pdf 仅保证 `Decode()` 正确性。转码质量参数、二次压缩是否计为质量损失的约定，在 go-pdf 与 pdf2pptx 的对接约定（第 21 章）中明确。

### 9.5 渐变采样规则（V1.4 新增，ADR-025）

连续渐变函数（类型 2/3 及拼接函数）输出为离散色标时，按以下规则采样：

- 按函数曲率自适应采样，相邻色标感知色差 **ΔE ≤ 2**，色标上限 **64**；
- 双圆心径向渐变等超出 DrawingML 表达能力的情形，输出近似描述并同时给出**误差界**（最大 ΔE 估计值），作为下游降级决策的一等输入；
- 采样结果写入 `Shading.Stops`，误差界写入诊断与 `Capabilities()`，供 pdf2pptx 在「近似渐变」与「区域栅格」之间量化决策。

## 10. 显示列表规格（对下游的核心契约）

### 10.1 设计原则

1. **稳定**：类型与字段遵循版本化兼容策略，只增不删；版本号 `displaylist.Version` 随 minor 递增。
2. **自描述**：每个项携带绘制顺序、裁剪、透明度、图层与标记内容引用，下游无需回读 PDF。
3. **可降级**：go-pdf 未能精确表达的内容以带标记的近似项输出，并附诊断，不静默丢弃。
4. **与 PPTX 对齐**：路径模型对齐 OOXML 的 `moveTo/lnTo/cubicBezTo/close`，便于 pdf2pptx 映射到 `a:custGeom`。

### 10.2 核心类型

```go
package displaylist

type Matrix [6]float64 // a b c d e f，页面空间
type Point struct{ X, Y float64 }
type Rect struct{ X0, Y0, X1, Y1 float64 }

type Page struct {
    Index         int
    Width, Height float64 // pt，已应用 Rotate 与 UserUnit
    Rotate        int
    MediaBox      Rect
    CropBox       Rect
    Clips         []Clip       // 裁剪池，Item 通过索引引用
    Items         []Item       // 按绘制顺序（Z 递增）
    Diag          []Diagnostic
}

type Common struct {
    Z         int     // 绘制顺序
    Clip      int     // Page.Clips 索引，-1 无裁剪
    Alpha     float64 // 综合填充透明度，1 不透明
    Blend     string  // 记录用，Normal 为空
    Layer     string  // 可选内容名，空表示无
    MCID      int     // 标记内容 ID，-1 无
    StructRef int     // 结构树节点索引，-1 无
    Flags     Flags   // OutOfBounds、Approx、FromType3 等
}

type Item interface{ Base() *Common }

type TextRun struct {
    Common
    Font       *FontInfo
    Size       float64
    Fill       Paint
    Stroke     *Paint
    RenderMode int
    Vertical   bool
    Glyphs     []Glyph
}

type Glyph struct {
    Text      string  // Unicode，可为空
    UnicodeOK bool    // 是否来自可信映射
    Code      uint32  // 字符码
    CID       uint32
    Origin    Point   // 页面空间基线原点
    Advance   float64 // 沿书写方向的前进量（pt）
    Matrix    Matrix  // 字形到页面空间的完整变换
}

type Path struct {
    Common
    Segments []Segment // MoveTo/LineTo/CubicTo/Close
    Fill     *Paint
    FillRule FillRule  // NonZero / EvenOdd
    Stroke   *Paint
    Line     LineStyle // 线宽、端点、连接、斜接限制、虚线
}

type Image struct {
    Common
    Matrix Matrix // 单位正方形到页面空间
    Info   ImageInfo
    Data   ImageRef
    Mask   *MaskRef
}

type Shading struct { Common; Kind ShadingKind; Geometry any; Stops []ColorStop; Extend [2]bool }
type Group   struct { Common; Name string; Items []Item } // 仅 WithPreserveForms

type Paint struct {
    Kind     PaintKind  // Solid / Shading / Pattern（降级）
    RGBA     [4]float64 // Solid 时有效，已转 sRGB
    Space    string     // 原始颜色空间名
    Raw      []float64  // 原始颜色分量（V1.2 新增），按 Space 解释；CMYK/专色供下游二次转换
    Approx   bool       // 是否近似转换
}
```

### 10.3 消费方式

```go
// 流式：低内存，适合大文件与并发
err := page.Walk(ctx, func(it displaylist.Item) error { ... })

// 物化：便于分析与测试
dl, err := page.DisplayList(ctx)
```

两种方式产出的项与顺序必须逐项一致，由测试强制。

### 10.4 兼容、演进与消费方验证（V1.2 强化）

- 新字段只能追加，旧字段含义不变；枚举新增值须在文档中标注引入版本。
- 移除或语义变更须递增 major，并按 ADR 记录迁移方案。
- pdf2pptx 在启动时校验 `displaylist.Version` 的 major 兼容。
- **消费方契约测试（V1.2 新增）**：pdf2pptx 在其 CI 中锁定一组显示列表金样的序列化哈希（JSON 规范化后）；go-pdf 任何字段级变更通过联调流水线触发对方 CI 预警，防止 `Version` 手工递增遗漏。

### 10.5 文字事件语义

显示列表以字形运行（TextRun）为单位输出，不在 go-pdf 内合并为段落。`text` 子包提供基于几何的辅助切分（字→词→行），依据字宽、字间距、字体度量与基线对齐恢复空格和换行；段落合并、列检测、阅读顺序、表格识别由 pdf2pptx 的版面分析层负责。

`ActualText` 的处理规则见 8.1：优先采用并诊断冲突（V1.2，ADR-017）。

### 10.6 V1.1 扩展字段（面向高保真转换）

高保真转换需要更多"可追溯、可比对、可度量"的信息。以下字段只增不删，属于兼容变更，`displaylist.Version` 升为 1.1（`Major=1, Minor=1`）。

```go
// Common 增加
Bounds Rect   // 页面空间轴对齐包围盒，便于版面分析
ID     int    // 页内稳定编号（同输入同版本稳定）
Source SrcRef // 来源：内容流操作符序号与对象号，用于追溯与诊断
Sig    uint64 // 内容签名（类型、样式、相对几何、文字或图像哈希），用于跨页重复检测

// TextRun 增加
CharSpace, WordSpace, HScale, Rise float64
Flags TextFlags // FakeBold、Stroked、Invisible、Clipped

// FontInfo 增加
type FontMetrics struct {
    UnitsPerEm int
    Ascent, Descent, CapHeight, XHeight, AvgWidth int
    Advances map[uint32]int // 按字符码或 CID 的字宽（千分单位）
    Substituted bool       // 字体未嵌入且度量来自回退
}

// Path 增加
Hint   PathHint // Rect / RoundRect / Ellipse / Line / Polyline / Generic
Closed bool

// Image 增加
Interpolate bool; Decode []float64; Role ImageRole // Photo / Mask / Background / Tile

// Page 增加
Links      []Link
Background *BackgroundHint // 整页铺底矩形或图片
Scanned    bool            // 整页图像且无可选文字
```

设计说明：`Sig` 支撑下游母版提取；`FontMetrics` 与 `Advances` 支撑字宽补偿；`Hint` 让下游无需自行识别矩形与直线；`Scanned` 与 `Source` 支撑降级决策与报告追溯。

## 11. 结构、大纲与注释

| 项 | 输出 | 说明 |
| --- | --- | --- |
| Tagged PDF | 结构树节点（角色、层级、`MCID`、`Alt`、`ActualText`） | 显示项通过 `StructRef` 关联；轻量读取 G3、完整版 G5（V1.2 拆分，见 6.6）；Office 类生成器的标题、段落、表格线索可显著提升下游语义还原 |
| 角色映射 | `RoleMap` 解析到标准角色 | 自定义角色保留原名并附映射结果 |
| 大纲 | 树形标题与目标页 | 命名目标与显式目标均解析 |
| 页面标签 | 逻辑页码字符串 | 如 `i`、`ii`、`1` |
| 链接注释 | 区域矩形、URI 或目标页 | 输出为独立列表，不混入显示列表 |
| 其他注释 | 类型、区域、内容文本 | 外观流默认不展开 |
| 表单字段 | 名称、类型、值（只读） | 不执行 JavaScript |
| 附件 | 文件名、大小、原始字节引用 | 只提供读取，不自动解压 |

## 12. 文字辅助子包与有限光栅化

### 12.1 `pdf/text`

输入显示列表，输出按几何关系切分的字、词、行：

- 空格恢复：以字体空格宽度、字间距与相邻字形间隙之比判定，**默认阈值取间隙 > 0.15 × 字号 × wordSpace 判定为空格（V1.2 给出初始经验值，区间 0.10–0.30 随语料校准）**；
- 行聚类：按基线、字号和书写方向聚类，支持小幅上下标偏移；
- 竖排与旋转文字按各自方向独立处理；
- 去重：同位置叠加的重复文字（伪粗体、阴影）可选合并。

该子包不判断段落、栏、阅读顺序与表格，保持几何层面的确定性输出。

### 12.2 `pdf/raster`（G6，可选）

只提供评测与缩略图所需的有限光栅化：填充与描边路径、图像、基础渐变、裁剪与常量透明度；不承诺像素级与参考引擎一致。纯 Go 实现，不引入 CGO；文字使用嵌入字体轮廓，缺失字体时以占位方式绘制并写诊断。

### 12.3 区域渲染接口（G3b）

pdf2pptx 的"局部降级"需要把页面某个区域渲染成图片，因此从 G6 中拆出**区域渲染**并前移为 G3b；G6 保留为整页评测级光栅化。

```go
img, err := page.RenderRegion(ctx, box Rect, dpi float64, opt RenderOpts)

type RenderOpts struct {
    Transparent bool                // 背景透明或带白底
    IncludeText bool                // 是否绘制文字（默认 true）
    Exclude     func(Item) bool     // 排除指定显示项（按 ID 或谓词）
    Layers      []string            // 可选内容图层覆盖
}
```

- 覆盖范围：路径填充与描边、图像（含掩码）、裁剪、常量透明度、轴向与径向渐变（近似）、文字（使用嵌入字体轮廓）。
- `Exclude` 的用途：只渲染被降级的元素，或渲染"去掉文字后的背景"，使下游能在栅格背景上叠加可编辑文字而不重复显示。
- 范围外：混合模式、软蒙版的精确合成、叠印；遇到时写入诊断并近似。
- 不承诺与参考引擎像素级一致；质量要求以降级与评测可用为界。外部渲染引擎可通过 pdf2pptx 的 `Rasterizer` 适配器替换本接口。
- 依赖：字形轮廓读取（FNT-05，基于 x/image/font/sfnt）；受 `MaxRasterPixels` 预算约束。
- **DPI 约定（V1.4 新增）**：默认 220 DPI，上限 300 DPI，纳入评测指标与金样；默认值与上限随首轮语料固化。

## 13. 公共 API 与错误模型

### 13.1 文档与页面

```go
doc, err := pdf.OpenFile(ctx, "in.pdf",
    pdf.WithPassword(""),          // 可选
    pdf.WithRepair(false),         // 默认严格
    pdf.WithBudget(pdf.DefaultBudget),
    pdf.WithHiddenLayers(false),
    pdf.WithPreserveForms(false),
)
if err != nil { return err }
defer doc.Close()

n := doc.NumPages()
pg, err := doc.Page(0) // 0 起始

pg.Size()                              // 页面空间宽高
pg.Walk(ctx, fn)                       // 流式显示列表
dl, err := pg.DisplayList(ctx)         // 物化显示列表
imgs, err := pg.Images(ctx)            // 图像清单（含原始编码）
fonts, err := pg.Fonts(ctx)            // 页面使用的字体信息

doc.Info(); doc.Outline(ctx); doc.StructTree(ctx)
doc.Capabilities(ctx)                  // 能力报告（第 14 章）
doc.Diagnostics()                      // 全局诊断
doc.Encrypted(); doc.Repaired(); doc.Producer()
```

除 `Close` 外，所有可能阻塞的方法接收 `context.Context`。`Document` 与已解析对象缓存并发安全；`Page` 句柄仅保证单协程使用，并发需各自 `doc.Page(i)`。

### 13.2 错误

| 错误 | 含义 |
| --- | --- |
| `ErrCorrupt` | 文件结构损坏且未启用 Repair，或 Repair 失败 |
| `ErrPassword` | 需要密码或密码错误 |
| `ErrUnsupported` | 命中显式不支持特性（如公钥加密） |
| `ErrBudget` | 超出资源预算 |
| `ErrClosed` | 文档已关闭 |
| `ErrCanceled` | 上下文取消或超时 |

所有错误统一包装为 `*pdf.Error{Code, Op, Offset, Obj, Page, Err}`，支持 `errors.Is/As` 与 `Unwrap`（V1.2 明确）；错误信息不得包含文档文本内容，避免日志泄露。

## 14. 诊断与能力报告

### 14.1 诊断

```go
type Diagnostic struct {
    Level  Level  // Info / Warn / Degraded / Error
    Code   string // 如 SYN-LEN-MISMATCH、FNT-NO-TOUNICODE
    Msg    string
    Page   int    // -1 全局
    Obj    int    // 对象号，0 无
    Offset int64  // 文件偏移，-1 无
}
```

诊断码按前缀分类：`SYN`（语法）、`FLT`（过滤器）、`CRY`（加密）、`DOC`（文档）、`CNT`（内容流）、`FNT`（字体）、`CLR`（颜色）、`IMG`（图像）、`BUD`（预算）。`Degraded` 表示输出已降级但仍可用，下游据此决定是否告警。

### 14.2 能力报告

`Capabilities()` 汇总文档实际用到的特性及处理结果，格式与 go-pptx 的 capability manifest 风格一致：

```json
{
  "pages": 12, "producer": "Microsoft Word", "repaired": false,
  "features": [
    {"name": "font.type0.cid",  "status": "supported", "count": 3},
    {"name": "font.tounicode.missing", "status": "degraded", "count": 1, "pages": [4]},
    {"name": "shading.mesh",    "status": "passthrough", "count": 1, "pages": [7]},
    {"name": "image.jbig2",     "status": "passthrough", "count": 2}
  ]
}
```

`status` 取值：`supported`、`approximate`、`degraded`、`passthrough`、`unsupported`。pdf2pptx 以此判断每页是否可按「可编辑」路径转换，或需降级为图片。

## 15. 安全与资源预算

### 15.1 预算项

| 预算项 | 作用 | 建议初始默认值 |
| --- | --- | --- |
| `MaxFileSize` | 输入文件大小 | 512 MiB |
| `MaxObjects` | 加载对象数量 | 5,000,000 |
| `MaxStreamBytes` | 单流解码输出 | 256 MiB |
| `MaxTotalDecoded` | 累计解码字节 | 2 GiB |
| `MaxInflateRatio` | 压缩比 | 按流类型区分，图像类放宽 |
| `MaxXRefChain` | `Prev` 链长度 | 256 |
| `MaxPageTreeDepth` | 页面树深度 | 64 |
| `MaxFormDepth` | Form XObject 递归 | 16 |
| `MaxNestingDepth` | 对象/数组/字典嵌套深度（V1.2 新增） | 64 |
| `MaxOperators` | 单页操作符数 | 20,000,000 |
| `MaxPathSegments` | 单页路径段数 | 5,000,000 |
| `MaxImagePixels` | 单图像解码像素 | 256 Mpx |
| `MaxFunctionSteps` | PostScript 函数指令数 | 100,000 |
| `MaxConcurrency` | 批量解析/页面流水线协程数上限（V1.3 新增） | `GOMAXPROCS` |
| `Timeout` | 单次操作超时 | 由调用方 `context` 控制 |

默认值仅为起点，需用第 17 章语料校准后固化；所有项均可通过 `WithBudget` 覆盖。

### 15.2 威胁模型

恶意或畸形文件可能导致：循环引用、超深嵌套、压缩炸弹、巨大尺寸图像、无限循环、整数溢出、超大 `Count` 或数组、对解析器 panic。对策：所有循环用访问集合与计数器；所有分配先经预算检查；所有整数运算先做范围检查；解析入口使用 `recover` 转为 `ErrCorrupt`；以原生 fuzz 持续覆盖。

### 15.3 隐私

日志与错误信息不包含页面文本、元数据值和字体名以外的文档内容；诊断只记录结构信息、对象号和偏移。

## 16. 性能工程（V1.3 扩展：从约束到设计目标）

性能是本项目的对外亮点之一。叙事锚点：**批量吞吐 ≥ MuPDF 2×（页/秒/核）+ 纯 Go 无 CGO**——单线程 C 引擎的每核吞吐不可战胜，但页面级并行 × 无 CGO 边界 × 确定性内存预算，使服务端批量场景成为主场。性能按四层级设计，全部纳入测试与发布门槛。

### 16.1 L1 架构层：零拷贝与流式

- **字节源抽象**：`Open` 接受 `io.ReaderAt`（内存 / mmap / 文件统一视图）；仅前向流式源退化为只读顺序模式并写诊断；
- **惰性解码**：对象、流、字体、图像全部按需解码；字体 / CMap / 颜色空间在文档级缓存并跨页复用，缓存接口规格随 G0 冻结；
- **零拷贝切片**：语法层对象以 `[]byte` 子切片引用原始缓冲，字符串与名称延迟物化，物化前不复制；
- **显示列表双模式**：`Walk` 为生产默认（单趟、O(1) 额外内存）；`DisplayList` 物化仅限调试与测试，下游生产路径禁止物化整页（见 21 章约定）；
- **输出配合（联动 go-pptx）**：流式 zip 打包，避免整包驻留内存（需求提给 go-pptx 方案）。

### 16.2 L2 运行时层：内存与 GC 纪律

- **分配纪律**：热路径（词法器、操作符分发、字形循环）零分配目标；`allocs/op` 与耗时同权进入 CI 指标；
- **对象池**：字形、路径段、矩阵等短命对象使用 `sync.Pool`；禁止在池中存放指针逃逸数据；
- **结构体布局**：`Glyph`、`Segment` 等热结构按缓存行友好排列，字段顺序经 benchmark 验证后冻结，验证前不得"顺手优化"；
- **大对象外置**：图像码流与解码后位图经 `[]byte` 池 + 显式释放语义管理，降低 GC 扫描压力；
- **预算即水位**：`MaxTotalDecoded` 兼作内存水位目标——"解析 1 GB PDF 峰值内存稳定在预算内"是可对外宣传的指标，纳入 17.4 门槛。

### 16.3 L3 并发层：吞吐模型

- 页面级并行为**唯一默认并行粒度**（粗粒度、无锁、输出天然确定性）；
- 页内并行仅用于超大页的解释阶段，结果必须确定性合并（输出顺序与单线程一致）；
- 批量场景三级流水："解析页 N+1 ‖ 物化 / 解释页 N ‖ 消费页 N-1"，由 `budget.MaxConcurrency`（默认 `GOMAXPROCS`）控制；核心不隐式创建无上限协程；
- 约束不变：单 `Page` 句柄不保证并发安全；`Document` 与对象缓存并发安全。

### 16.4 L4 度量层：性能预算制度

- 每个工作包建立 `benchstat` 基线（含 `-benchmem`），CI 对比上一版本，**劣化 > 5% 阻断合并**；
- 评测流水线附 pprof 火焰图与分配热点报告，每次发布写入"已知性能特征"章节；
- **公开发布基准**：固定语料集 + 固定机器规格，发布四组数据——单页解析 P50/P95、批处理吞吐（页/秒/核）、峰值内存、输出体积比——与 MuPDF 的对比表同时公布，以可复现为前提（复现脚本随仓库发布）；
- 性能 SLO 矩阵见 17.4：单页延迟、批量吞吐、内存水位、体积比四个维度各有锚点。

### 16.5 基准覆盖

`go test -bench` 覆盖：语法层（词法 + xref 解析）、过滤器（含 CCITT 解码）、内容流解释器、字体解码（sfnt 度量与轮廓）、显示列表产出与 `Walk` 流式路径；基准数据入库长期对比，作为 16.4 预算制度的数据源。

## 17. 测试矩阵与发布门槛

### 17.1 语料组织

每个样本带四类标签：

| 标签 | 取值示例 |
| --- | --- |
| 生成器 | Word、WPS 文字、WPS 演示、PowerPoint、LaTeX、Chrome/Skia、LibreOffice、Acrobat Distiller、国产打印驱动等 |
| 类型 | 原生、扫描、混合 |
| 特性 | 中文 CID 字体、无 ToUnicode、增量更新、对象流、加密、渐变、透明、大表格、竖排、Type3 等 |
| 语言 | 中文、英文、中英混排、日韩 |

语料以「生成器 × 特性」覆盖矩阵管理；缺口以清单形式跟踪。语料来源与许可逐文件登记，涉密或客户文件不进入公共仓库。

### 17.2 必须覆盖的测试

- 语法层：xref 表与流、对象流、混合引用、增量更新、重建模式的金样与故障注入；
- 过滤器：每种过滤器与 Predictor 的边界用例，压缩炸弹用例；CCITT G3/G4 各模式边界用例（V1.2）；
- 加密：各算法与版本的有密码、空密码、错误密码、`EncryptMetadata`、含非 ASCII 密码（V1.2）；
- 内容流：操作符覆盖、`q/Q` 失衡、Form 递归与循环、内联图像边界；
- 字体：每类字体的字符码到 Unicode 映射、字宽、中文 CMap、无 ToUnicode 降级、`ActualText` 优先级与冲突（V1.2）；
- 显示列表：`Walk` 与 `DisplayList` 一致性、页面空间坐标（含 `Rotate`、`UserUnit`、`CropBox`）；
- 能力报告与诊断：特性命中与实际输出一致；
- fuzz：词法、xref、过滤器、内容流、CMap、PDF Function、字体表。

### 17.3 对照基准（仅测试环境）

以成熟引擎（如 pdfium、MuPDF、pdf.js、Poppler）的输出作为「预言机」：对比文字内容、字形位置误差与页面渲染图。这些引擎仅在 CI 与评测工具中使用，不进入发布产物，也不以其源码作为实现依据。

### 17.4 发布硬门槛

以下数值为初始建议，须在首轮语料评估后固化：

1. 纯 Go 构建（`CGO_ENABLED=0`）通过，覆盖 linux/darwin/windows 的 amd64 与 arm64；
2. 主流生成器原生 PDF 的文字字符召回率不低于 98%，位置误差统计达标；
3. 性能 SLO 矩阵达标（V1.3 扩展为四维度，数值首轮固化）：单页解析 P50 ≤ MuPDF 1.5×；**批量吞吐 ≥ MuPDF 2×（页/秒/核，靠页面级并行兑现）**；峰值内存 ≤ 预算水位且内存曲线可复现；输出体积比 ≤ 基准链路 1.2×；
4. 语料中不出现未恢复的 panic，所有失败为带诊断的错误；
5. 各 fuzz 目标累计运行达到约定时长且无新增崩溃；
6. 资源预算用例全部通过，内存峰值在约定上限内；
7. 能力矩阵中声明为 F 的特性均有金样；声明为 P/R 的特性均有降级与诊断用例；
8. 第三方依赖与内嵌资源的许可审计通过；
9. 无已知高危的 `govulncheck` 结果。

### 17.5 元素级保真门槛（V1.4 新增，ADR-026）

页级指标之外，按元素类型设验收门槛（首轮语料评测后固化）：

| 元素 | 指标 | 初始门槛 |
| --- | --- | --- |
| 文字 | 字形位置 P95 偏差 | ≤ 0.3 pt |
| 表格 | 网格线位置 P95 偏差 / 合并单元格识别正确率 | ≤ 0.5 pt / ≥ 95% |
| 图片 | 解码像素 SSIM（对原码流解码基准）/ SMask 合成 alpha PSNR | ≥ 0.99 / ≥ 45 dB |
| 渐变 | 采样色标与原函数最大 ΔE | ≤ 3 |
| 形状 | 几何顶点位置 P95 / 线宽误差 | ≤ 0.3 pt / ≤ 5% |

## 18. 分阶段交付

| 阶段 | 内容 | 退出标准 |
| --- | --- | --- |
| G0 | 语法层、xref 与对象流、过滤器、页面树、重建模式、预算、fuzz 与语料框架 | 语料中全部文件可打开并遍历页面，重建用例通过 |
| G1 | 内容流解释器、图形状态、标准字体与基础文字定位 | 英文原生 PDF 输出带位置文字，位置误差达标 |
| G2 | 完整字体栈：TrueType、CID、CMap、ToUnicode、中文、空格恢复 | 中文语料文字召回率达到门槛；输出 `FontMetrics` |
| G2 末 | AES-256（R5/R6）解密（V1.2 前移） | R5/R6 各密码用例通过 |
| G3 | 路径、裁剪、颜色、图像（含 CCITT 解码）、Form XObject、可选内容、结构树轻量读取 → 完整显示列表 | pdf2pptx 可开始 P0–P2 联调；V1.1 扩展字段、内容签名、路径形状提示、`Page.Scanned` 全部有金样 |
| G3b | 区域渲染（基础路径、图像、裁剪、透明、字体轮廓文字） | pdf2pptx P4 的前置 |
| G4 | 加密完善（RC4/AESV2 收尾）、更强容错、能力报告与诊断完善 | 加密语料通过，能力报告与实际一致 |
| G5 | Tagged 结构完整版、大纲、链接、注释、竖排、透明与图案记录 | 结构线索可用于 pdf2pptx 语义增强 |
| G6 | 有限光栅化 | 能自动生成评测对比图 |
| G7 | 写入、增量更新、覆盖编辑（按需立项） | 单独立项评审 |

并行规则：G0 完成时必须冻结显示列表接口 `displaylist.Version=1`，pdf2pptx 以手工构造的 IR 先开发映射与布局，不阻塞于 go-pdf 的后续阶段。

### 18.1 V1.1 阶段调整

| 调整 | 内容 |
| --- | --- |
| 新增 G3b | 区域渲染（基础路径、图像、裁剪、透明、字体轮廓文字）；依赖 G3；是 pdf2pptx P4 的前置 |
| G2 退出标准增加 | 输出 `FontMetrics`，字宽补偿所需数据齐全 |
| G3 退出标准增加 | V1.1 扩展字段、内容签名、路径形状提示、`Page.Scanned` 全部有金样 |
| G6 重新定义 | 整页评测级光栅化与基准引擎对比统计，不再承担区域渲染 |

### 18.2 V1.2 阶段调整

| 调整 | 内容 | 理由 |
| --- | --- | --- |
| AES-256 前移至 G2 末–G3 | R5/R6 自 Acrobat X 起为加密文件主流，前移使 G0–G3 语料与联调金样覆盖加密件 | 避免联调期遇到加密件即硬失败 |
| CCITT G3/G4 自研解码列入 G3 | 扫描件是区域渲染与局部降级的核心场景，透传会导致该区域渲染不可用 | 纯 Go 实现量可控（ADR-018） |
| 结构树读取拆分为轻量（G3）/完整（G5） | pdf2pptx P0–P2 联调期需要标题/表格线索 | 轻量版仅角色 + MCID + ActualText |
| 字体程序解析基于 x/image/font/sfnt | 避免手写 glyf/CFF 解析器的重复劳动与 fuzz 面 | 见 ADR-019 |

## 19. 关键决策记录

| 编号 | 决策 | 原因 |
| --- | --- | --- |
| ADR-001 | 纯 Go、无 CGO | 与 go-pptx 一致，部署简单，便于交叉编译 |
| ADR-002 | 显示列表是对下游唯一契约 | 隔离 PDF 复杂度，便于测试与版本化 |
| ADR-003 | 首版不做写入与渲染 | 聚焦读取质量，避免范围膨胀 |
| ADR-004 | 不借鉴 AGPL/GPL 源码 | 避免许可污染；参考其他许可项目仅取思路并保留归属 |
| ADR-005 | 段落、表格、阅读顺序放在 pdf2pptx | go-pdf 保持几何层面的确定性 |
| ADR-006 | 页面空间：左上原点、pt、已应用 Rotate | 与 PPT 坐标系对齐，下游无需处理 PDF 坐标 |
| ADR-007 | 默认严格，`Repair` 显式开启 | 避免静默容错掩盖问题 |
| ADR-008 | CMYK 使用固定近似公式，ICC 不做变换 | 可复现；颜色精度作为已知限制；原始分量经 `Paint.Raw` 透传供下游二次转换（V1.2 补充） |
| ADR-009 | OCR 与版面模型外置 | 保持核心纯 Go 与体积可控 |
| ADR-010 | CMap 与标准字体度量以 `embed` 内置 | 避免运行时外部资源依赖；许可核对在建仓前完成 |
| ADR-011 | 基准引擎仅用于测试 | 不进入发布依赖 |
| ADR-012 | DCT 使用标准库 `image/jpeg` | 避免自研；CMYK/Adobe 变体按诊断降级 |

### 19.1 V1.1 新增决策记录

| 编号 | 决策 | 原因 |
| --- | --- | --- |
| ADR-013 | 区域渲染前移为 G3b，与整页评测级光栅化（G6）分离 | pdf2pptx 的局部降级必须有区域渲染能力 |
| ADR-014 | 规范版本策略：以 PDF 1.7 为主体，叠加 2.0 差异，旧特性仍兼容读取 | 存量文件多为 1.x，且仍使用被 2.0 弃用的特性 |
| ADR-015 | 显示列表按兼容方式增补转换所需字段（`Version` 升 minor） | 支撑母版提取、字宽补偿、降级追溯，不破坏已有下游 |
| ADR-016 | 附录 C 条款映射表是能力声明的追溯依据；未映射条款与金样的特性不得声明为 F 档 | 对标 go-pptx 的条款与金样可追溯要求 |

### 19.2 V1.2 新增决策记录

| 编号 | 决策 | 原因 |
| --- | --- | --- |
| ADR-017 | `ActualText` 优先于字形映射结果，冲突时输出 `ActualText` 并写诊断 | 与主流引擎（pdf.js/MuPDF）行为对齐；扫描+隐藏文本层场景依赖 ActualText |
| ADR-018 | CCITT G3/G4 自研纯 Go 解码并前移 G3；JBIG2/JPX 透传但 G3 定义外部解码器注册接口 | 扫描件是区域渲染的核心场景；JBIG2/JPX 自研成本高且可由外部解码器补齐 |
| ADR-019 | 字体程序解析与度量基于 `golang.org/x/image/font/sfnt`，仅自研 sfnt 不覆盖的差异部分 | 避免手写 glyf/CFF 解析器的重复劳动与 fuzz 面；`golang.org/x/*` 在依赖白名单内 |
| ADR-020 | 以 MuPDF 为参照系设定相对性能 SLO，不承诺绝对数字 | 绝对数字不可预知；相对锚点可判定、可回归 |

### 19.3 V1.3 新增决策记录

| 编号 | 决策 | 原因 |
| --- | --- | --- |
| ADR-021 | 建立性能预算制度：`benchstat` 基线 + `-benchmem` 同权 + 劣化 >5% 阻断合并 | 性能是设计目标而非事后测量；回归防劣化需要硬门禁而非"需说明" |
| ADR-022 | 页面级并行为唯一默认并行粒度，页内并行必须确定性合并 | 粗粒度并行无锁、输出顺序稳定，与 ADR-010（下游字节可复现）兼容 |
| ADR-023 | 公开发布基准数据（四组指标 + MuPDF 对比表 + 复现脚本） | 性能亮点的可信度来自可复现的公开数字，而非单次测试截图 |
| ADR-024 | 图像解码契约：透传编码未注册解码器时区域渲染显式失败，禁止静默输出缺图页面 | 「页面缺图」是事故级缺陷而非偏差，必须显式失败并让下游选择替代路径 |
| ADR-025 | 渐变按曲率自适应采样（ΔE ≤ 2，上限 64 色标），近似情形输出误差界 | 色标数量直接决定渐变保真；误差界使下游降级决策可量化 |
| ADR-026 | 建立元素级保真门槛（17.5），与页级指标并行考核 | 页级 SSIM 会掩盖单类元素的系统性偏差；表格/渐变/图片需各自的可验收指标 |

## 20. 开发工作包与完成标准

| 编号 | 工作包 | 前置 | 完成标准 |
| --- | --- | --- | --- |
| CORE-01 | module、CI、许可证、THIRD\_PARTY | 无 | `CGO_ENABLED=0` 构建；示例编译；基准（`-benchmem`）基线入库 |
| BUD-01 | 预算与诊断收集器 | CORE-01 | 预算项（含 `MaxNestingDepth`、`MaxConcurrency`）与诊断码单测通过 |
| SYN-01 | 词法与对象解析 | CORE-01 | 数字、字符串、名称、边界金样通过 |
| SYN-02 | xref 表/流、对象流、`Prev` 链 | SYN-01 | 混合引用与增量更新金样通过 |
| SYN-03 | 重建模式 | SYN-02 | 损坏语料恢复用例通过 |
| FLT-01 | Flate、LZW、ASCII85、ASCIIHex、RunLength | SYN-01 | 边界与炸弹用例通过 |
| FLT-02 | Predictor | FLT-01 | TIFF/PNG 预测金样通过 |
| FLT-03 | CCITT G3/G4 解码（V1.2 新增） | FLT-01 | G3 1D/2D、G4 各模式金样与边界用例通过 |
| CRY-01 | 标准安全处理器 RC4/AES、AES-256 R5/R6（V1.2 合并前移） | SYN-02、FLT-01 | 各版本有/空/错误/非 ASCII 密码用例通过 |
| DOC-01 | 页面树、继承属性、页面空间 | SYN-02 | `Rotate`、`UserUnit`、`CropBox` 金样通过 |
| DOC-02 | 元数据与可选内容 | DOC-01 | Info/XMP、图层可见性用例通过 |
| CNT-01 | 内容流词法与操作符分发 | DOC-01 | 全部操作符覆盖用例通过 |
| CNT-02 | 图形状态、路径、裁剪 | CNT-01 | 路径与裁剪金样通过 |
| CNT-03 | Form XObject 与内联图像 | CNT-02 | 递归与循环用例通过 |
| FNT-01 | 简单字体与标准 14 度量 | CNT-01 | 字宽与编码金样通过 |
| FNT-02 | TrueType 与 CID 字体、Type0 | FNT-01 | 中文语料映射通过 |
| FNT-03 | CMap 与 ToUnicode | FNT-02 | 预定义与嵌入 CMap 金样通过；`ActualText` 优先级用例通过（V1.2） |
| FNT-04 | CFF、Type1、Type3 | FNT-02 | 字体程序读取用例通过 |
| CLR-01 | 颜色空间与 PDF Function | CNT-02 | 各空间转换金样通过；`Paint.Raw` 分量透传用例通过（V1.2） |
| IMG-01 | 图像对象、掩码、`image/jpeg` | CNT-03、CLR-01 | 原始码流与解码输出一致 |
| IMG-02 | CCITT 解码集成与外部解码器接口（V1.2 新增） | FLT-03、IMG-01 | 扫描件语料解码通过；无解码器时诊断正确 |
| DL-01 | 显示列表类型与 `Walk`/`DisplayList` | CNT-03、FNT-03、CLR-01、IMG-01 | `Version=1` 冻结，两种方式一致 |
| TXT-01 | `pdf/text` 字词行切分 | DL-01 | 空格恢复（含默认阈值）与行聚类指标达标 |
| STR-01a | 结构树轻量读取（V1.2 新增） | DOC-01、DL-01 | 角色 + MCID + ActualText 金样通过 |
| STR-01b | Tagged 结构完整版（原 STR-01） | STR-01a | 完整结构线索金样通过 |
| CAP-01 | 能力报告 | DL-01 | 声明与实际一致 |
| CLI-01 | CLI 与 JSON 输出 | DL-01 | 命令与退出码用例通过 |
| FUZZ-01 | fuzz 目标与语料种子 | SYN-01 起逐步 | 达到发布门槛时长 |
| RAS-01 | 有限光栅化（G6） | DL-01 | 评测对比图自动生成 |

前置任务未通过时不得标记依赖功能完成。

### 20.1 V1.1 新增工作包

| 编号 | 工作包 | 前置 | 完成标准 |
| --- | --- | --- | --- |
| DL-02 | 显示列表 V1.1 扩展字段（`Bounds`、`ID`、`Source`、`Sig`、文字、路径、图像、页面扩展） | DL-01 | 字段金样与兼容性测试通过；pdf2pptx 联调金样通过 |
| FNT-05 | 字体度量输出与字形轮廓读取（基于 `x/image/font/sfnt`） | FNT-04 | 度量与轮廓金样通过 |
| SHP-01 | 路径形状提示识别（矩形、圆角矩形、椭圆、直线、折线） | CNT-02 | 识别金样与误判率达标 |
| SCN-01 | 扫描页识别（`Page.Scanned`） | DL-02 | 扫描与原生混合语料识别正确 |
| RAS-02 | 区域渲染接口（G3b） | DL-02、FNT-05、CLR-01、IMG-01 | 区域渲染金样通过；排除项正确；预算生效 |
| DOC-03 | XMP 优先的元数据与生成器标签 | DOC-02 | 冲突标注金样通过 |
| CRY-02 | R6 密码规范化与旧算法诊断 | CRY-01 | 含非 ASCII 密码语料通过 |
| SPEC-01 | 条款映射表维护与覆盖检查脚本 | CORE-01 | CI 校验：F 档特性均有条款与金样 |

## 21. 与 pdf2pptx 的对接约定

| 主题 | 约定 |
| --- | --- |
| 输入 | pdf2pptx 只消费 `displaylist` 与能力报告，不直接访问 go-pdf 内部类型 |
| 契约验证 | pdf2pptx 启动时校验 `displaylist.Version` major；双方 CI 通过显示列表金样序列化哈希做消费方契约测试（V1.2，见 10.4） |
| 尺寸与单位 | 页面空间 pt；转换 EMU 使用 1 pt = 12700 EMU；幻灯片尺寸由 pdf2pptx 按页面宽高或目标比例决定 |
| 文字 | `TextRun` 为最小输入；`UnicodeOK=false` 的字形由 pdf2pptx 决定是否以图形保留或交给 OCR；`ActualText` 已由 go-pdf 优先采用并标注冲突 |
| 字体 | `FontInfo` 提供族名、粗斜体、嵌入标志；`FontMetrics` 供字宽补偿；字体替换策略由 pdf2pptx 负责 |
| 路径 | `Path` 与 OOXML 的 `moveTo/lnTo/cubicBezTo/close` 对齐；对应 go-pptx 的自定义几何写入能力（需提前评估） |
| 图像 | 优先使用 `Raw()` 原编码，避免二次压缩；转码由 pdf2pptx 负责并约定质量参数（V1.2）；掩码与软蒙版由 pdf2pptx 决定合成或降级；JBIG2/JPX 经外部解码器或降级处理 |
| 解码契约（V1.4） | 含透传编码且未注册外部解码器时，`RenderRegion` 显式失败（`IMG-NO-DECODER`），pdf2pptx 必须改走 sidecar/外部引擎，禁止输出缺图页面 |
| 渐变误差（V1.4） | `Shading.Stops` 按 9.5 采样；近似情形的最大 ΔE 误差界供 pdf2pptx 量化「近似渐变 vs 区域栅格」决策 |
| 颜色 | `Paint.RGBA` 为 sRGB 归一值；需要精确专色时 pdf2pptx 可读取 `Paint.Raw` 原始分量（V1.2） |
| 渐变与透明 | `Shading`、`Alpha` 映射到 `a:gradFill` 与 alpha；go-pptx 暂不支持的写入项按降级清单处理 |
| 结构线索 | G3 起提供轻量结构读取（角色 + MCID + ActualText），完整版 G5 增强 |
| 降级协商 | `Flags` 与 `Diagnostic` 驱动 pdf2pptx 的逐页降级策略，并写入 pdf2pptx 自己的转换报告 |
| 联调 | 双方共享一批手工构造的显示列表金样，用于在 go-pdf 完成前并行开发 |

## 22. CLI

| 命令 | 作用 |
| --- | --- |
| `pdf info` | 基本信息：页数、版本、加密、生成器、是否重建 |
| `pdf capability` | 输出能力报告（JSON） |
| `pdf dump --page N --format json` | 导出指定页显示列表 |
| `pdf text` | 按行输出文字（含坐标选项） |
| `pdf images` | 导出图像（保留原编码） |
| `pdf fonts` | 列出字体与嵌入状态 |
| `pdf check` | 结构检查与诊断汇总 |

约定：默认输出 JSON；退出码区分成功、带降级成功、损坏、需密码、预算超限、内部错误；`--budget` 与 `--repair` 对应库选项。

## 23. 工具链、依赖与发布管理

- Go 版本：建仓时取当时最新稳定版，并声明最低支持版本；CI 覆盖最低版本与最新版本。
- 依赖：标准库与 `golang.org/x/*`（含 `x/image/font/sfnt`，V1.2）；新增依赖走 ADR 与许可审计。
- CI：`go vet`、`staticcheck`、`-race`、单元测试、**基准与 `benchstat` 基线对比（劣化 >5% 阻断合并，V1.3）**、fuzz 冒烟、交叉编译、`govulncheck`、许可扫描。
- 内嵌资源：CMap 与字体度量表由生成工具在构建期产出，生成脚本与数据来源纳入仓库并随 `THIRD_PARTY.md` 登记许可。
- 发布：遵循语义化版本；显示列表 `Version` 与模块 major 独立管理；每次发布附能力矩阵与已知限制。

## 24. 参考资料

以下资料作为实现依据入口；涉及具体结构的功能须在代码或测试清单记录规范章节、支持版本与金样来源。仓库建立时补充并核对各资料的官方链接与访问方式。

- &#91;S1\] ISO 32000-1:2008（PDF 1.7）与 ISO 32000-2:2020（PDF 2.0）
- &#91;S2\] Adobe CMap 与 CID 字体规范（Technical Note #5014）及 CJKV 字符集与 CMap 说明（Technical Note #5094）
- &#91;S3\] Adobe Glyph List 规范
- &#91;S4\] OpenType/TrueType 规范（`head`、`hhea`、`hmtx`、`cmap`、`post`、`name`、`OS/2`）与 CFF 规范（Technical Note #5176）；`golang.org/x/image/font/sfnt`（V1.2 实现依据）
- &#91;S5\] Adobe Type 1 字体格式
- &#91;S6\] PDF Association 提供的 PDF 技术资料与测试语料入口（使用前逐项核对许可）
- &#91;S7\] [Go：compress/flate](https://pkg.go.dev/compress/flate)、[crypto/aes](https://pkg.go.dev/crypto/aes)、[image/jpeg](https://pkg.go.dev/image/jpeg)、[x/image/font/sfnt](https://pkg.go.dev/golang.org/x/image/font/sfnt)
- &#91;S8\] pdfcpu（Apache-2.0）：文件层 validate/relaxed 设计思路参考；MuPDF/pdfium/pdf.js：解释层与文本层架构参照系（仅测试对照，不借鉴源码，见 ADR-004/011）

## 附录 A 版本变更记录

| 版本 | 日期 | 变更 |
| --- | --- | --- |
| V1.0 | 2026-10-13 | 初版：定位与边界、能力矩阵、分层架构、显示列表契约、字体与 CMap 策略、预算与测试、阶段与工作包、与 pdf2pptx 对接约定 |
| V1.1 | 2026-10-13 | 新增 2.5 PDF 规范版本策略、5.5 密码规范化与规范版本差异、6.5 元数据优先级、10.6 显示列表 V1.1 扩展字段、12.3 区域渲染接口（G3b）、18.1 阶段调整、19.1 ADR-013 至 016、20.1 新增工作包、附录 C 条款映射表（含 PDF 2.0 差异）；修正 10.5 的排版问题 |
| V1.4 | 2026-10-20 | 保真度专项整改：9.2 CMYK 统一转换（ΔE 闭环校验）与 SMask 合成 PSNR 验证；9.3 图像解码契约（透传编码未解码时显式失败，禁缺图）；9.5 渐变采样规则（ΔE ≤ 2、上限 64、误差界输出）；12.3 区域渲染 DPI 默认 220/上限 300；17.5 元素级保真门槛（17.4 之外新增五类元素指标）；21 章对接约定新增解码契约与渐变误差行；19.3 ADR-024 至 026；附录 B 新增 Q12 |
| V1.3 | 2026-10-20 | 性能工程设计：第 16 章扩为「性能工程」专章（L1 零拷贝流式 / L2 GC 纪律 / L3 并发吞吐模型 / L4 性能预算制度）；15.1 新增 `MaxConcurrency`；17.4 门槛扩展为四维度 SLO 矩阵（批量吞吐 ≥ MuPDF 2× 页/秒/核）；19.3 ADR-021 至 023；CORE-01/BUD-01 完成标准纳入 `-benchmem`；23 章 CI 增加 benchstat 硬门禁；附录 B 新增 Q11 |
| V1.2 | 2026-10-20 | 竞品对标整改与路线决策：1.1 生态位与"与 pdfcpu 的关系"明确；2.3 CCITT 升 F 档（G3 自研）、JBIG2/JPX 透传 + 外部解码器接口（9.3 新增）；AES-256（R5/R6）前移至 G2 末–G3（5.4、18.2）；8.1 `ActualText` 优先级（ADR-017）；FNT-05 基于 `x/image/font/sfnt`（ADR-019）；`Paint.Raw` 原始颜色分量透传（9.1、10.2）；9.4 图像转码策略约定；6.6 结构树读取拆分为轻量（G3）/完整（G5）；12.1 空格恢复默认阈值经验值；15.1 新增 `MaxNestingDepth`；16/17.4 相对性能 SLO 锚点（ADR-020）；10.4 消费方契约测试；13.2 明确 `Unwrap`；19.2 ADR-017 至 020；20 新增 FLT-03、IMG-02、STR-01a 工作包；附录 B 关闭 Q2/Q3、修订 Q4；附录 A 补充 S8 |

## 附录 B 待决事项

| 编号 | 事项 | 默认处理 |
| --- | --- | --- |
| Q1 | 首版是否需要写入或编辑能力 | 默认不需要，列入 G7 |
| ~~Q2~~ | ~~预定义 CMap 与标准字体度量的数据来源与许可~~ | **V1.2 关闭**：Adobe CMap 资源与 Glyph List 允许免费再分发（具体条款以官方文本为准），按 S2/S3 登记 `THIRD_PARTY.md` |
| ~~Q3~~ | ~~语料来源与许可，涉密样本的存放方式~~ | **V1.2 关闭**：公共语料与内部语料分库管理，涉密样本不入公共仓库 |
| Q4 | 是否需要纯 Go 的 CCITT、JBIG2、JPX 解码 | **V1.2 修订**：CCITT G3/G4 自研纯 Go 解码并列入 G3（ADR-018）；JBIG2、JPX 透传 + 外部解码器接口，是否自研视扫描件需求再评估 |
| Q5 | 正式 module path 与仓库命名 | 建仓时确定 |
| Q6 | 预算默认值与召回率门槛 | 首轮语料评估后固化；SLO 锚点数值同步固化（ADR-020） |
| Q7 | 区域渲染（G3b）自研范围，尤其文字轮廓与渐变的覆盖程度 | 先覆盖路径、图像、裁剪、常量透明度与嵌入字体文字（基于 sfnt 轮廓）；其余近似并诊断 |
| Q8 | ISO 32000-2 的获取方式与许可，以及附录 C 条款号与 2.0 差异的核对责任人 | 建仓前指定责任人，核对完成前附录 C 标注为"待核" |
| Q9 | 显示列表 V1.1 字段是否满足 pdf2pptx 的全部需求 | 以 pdf2pptx P0–P1 联调结果为准，必要时按兼容方式增补 |
| Q10 | 相对 SLO 的具体数值（vs MuPDF 的倍率与召回率） | **V1.3 扩展**：按 17.4 四维度矩阵（延迟 / 吞吐 / 内存 / 体积）分别固化；批量吞吐锚点 ≥2×（页/秒/核） |
| Q11 | 公开发布基准的机器规格与语料子集 | 随首次发布确定并冻结，复现脚本随仓库发布（V1.3 新增） |
| Q12 | 区域渲染 DPI 默认值与降级栅格清晰度阈值 | 默认 220 / 上限 300 起步，按语料首轮固化（V1.4 新增） |

## 附录 C 规范条款映射表（含 PDF 2.0 差异）

使用方式：能力矩阵（2.3）中每个 F 档特性必须至少对应一条条款和一个金样；没有对应条款视为遗漏。章节号按我对 ISO 32000-1:2008 目录的记忆整理，须在建仓时对照正式文本逐条核对；"PDF 2.0 差异"列同样是待核对项，核对前不得据此做实现决策。

| ISO 32000-1 章节 | 主题 | 目标档位 | 工作包 | 常见偏差与处理策略 | PDF 2.0 差异（待核） |
| --- | --- | --- | --- | --- | --- |
| 7.2 | 词法约定（空白、注释、数字、字符串、名称） | F | SYN-01 | 数字格式异常、字符串转义非法：宽松解析并诊断 | 无重大变化 |
| 7.3 | 对象（布尔、数值、字符串、名称、数组、字典、流、间接对象） | F | SYN-01 | 流 `Length` 错误：搜索 `endstream`；嵌套深度超限：截断并诊断（V1.2） | 无重大变化 |
| 7.4 | 过滤器与 Predictor | F / R / P（见 2.3） | FLT-01、FLT-02、FLT-03 | 预测参数异常、截断流：部分输出并诊断 | 无重大变化 |
| 7.5 | 文件结构（头、xref、trailer、增量更新、对象流、xref 流、线性化） | F | SYN-02、SYN-03 | xref 偏移错误、尾部垃圾、`Prev` 成环：重建或环检测 | 版本信息以 Catalog 为准的处理 |
| 7.6 | 加密（标准安全处理器、权限） | F | CRY-01、CRY-02 | 缺少 ID 数组、权限位异常：容错 | 以 AES-256 R6 为准，旧算法被弃用，密码规范化 |
| 7.7 | 文档结构（Catalog、页面树、页面对象、继承属性、页面标签、名称树与数树） | F | DOC-01 | `Count` 错误、循环、缺 `Type`：以遍历为准 | 基本不变 |
| 7.8 | 内容流与资源 | F | CNT-01、CNT-03 | 资源缺失、操作数不足：忽略并诊断 | 基本不变 |
| 7.10 | 函数（类型 0、2、3、4） | R | CLR-01 | PostScript 函数超限：近似或降级 | 无重大变化 |
| 7.11 | 文件规范与附件 | R / P | STR-01 | 路径格式不统一 | 新增关联文件（Associated Files） |
| 7.12 | 扩展（扩展级别） | R | DOC-01 | 扩展字典缺失 | 部分扩展被标准化（如 AES-256 R6） |
| 8.2–8.4 | 图形对象、坐标系统、图形状态 | F | CNT-02 | 奇异的 `cm` 矩阵：跳过并诊断 | 无重大变化 |
| 8.5 | 路径构造、绘制与裁剪 | F | CNT-02 | 空路径加 `W n` | 无重大变化 |
| 8.6 | 颜色空间 | F / R | CLR-01 | ICC 配置损坏：回退备用空间 | ICC v4 支持 |
| 8.7 | 图案与着色 | R / P | CNT-02、CLR-01 | 渐变函数异常：降级 | 无重大变化 |
| 8.8–8.9 | 外部对象与图像 | F | IMG-01、IMG-02 | 位深或尺寸与数据长度不符：截断或补齐并诊断 | 无重大变化 |
| 8.10 | Form XObject | F | CNT-03 | 递归、缺 `BBox` | 无重大变化 |
| 8.11 | 可选内容（图层） | F | DOC-02 | 配置缺失：按默认可见 | 无重大变化 |
| 9.2–9.4 | 文本状态与文本对象 | F | CNT-02 | 字体缺失：诊断并回退 | 无重大变化 |
| 9.5–9.7 | 字体（简单字体、复合字体、CMap） | F | FNT-01–FNT-03 | 缺 `Widths`、无 `ToUnicode`、非标准编码：按 8.1 优先级回退 | 无重大变化 |
| 9.8–9.9 | 字体描述符与嵌入字体程序 | F / R | FNT-04、FNT-05 | 字体程序损坏：仅元数据 | 无重大变化 |
| 9.10 | 文本内容提取（`ToUnicode`、`ActualText`） | F | FNT-03、TXT-01 | `ActualText` 与字形不符：以 `ActualText` 为准并标注（V1.2，ADR-017） | 无重大变化 |
| 11 | 透明 | R | CNT-02、CLR-01 | 混合模式与软蒙版：记录并降级 | 对透明组与混合有澄清 |
| 12.3 | 文档级导航（大纲、页面标签） | R | STR-01 | 目标缺失 | 基本不变 |
| 12.5 | 注释（链接等） | R / P | STR-01 | 目标缺失 | 基本不变 |
| 12.7 | 交互表单 | R / P | STR-01 | 字段树不规范 | 基本不变 |
| 12.8 | 数字签名 | P | — | 透传，不校验 | 基本不变 |
| 14.3 | 元数据（Info、XMP） | R | DOC-02、DOC-03 | Info 与 XMP 不一致：同时暴露 | Info 多数字段被弃用，推荐 XMP |
| 14.6–14.8 | 标记内容、逻辑结构、Tagged PDF | R | STR-01a、STR-01b | 结构树缺失或不规范：忽略并诊断 | 增加命名空间与新结构类型 |
| 10 | 渲染（半色调、传递函数、叠印等） | 不做 | — | 识别并记录 | 增加黑点补偿等 |
| 13 | 多媒体、3D | 不做 | — | 识别并忽略 | 基本不变 |
| 2.0 新增 | 文档部分（DPart）等 | P | — | 识别并记录 | 新增 |
