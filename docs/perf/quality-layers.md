# 质量指标分层

本文件固定 Core、Preview、Extended 的报告边界，避免把文本正确性、屏幕渲染和外部
provider 差异混成一个分数。指标只用于同层、同参数、同版本的回归。

## 分层

| 层 | 报告 | 命令 | 主要指标 | 明确不包含 |
| --- | --- | --- | --- | --- |
| Core | `docs/perf/server-core-baseline.md` | `./scripts/bench-server.sh` | 打开、单页文本、批量提取、预检、页失败、文本一致性、P50/P95 | PSNR、SSIM、字体形状 |
| Preview | `docs/perf/poppler-corpus-baseline.md`、`docs/perf/p0-baseline.md` | `POPPLER_CORPUS=1 ... go test ./render`、`scripts/corpus-poppler-full.sh` | 固定字体/DPI/背景下的语义缺口、PSNR、SSIM、ΔE、render_error | 核心提取吞吐、外部 provider 差分 |
| Extended | 各 provider 的测试/报告（P3） | `go test ./extension/...`，真实二进制作为可选集成 | provider 名称/版本/许可/确定性、成功/失败隔离、尺寸/样本差分 | Core 总分、Poppler 像素全等 |
| 发布 | `docs/release.md`、`docs/perf/p4-baseline.md` | `./scripts/release-check.sh`、`./scripts/perf-gate.sh` | 构建/测试/依赖/边界/体积/性能 | 未校准的跨机性能数字 |

## 报告路径约定

- Core 报告只写 `docs/perf/server-core-baseline.md` 或 `/tmp/quality/core/`。
- Preview 报告只写 `docs/perf/poppler-corpus-baseline.md`、`docs/perf/p0-baseline.md`
  或 `/tmp/quality/preview/`；逐页 CSV 可能包含本地文件名，不提交。
- Extended 报告写 `/tmp/quality/extended/` 或 provider 自己的目录，不写入 Preview 报告。
- 任何报告必须记录：提交、OS/架构、Go/Poppler/FreeType、DPI、背景、ICC、字体来源、
  注释策略和语料摘要。

## 字体策略

- 默认 Preview 质量回归使用 `render.BuiltinFonts()`。
- 需要 CJK 或接近本机效果时，用 `POPPLER_CORPUS_FONT_DIR=/path1:/path2` 固定字体目录。
- `POPPLER_CORPUS_SYSTEM_FONTS=1` 仅用于本机诊断；报告必须标记 host-dependent，不能
  作为发布判定。
- 内置字体缺少 CJK 字形时，SSIM 会下降；这是可复现基线的已知限制，不是产品默认
  渲染回归。产品默认仍可使用系统字体，但跨主机不可复现。

## 判定规则

- Core 失败页、panic、错误泄漏、批量不一致是硬失败。
- Preview 语义缺陷（缺图、缺字、透明错误、mesh/pattern 静默空白）是硬失败；
  hinting/AA/hatch 的像素差异只作为观察项。
- Extended provider 缺失必须显式 `Unsupported`/`NotScanned`；外部进程失败不能拖垮 Core。
- Poppler 像素全等不是发布门禁。
