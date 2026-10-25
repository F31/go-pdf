// Package legacy adapts the original pdf.DecoderFactory, pdf.ColorManager and
// render.FontProvider interfaces to the extension ports.
//
// The adapters exist for migration and compatibility. Prefer native extension
// providers when source samples, metrics or provider-specific capabilities are
// required. In particular, the image adapter converts the legacy image.Image
// result to an 8-bit sRGB RGBA plane and therefore cannot preserve an ICCBased
// or CMYK source plane. The colour adapter only targets sRGB. The font adapter
// exposes outlines only; it does not recover metrics or GIDs.
package legacy
