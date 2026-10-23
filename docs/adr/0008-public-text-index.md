# ADR-0008: Public text index over the display list

日期：2026-10-21｜状态：已采纳

## 背景

显示列表已提供字形位置与 ActualText，但调用方仍需要自行实现空格恢复、词/行聚类和
阅读顺序。V1.6 将文本索引列为独立公共能力，且不应依赖 render 或 PPTX 类型。

## 决策

- 新增公共 `text` 包，输入 `*displaylist.Page`，输出字符、词、行与整页文本。
- `Page.Text(ctx)` 作为便捷入口；高级调用方可直接使用 `text.Extract` 与 `text.Options`。
- 首版采用基线/Y 坐标聚类、字距空格阈值和从左到右排序；ActualText 作为独立字符项输出。
- 包不依赖 `pdf` 根包实现，也不依赖 render。

## 影响

- 下游不再重复实现基础文字切分；后续 TextSpan、Tagged 阅读顺序和双向/竖排可在同一
  包内演进。
- 当前实现支持基础水平基线与竖排列聚类，不宣称完整的多栏/双向/复杂竖排阅读顺序。
