# P3 Extended Provider 基线

日期：2026-10-25
用途：记录 Extended provider 的元数据、进程隔离、codec/color/font 边界和 Core
独立性。Core/Preview 指标另见 P1/P2 基线。

## Provider Registry

- `Registry` 现在校验 feature：名称非空、status 属于声明词汇；approximate、
  external、unsupported、unknown、not-scanned 必须带 reason。
- `Registry.Features()` 返回按 provider/feature 排序的副本，注册顺序不影响结果。
- 同一 encoding/名字/版本的 provider 冲突继续拒绝；metadata 冲突返回错误。
- 新增 `TestRegistryFeaturesAreSortedAndImmutable`、
  `TestRegistryRejectsInvalidFeatures`、`TestRegistrySupportsG32DEncoding`。

## 进程 adapter 加固

新增 `internal/procutil`：

- stdout/stderr 合并上限 64 KiB，超限加 `... (truncated)`。
- `Run` 将 context 取消/超时映射为 context error。
- 输出文件大小和 PNG 像素数有限制。
- 临时目录仍在 defer 中删除。

应用到：

- `extension/openjpeg`：32/64 MiB 输出检查、PNG 像素上限、bounded stderr。
- `extension/poppler`：bounded 错误、PNG 文件/像素上限、`MaxPixels`/
  `MaxOutputBytes` 可覆盖；新增 `ProviderInfo`/`Features`。
- `extension/cairo`：bounded 错误、SVG 文件上限、`MaxOutputBytes`；
  新增 `ProviderInfo`/`Features`。

## Codec 边界

- `extension/openjpeg`：JPX 外部进程 provider；fake e2e 覆盖命令构造、版本、
  像素转换、超时和错误截断；新增 `TestRealOpenJPEGRoundTrip`，在安装
  `opj_compress`/`opj_decompress` 时生成 JP2 并真实解码；CI 无工具时跳过。
- `extension/jbig2`：新增可选 `jbig2dec` 进程 provider，输出 PNG 转
  `ImagePlane`；fake e2e 覆盖 Gray 平面。真实二进制仍需部署方安装。
- `CCITTG32D`：Registry 已支持该 encoding；核心没有内置 G3 2-D 解码，
  无 provider 时返回 Unsupported/NotScanned，按 Extended 边界处理。
- Core 不导入 `extension/poppler|cairo|openjpeg|jbig2|legacy`；
  `scripts/check-core-deps.sh` 和 CI 检查该约束。

## Color/Font

- `ColorEngine` 已有 `NewColorManagerFromEngine` 适配与 transform/cache/close
  contract；Core 不链接 lcms2。
- `FontEngine` 已有 render provider 适配与 embedded/fallback 路径；
  shaping 明确不在当前范围。
- 本阶段不捆绑 lcms2/FreeType 二进制；部署方按显式 provider 接入。

## 验收

```bash
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
./scripts/check-core-deps.sh
```

本地结果：

- `go test ./...`、`CGO_ENABLED=0 go test ./...`、`go test -race ./...`、
  `go vet ./...`、`staticcheck ./...`：通过。
- `./scripts/check-core-deps.sh`：通过。
- 本机安装 OpenJPEG 2.5 后，真实 JP2 round-trip 通过；CI 无工具时该测试跳过。
- `jbig2dec` fake e2e 通过；真实 JBIG2 样本仍需部署方提供。

P3 远端 CI：https://github.com/F31/go-pdf/actions/runs/40172626554（10/10 success）。

## P3 验收

- Registry feature 校验、排序和副本语义：通过。
- 进程 adapter 超时、输出截断、文件/像素上限、临时目录清理：通过。
- OpenJPEG fake e2e 与可选真实 JP2 round-trip：通过。
- JBIG2 jbig2dec provider 与 G3 2-D Registry 契约：通过。
- ColorEngine/FontEngine 可选路径：已有 contract 测试。
- Poppler/Cairo 版本/许可/feature 与失败隔离：通过。
- Core 不导入具体 provider，CI 检查：通过。
- Linux/macOS/Windows、race、vet、staticcheck、fuzz、交叉编译：通过。

P3 不改变 Core/Preview 默认行为，也不新增 Core 第三方依赖。
