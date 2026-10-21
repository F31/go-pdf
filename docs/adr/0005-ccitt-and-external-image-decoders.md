# ADR-0005: CCITT and external image decoders

日期：2026-10-21｜状态：已采纳

## 背景

设计 V1.4 的 ADR-018 将 CCITT G3/G4 列为自研纯 Go 解码，并为 JBIG2/JPX
定义外部解码器接口。CCITT 的 run-length 与二维模式表较大，重复实现会扩大
fuzz 面；`golang.org/x/image/ccitt` 已在允许依赖 `golang.org/x/image` 内，
且为纯 Go 实现。

## 决策

- CCITT Group 4 与 Group 3 1-D（含 EOL）使用 `x/image/ccitt` 解码。
- `K > 0` 的 Group 3 2-D 与无 EOL 的 G3 变体暂不支持，返回
  `ErrUnsupported`/诊断，不猜测。
- JBIG2/JPX 由调用方注册 `pdf.ExternalDecoder`；核心在无解码器时返回
  `ErrUnsupported` 并写 `IMG-NO-DECODER`，禁止输出缺图页面。
- 外部解码器按注册顺序查询；注册应在并发页面解码开始前完成。

## 影响

- 减少自研解码器和对应表数据的维护成本；CCITT 语料可直接验证。
- 设计 V1.4 中“CCITT 自研”的表述由本 ADR 调整；G3 2-D 的限制记录在
  `docs/g3-status.md`，后续按语料需要补齐。
- 不新增模块依赖；`x/image` 已在 `THIRD_PARTY.md` 登记。
