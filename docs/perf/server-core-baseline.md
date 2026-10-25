# Server Document Core 性能基线

日期：2026-10-24
用途：记录 Core 批量文本和预检的初始性能，作为回归参考，不是 Poppler 渲染对比。

## 环境

- Go 1.25.0 linux/amd64
- Linux 6.6 WSL2，24 vCPU
- 合成 PDF：50 页 A4 横版，每页 1 个文本 run + 1 个矩形，约 20 KB
- 预热后测量；不含调用方落盘/网络开销

## 命令

```bash
./scripts/bench-server.sh
```

单项：

```bash
go test . -run '^$' -bench 'BenchmarkExtractText$' -benchmem -count=3
PDF_SLO=1 go test . -run '^TestServerCoreSLO$' -v
```

## 吞吐基准

2026-10-24 本机样本，`-count=3` 的中位数：

| 基准 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| OpenReaderAt 50 页 | 134,262 | 166,672 | 2,382 |
| ExtractText workers=1 | 1,578,929 | 3,865,278 | 8,939 |
| ExtractText workers=4 | 1,606,195 | 3,865,980 | 8,942 |
| ExtractTextEach workers=4 | 1,683,828 | 3,864,730 | 8,948 |
| Preflight workers=1 | 643,240 | 729,139 | 5,390 |
| Preflight workers=4 | 500,870 | 729,469 | 5,393 |

## P50/P95/P99

`PDF_SLO=1`，50 页，40 次采样：

| 场景 | P50 | P95 | P99 |
|---|---:|---:|---:|
| Open | 165 µs | 303 µs | 306 µs |
| ExtractText workers=4 | 1.64 ms | 2.05 ms | 2.21 ms |
| Preflight workers=4 | 441 µs | 796 µs | 826 µs |

## 解读

- 合成页非常小，ExtractText 每页约 31–34 µs；列/阅读顺序、旋转与竖排元数据会增加开销，真实文本、字体和 Form 会展开销更高。
- 50 页小文档上 `workers=4` 的文本提取与单 worker 接近，调度和缓存竞争抵消了并行收益；预检因页面扫描更重，4 workers 明显更快。
- 这些数字只用于同机回归。真实语料必须单独记录页数、字体、图像、加密、Form 深度和峰值 RSS。
- `ExtractText` 的 B/op 包含保留全部页文本；`ExtractTextEach` 用于避免长期保留结果。
- 当前不是 Poppler 对比数据；Preview 渲染性能应使用 `docs/perf/poppler-24.02-baseline.md` 的方法单独测。

## 待补

- 真实冻结语料上的 P50/P95/P99、RSS 和吞吐。
- 按页类型分层：纯文本、扫描、复杂 Form、大量图像。
- 与 Poppler `pdftotext` 同机对比，但只作为参考，不作为功能等价。
- CI 的 10% 回归门禁需要稳定 runner 和固定语料摘要。
