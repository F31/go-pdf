# P0 基线与能力审计

日期：2026-10-25
基线提交：`be6818a`（P0 改动在本次工作区）
用途：固定 P0 的能力声明、字体/质量基线和 CI 分层；不是 Poppler 像素对等声明。

## 环境

- Go 1.25.0 linux/amd64，`CGO_ENABLED=0` 构建
- Poppler 24.02.0，`pdftoppm` 24.02
- 24 vCPU，WSL2 Linux 6.6
- 语料：`docs/pdf/` 57 个文件，第一页样本，72 DPI，白底，`-cropbox -hide-annotations`
- Preview 字体：`render.BuiltinFonts()`（固定，不读取主机字体）

## 能力审计结论

已修正：

- README 曾声称 type 4/5 mesh 可渲染；实际 type 4/5 仅解析/透传 IR，当前渲染覆盖
  type 6/7 patch。已改为明确的能力描述。
- `pdf.Capabilities()` 原先把 axial/radial/function/mesh 合并为一条；已拆成三条，
  分别说明采样、patch 和 type 4/5/pattern 限制。
- Pattern 能力说明补充 mesh/function pattern 诊断与复杂 Matrix/clip 仍为近似。
- 系统字体能力说明补充 host-dependent，并要求可复现输出显式设置 `Fonts`。
- Preview 质量回归默认字体从主机系统字体改为内置字体；新增
  `POPPLER_CORPUS_FONT_DIR` 固定字体目录。
- 质量指标拆分为 Core/Preview/Extended，见 `docs/perf/quality-layers.md`。
- CI 增加 macOS/Windows 的 `CGO_ENABLED=0` 构建、vet 与 test；Linux 继续跑 race。
- 修正跨平台测试：`.gitattributes` 固定 golden 为 LF、PDF/字体/图片为 binary；Windows 跳过 POSIX shell 的 OpenJPEG fake adapter；displaylist signature 使用跨架构稳定的浮点哈希；golden JSON 使用 6 位小数容差。

未改变的能力边界：

- Core 不依赖 Poppler、Cairo、lcms2、FreeType 或外部二进制。
- type 4/5 mesh、mesh/function pattern、复杂 hinting/hatch 仍是 Preview 缺口，
  不作为 supported 声明。
- 写入、签名、打印色彩、完整 shaping 不在本程序范围。

## 固定字体 Preview 样本

命令：

```bash
POPPLER_CORPUS_PAGES=1 POPPLER_CORPUS_SHARDS=8 \
POPPLER_CORPUS_CHUNK_PAGES=8 \
POPPLER_CORPUS_DIR=/tmp/poppler-corpus-p0 \
./scripts/corpus-poppler-full.sh
```

结果：

| 指标 | 值 |
| --- | --- |
| 文件/页 | 57 / 57 |
| 渲染错误 | 0 |
| fallback（<20 dB 或 <0.85 SSIM） | 21/57（36.8%） |
| PSNR p10/p50/p90 | 19.263 / 25.121 / 34.269 dB |
| SSIM p10/p50/p90 | 0.611 / 0.900 / 0.985 |

说明：内置字体缺少 CJK 字形，相比历史系统字体样本，SSIM p10 从约 0.820 降至
0.611；这是确定性字体替换的预期结果。需要 CJK 质量时使用固定字体目录：

```bash
POPPLER_CORPUS_FONT_DIR=/usr/share/fonts ... ./scripts/corpus-poppler-full.sh
```

固定目录验证（同一 57 页样本，`POPPLER_CORPUS_FONT_DIR=/usr/share/fonts`，约 31s）：

| 指标 | 值 |
| --- | --- |
| 渲染错误 | 0 |
| fallback | 21/57（36.8%） |
| PSNR p10/p50/p90 | 19.263 / 25.739 / 34.269 dB |
| SSIM p10/p50/p90 | 0.820 / 0.903 / 0.985 |

历史全量 final8（10,866 页、系统字体、0 错误）仍是观察基线，不与本次固定字体内置样本
直接混合比较。

## P0 验收

- 能力表/README/状态文档与实现一致：通过。
- 固定字体 57 页样本可重复运行：通过，0 render_error。
- Core/Preview/Extended 指标分离：`docs/perf/quality-layers.md`。
- Linux/macOS/Windows CI 基线：通过，run 40159890850 全部 10 个 job success。
- 调试输出/临时文件清理：仓库内无 `println` 调试；已删除本地 `go-pdf.test`。
- 全量测试：`go test ./...`、`CGO_ENABLED=0 go test ./...`、`go test -race ./...`、
  `go vet ./...`、`staticcheck ./...`：通过。
- GitHub Actions 语法：`actionlint .github/workflows/ci.yml`：通过。
- 远端 CI：https://github.com/F31/go-pdf/actions/runs/40159890850（10/10 success）。
