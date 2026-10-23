# ADR-0009: Standard security handler R2-R6

日期：2026-10-22｜状态：已采纳

## 决策

- 支持 PDF Standard security handler revisions 2–6：
  - RC4 40/128-bit
  - AESV2（AES-128-CBC）
  - AESV3（AES-256-CBC，R5/R6）
- `WithPassword` 同时尝试用户密码和所有者密码；错误或缺失返回 `ErrPassword`。
- 加密对象流在 Flate/object-stream 解析前解密；普通对象字符串在解析后递归解密。
- XRef 流与未加密元数据流不参与解密。
- R6 password hash 按 PDF Algorithm 2.B 实现。
- V5/V6 密码执行常用 SASLprep 映射、NFKC 与禁止字符检查；完整双向文本规则尚未实现，
  失败时按 UTF-8 原样回退。

## 测试

- 使用 pypdf 生成的 RC4-40、RC4-128、AES-128、AES-256-R5、AES-256 小型 PDF 作为只读 fixtures。
- 覆盖用户密码、所有者密码、错误/缺失密码、Perms、Info 字符串和内容流。
