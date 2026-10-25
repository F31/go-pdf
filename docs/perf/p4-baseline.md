# P4 质量与发布基线

日期：2026-10-25
用途：汇总 Core/Preview/Extended 的分层质量、跨引擎诊断和发布检查。不是
Poppler 像素对等声明。

## 环境

- 质量样本：Go 1.25.0 linux/amd64，`CGO_ENABLED=0` 构建
- 发布/漏洞门禁：Go 1.26.10（`govulncheck` 0 vulnerabilities）
- Poppler/pdftoppm/pdftocairo 24.02.0
- FreeType 26.1.20，系统字体 `DejaVu Sans`（仅本机诊断）
- Preview 固定基线：`render.BuiltinFonts()`、72 DPI、白底、sRGB Preview、隐藏注释

机器可读参数：

```bash
./scripts/quality-metadata.sh
```

## Core

同机 P4 样本（50 页合成文档，40 次采样，`PDF_SLO=1`）：

| 场景 | P50 | P95 |
| --- | ---: | ---: |
| Open | 0.174 ms | 0.520 ms |
| ExtractText workers=4 | 2.204 ms | 3.046 ms |
| Preflight workers=4 | 0.657 ms | 0.979 ms |

门禁：

```bash
./scripts/perf-gate.sh
```

默认 P95 上限为 0.700/3.380/1.620 ms，按本机样本留约 10% 余量。换机器必须重设
`PERF_OPEN_P95_MS`、`PERF_EXTRACT_P95_MS`、`PERF_PREFLIGHT_P95_MS`。

## Preview

| 样本 | 结果 |
| --- | --- |
| 57 页固定内置字体 | fallback 21/57，PSNR p50 25.121，SSIM p50 0.900 |
| 10,866 页 system-font full | 0 render_error，fallback 6,837，PSNR p50 25.087，SSIM p50 0.837 |
| 边缘计算胶片 02 p2 | PSNR 28.826，SSIM 0.913 |
| OpenStack 产品介绍 44 页 | 0 render_error，41 ok / 3 fallback |

完整方法见 `docs/perf/p2-baseline.md` 与 `docs/perf/poppler-corpus-baseline.md`。

## Extended

- Registry feature/status/reason 校验和确定性排序通过。
- OpenJPEG fake e2e；本机安装 OpenJPEG 2.5 后真实 JP2 round-trip 通过。
- JBIG2 `jbig2dec` fake e2e；真实样本需部署方提供。
- Poppler/Cairo 版本、许可、external feature、超时和错误截断通过。
- Core 不导入具体 provider；`check-core-deps.sh` 通过。

## 跨引擎

```bash
./scripts/cross-engine.sh <pdf> [page]
```

当前脚本用 Poppler `pdfinfo`/`pdftotext` 对比页面数、加密状态和规范化文本；
页面数/加密不一致为失败，文本差异为诊断。PDFium 作为后续可选参考，不进入
发布依赖。

## 发布检查

```bash
./scripts/release-check.sh
```

P4 本机结果：

- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`-race`、vet、staticcheck：通过
- `govulncheck`（Go 1.26.10）：0 vulnerabilities
- 依赖白名单、Core/provider 分离：通过
- 静态 strip 二进制：约 13.1 MB，`CGO_ENABLED=0`，无外部 C 库
- `go version -m` 仅含 `golang.org/x/image`、`golang.org/x/text`

## 已知限制

见 `docs/release.md`。发布前必须确认这些限制没有通过“supported”能力声明被误解。
