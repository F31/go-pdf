# ADR-0007: Public FontProvider fallback SPI

日期：2026-10-21｜状态：已采纳

## 背景

区域渲染原本只使用嵌入字体的 `displaylist.GlyphSource`。Base-14、替代字体或无轮廓
字体无法渲染文字，而 V1.6 要求字体替代通过公共扩展接口注入，不能依赖 internal/font。

## 决策

- `render.PageOptions` 增加 `Fonts render.FontProvider`。
- Provider 接收公共 `*displaylist.FontInfo` 与 `displaylist.Glyph`，返回 1/1000
  text-space units 的轮廓。
- 嵌入 `GlyphSource` 优先；无轮廓或轮廓不可用时回退到 Provider。
- Provider 的错误传播到 RenderRegion，不静默吞掉。

## 影响

- 调用方可注入 base-14、系统字体或自定义替代字体，不需要 internal 包。
- 轮廓坐标系与 `FontInfo.GlyphSource` 一致，由 `Glyph.Matrix` 缩放。
- 后续 FontProvider 扩展（字形覆盖、度量匹配、缺字诊断）在同一接口上演进。
