# 发布与支持边界

## 支持档位

| 档位 | 承诺 | 不承诺 |
| --- | --- | --- |
| Core | 纯 Go、`CGO_ENABLED=0`、解析/解密/文本/结构/预检、预算与取消 | 像素渲染、外部 codec、写入、签名 |
| Preview | sRGB 屏幕渲染、常见路径/图像/Transparency/Pattern/Mesh、显式近似诊断 | 打印色彩、Adobe/Poppler 像素全等、FreeType hinting 对齐 |
| Extended | 显式选择的进程/CGO provider，高保真渲染、JPX/JBIG2、可选颜色/字体后端 | Core 默认依赖、未审计的许可证、静默回退 |

## 发布门禁

```bash
./scripts/verify.sh
./scripts/release-check.sh
PERF_GATE=1 ./scripts/release-check.sh   # 同机校准后
```

`release-check.sh` 检查构建、测试、race、vet、依赖白名单、Core/provider 分离、
静态二进制和版本元数据。`quality-metadata.sh` 输出可复现实验参数。

发布构建使用 Go 1.26.10 或更高的已修复工具链。Go 1.25.0 的标准库会被
`govulncheck` 报告已知漏洞，不能作为发布工具链；CI 的 1.25.x 使用最新补丁版本。

## 质量门禁

- Core：打开/提取/预检失败页、panic、错误泄漏、批量不一致为硬失败。
- Preview：语义缺图/缺字/透明错误、mesh/pattern 静默空白为硬失败；
  hinting/AA/hatch 只作观察。
- Extended：缺失 provider 必须 `Unsupported`/`NotScanned`；外部进程失败不能拖垮 Core。
- 跨引擎：Poppler 用 `scripts/cross-engine.sh` 做页面数/加密/文本语义诊断；
  PDFium 作为后续扩展参考，不作为当前硬依赖。
- Poppler 像素全等不是发布阻塞条件。

## 性能

`scripts/perf-gate.sh` 使用同机校准的 Core open/extract/preflight P95 上限，
默认值按当前 P4 样本留 10% 余量。不同 CPU/负载不得直接复用默认值。

## 语料

- 合成 PDF 由 `internal/testkit` 运行时生成，覆盖经典 xref、xref stream、对象流、
  增量更新、加密、表单、扫描/图像、竖排、RTL、结构树和表格。
- 公开语料必须写入 `testdata/manifest.csv` 的 source/license；私有或客户语料
  只能留在内部语料库，禁止提交。
- 逐页 CSV、参考图和外部 provider 输出只写 `/tmp`，不进入发布包。

## 已知限制

- 没有 PDF 写入、页面编辑、表单保存、数字签名、打印色彩/PDF-X。
- 没有内置 JBIG2/JPX；G3 2-D 通过可选纯 Go `extension/g32d` 显式注册，未注册时 Unsupported。
- 没有完整 CFF/Type1/shaping、FreeType hinting、OCR、PDF/A 认证。
- Preview 默认系统字体会导致跨主机差异；可复现测试使用 `BuiltinFonts` 或固定字体目录。
- `displaylist` 当前 2.1，只保证增字段的 Minor 兼容，不保证 1.0 契约。
