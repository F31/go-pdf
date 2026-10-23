# ADR-0011: public ColorManager SPI

日期：2026-10-22｜状态：Accepted

## 背景

内置 ICC matrix/TRC 只覆盖 RGB/Gray；CMYK、CLUT profile 和自定义色彩策略需要
调用方接入，而公共 API 不应暴露 internal/color。

## 决策

- 新增公共 `pdf.ColorManager`：
  ```go
  Convert(ColorSpaceInfo, []float64) ([4]float64, bool, error)
  ```
- `ColorSpaceInfo` 提供 `Name`、`Components` 和可选 `ICCProfile` 原始字节。
- 通过 `pdf.WithColorManager` 绑定到 Engine/Document。
- 当前注入点：DeviceCMYK 与 ICCBased；`k`、`scn`、Shading、图案基色共享
  同一路径。
- 管理器返回错误时写诊断并停止该颜色操作；不静默生成伪颜色。
- 未注入管理器时继续使用内置转换：DeviceCMYK 固定近似、ICC matrix/TRC。

## 后果

- 外部可用纯 Go 或适配器实现 CMYK/CLUT/ICC 转换，核心依赖不变。
- `Space.Approx()` 对托管空间保守返回 true；`RGBA` 的 approx 结果仍由
  管理器返回。
- 原生 ICC CLUT/CMYK 仍待实现，不能据此宣称 Native Complete。
