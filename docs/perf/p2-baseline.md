# P2 Preview 语义基线

日期：2026-10-25
用途：记录 Preview mesh/pattern/描边/资源语义补全的结果；Core 另见
`docs/perf/p1-baseline.md`。

## 修复

### Mesh shading

- type 4/5 顶点网格和 type 6/7 patch 均解码 `Raw` 并渲染。
- 支持 mesh `/Function`：按函数输入数读取颜色分量，Function 输出映射到
  ColorSpace；type 4–7 共用实现。
- mesh 流解码上限 32 MiB；空流、超限和函数错误记录
  `CNT-SHADING-MESH-RAW`、`CNT-SHADING-MESH-LIMIT`、`CLR-SHADING-FUNCTION`，
  不再把未解码流当成功空白。
- 顶点/三角形数有上限，超限返回 `RND-MESH-BUDGET`。
- 修正 `dict.Get("Function")` 对缺失键返回 `syntax.Null` 导致的误诊断。

### Pattern 子渲染资源

- tiling pattern 的每个 tile 子 renderer 在完成后调用 `releaseAll()`；
  之前每 tile 的 clip/soft-mask lease 会泄漏，跨页累计触发
  `BUD-LIVE-MEMORY`。
- 共用 cache lease 在页面解释结束时释放。
- 修复后 OpenStack 产品介绍 44 页从 8 个 render_error 降为 0。
- 新增 mesh pattern 与 tiling pattern lease 回归测试。

### 描边策略

- `PageOptions.MinStrokeWidth` 显式控制 Preview 最小描边宽度（设备像素）。
  默认 1px；负值关闭非零线宽下限；零宽 PDF hairline 仍为 1px。
- 新增 `TestStrokeHalfWidthPolicy`，覆盖细线、关闭策略和零宽线。

## 结果

### 关键页面

| 页面 | 结果 |
| --- | --- |
| 边缘计算主打胶片 02 p2 | PSNR 28.826 / SSIM 0.913，背景 mesh 正常 |
| OpenStack 产品介绍 44 页 | 0 render_error，41 ok / 3 fallback，PSNR p50 22.79，SSIM p50 0.915 |
| 57 页固定内置字体样本 | fallback 21/57，PSNR p50 25.121，SSIM p50 0.900 |

### 全量 system-font 对比

使用 `POPPLER_CORPUS_SYSTEM_FONTS=1`，与 P0 历史 full run 同字体策略：

| 指标 | final8 | final9sys |
| --- | ---: | ---: |
| 页数 | 10,866 | 10,866 |
| render_error | 0 | 0 |
| fallback | 6,850 | 6,837 |
| PSNR p10/p50/p90 | 22.594 / 25.078 / 31.199 | 22.604 / 25.087 / 31.199 |
| SSIM p10/p50/p90 | 0.795 / 0.837 / 0.903 | 0.795 / 0.837 / 0.904 |

全量报告：

- `/tmp/poppler-corpus-final9sys/poppler-corpus.csv`
- 固定内置字体全量：`/tmp/poppler-corpus-final9/poppler-corpus.csv`
  （6,889 fallback；字体不同，不与 system-font 表直接比较）

## 本地验收

```bash
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
```

P2 关键测试：

- `TestRenderMeshFunctionColors`
- `TestRenderMeshPattern`
- `TestRenderTilingPatternReleasesChildLeases`
- `TestStrokeHalfWidthPolicy`
- 既有 `TestRenderMeshShading`、`TestRenderFreeFormMesh`、`TestRenderPatchMesh`、
  `TestTextStrokeMode`、Type3/透明/SMask 测试保持通过
