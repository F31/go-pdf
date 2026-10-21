#!/usr/bin/env python3
"""Generate internal/font/data/cmap.bin from Adobe CMap resources.

The source files are distributed with poppler-data. The embedded set covers
the Adobe-GB1 (simplified Chinese), Adobe-CNS1 (Big5), Adobe-Japan1, and
Adobe-Korea1 collections. The generated binary keeps code spaces, code-to-CID
ranges, notdef ranges, and (for legacy encodings) code-to-Unicode ranges. Run
this script from the repository root:

    POPPLER_CMAP_DIR=/usr/share/poppler/cMap scripts/gen_cmaps.py

The output is deterministic. Do not edit it by hand.
"""
import argparse
import io
import os
import re
import struct
import sys

# name -> (collection, CMap file, Unicode strategy, Unicode source file)
# Unicode strategies: direct UCS-2, direct UTF-16BE, a code-keyed bfrange CMap,
# or a CID-keyed bfrange CMap composed with the code-to-CID mapping.
SPECS = {
    "UniGB-UCS2-H": ("Adobe-GB1", "UniGB-UCS2-H", "ucs2", None),
    "UniGB-UCS2-V": ("Adobe-GB1", "UniGB-UCS2-V", "ucs2", None),
    "UniGB-UTF16-H": ("Adobe-GB1", "UniGB-UTF16-H", "utf16", None),
    "UniGB-UTF16-V": ("Adobe-GB1", "UniGB-UTF16-V", "utf16", None),
    "GBK-EUC-H": ("Adobe-GB1", "GBK-EUC-H", "table", "GBK-EUC-UCS2"),
    "GBK-EUC-V": ("Adobe-GB1", "GBK-EUC-V", "table", "GBK-EUC-UCS2"),
    "GB-EUC-H": ("Adobe-GB1", "GB-EUC-H", "table", "GBK-EUC-UCS2"),
    "GB-EUC-V": ("Adobe-GB1", "GB-EUC-V", "table", "GBK-EUC-UCS2"),

    "UniCNS-UCS2-H": ("Adobe-CNS1", "UniCNS-UCS2-H", "ucs2", None),
    "UniCNS-UCS2-V": ("Adobe-CNS1", "UniCNS-UCS2-V", "ucs2", None),
    "UniCNS-UTF16-H": ("Adobe-CNS1", "UniCNS-UTF16-H", "utf16", None),
    "UniCNS-UTF16-V": ("Adobe-CNS1", "UniCNS-UTF16-V", "utf16", None),
    "B5-H": ("Adobe-CNS1", "B5-H", "table", "B5pc-UCS2"),
    "B5-V": ("Adobe-CNS1", "B5-V", "table", "B5pc-UCS2"),
    "B5pc-H": ("Adobe-CNS1", "B5pc-H", "table", "B5pc-UCS2"),
    "B5pc-V": ("Adobe-CNS1", "B5pc-V", "table", "B5pc-UCS2"),
    "ETen-B5-H": ("Adobe-CNS1", "ETen-B5-H", "table", "ETen-B5-UCS2"),
    "ETen-B5-V": ("Adobe-CNS1", "ETen-B5-V", "table", "ETen-B5-UCS2"),
    "ETenms-B5-H": ("Adobe-CNS1", "ETenms-B5-H", "table", "ETen-B5-UCS2"),
    "ETenms-B5-V": ("Adobe-CNS1", "ETenms-B5-V", "table", "ETen-B5-UCS2"),

    "UniJIS-UCS2-H": ("Adobe-Japan1", "UniJIS-UCS2-H", "ucs2", None),
    "UniJIS-UCS2-V": ("Adobe-Japan1", "UniJIS-UCS2-V", "ucs2", None),
    "UniJIS-UTF16-H": ("Adobe-Japan1", "UniJIS-UTF16-H", "utf16", None),
    "UniJIS-UTF16-V": ("Adobe-Japan1", "UniJIS-UTF16-V", "utf16", None),
    "90ms-RKSJ-H": ("Adobe-Japan1", "90ms-RKSJ-H", "table", "90ms-RKSJ-UCS2"),
    "90ms-RKSJ-V": ("Adobe-Japan1", "90ms-RKSJ-V", "table", "90ms-RKSJ-UCS2"),
    "90msp-RKSJ-H": ("Adobe-Japan1", "90msp-RKSJ-H", "table", "90ms-RKSJ-UCS2"),
    "90msp-RKSJ-V": ("Adobe-Japan1", "90msp-RKSJ-V", "table", "90ms-RKSJ-UCS2"),
    "90pv-RKSJ-H": ("Adobe-Japan1", "90pv-RKSJ-H", "table", "90pv-RKSJ-UCS2"),
    "90pv-RKSJ-V": ("Adobe-Japan1", "90pv-RKSJ-V", "table", "90pv-RKSJ-UCS2"),
    "EUC-H": ("Adobe-Japan1", "EUC-H", "cidtable", "Adobe-Japan1-UCS2"),
    "EUC-V": ("Adobe-Japan1", "EUC-V", "cidtable", "Adobe-Japan1-UCS2"),
    "H": ("Adobe-Japan1", "H", "cidtable", "Adobe-Japan1-UCS2"),
    "V": ("Adobe-Japan1", "V", "cidtable", "Adobe-Japan1-UCS2"),

    "UniKS-UCS2-H": ("Adobe-Korea1", "UniKS-UCS2-H", "ucs2", None),
    "UniKS-UCS2-V": ("Adobe-Korea1", "UniKS-UCS2-V", "ucs2", None),
    "UniKS-UTF16-H": ("Adobe-Korea1", "UniKS-UTF16-H", "utf16", None),
    "UniKS-UTF16-V": ("Adobe-Korea1", "UniKS-UTF16-V", "utf16", None),
    "KSCms-UHC-H": ("Adobe-Korea1", "KSCms-UHC-H", "table", "KSCms-UHC-UCS2"),
    "KSCms-UHC-V": ("Adobe-Korea1", "KSCms-UHC-V", "table", "KSCms-UHC-UCS2"),
    "KSCms-UHC-HW-H": ("Adobe-Korea1", "KSCms-UHC-HW-H", "table", "KSCms-UHC-UCS2"),
    "KSCms-UHC-HW-V": ("Adobe-Korea1", "KSCms-UHC-HW-V", "table", "KSCms-UHC-UCS2"),
    "KSC-EUC-H": ("Adobe-Korea1", "KSC-EUC-H", "cidtable", "Adobe-Korea1-UCS2"),
    "KSC-EUC-V": ("Adobe-Korea1", "KSC-EUC-V", "cidtable", "Adobe-Korea1-UCS2"),
    "KSCpc-EUC-H": ("Adobe-Korea1", "KSCpc-EUC-H", "cidtable", "Adobe-Korea1-UCS2"),
    "KSCpc-EUC-V": ("Adobe-Korea1", "KSCpc-EUC-V", "cidtable", "Adobe-Korea1-UCS2"),
    "KSC-Johab-H": ("Adobe-Korea1", "KSC-Johab-H", "cidtable", "Adobe-Korea1-UCS2"),
    "KSC-Johab-V": ("Adobe-Korea1", "KSC-Johab-V", "cidtable", "Adobe-Korea1-UCS2"),
}

HEX = r"<([0-9A-Fa-f]+)>"
NUM = r"(\d+)"


def block(text, name):
    return re.findall(r"\bbegin" + name + r"\b(.*?)\bend" + name + r"\b", text, re.S)


def parse_pairs(text):
    out = []
    for m in re.finditer(HEX + r"\s*" + HEX, text):
        lo, hi = m.groups()
        out.append((len(lo) // 2, int(lo, 16), int(hi, 16)))
    return out


def parse_cidranges(text):
    out = {}
    for part in block(text, "cidrange"):
        for m in re.finditer(HEX + r"\s*" + HEX + r"\s*" + NUM, part):
            lo, hi, cid = m.groups()
            n, lo, hi, cid = len(lo) // 2, int(lo, 16), int(hi, 16), int(cid)
            if hi < lo or hi - lo > 1 << 20:
                continue
            for code in range(lo, hi + 1):
                out[(n, code)] = cid + code - lo
    for part in block(text, "cidchar"):
        for m in re.finditer(HEX + r"\s*" + NUM, part):
            src, cid = m.groups()
            out[(len(src) // 2, int(src, 16))] = int(cid)
    return out


def parse_notdef(text):
    out = {}
    for part in block(text, "notdefrange"):
        for m in re.finditer(HEX + r"\s*" + HEX + r"\s*" + NUM, part):
            lo, hi, cid = m.groups()
            n, lo, hi, cid = len(lo) // 2, int(lo, 16), int(hi, 16), int(cid)
            if hi < lo or hi - lo > 1 << 20:
                continue
            for code in range(lo, hi + 1):
                out[(n, code)] = cid + code - lo
    return out


SKIPPED = {"multi": 0, "range": 0}


def is_variation_selector(r):
    return 0xFE00 <= r <= 0xFE0F or 0xE0100 <= r <= 0xE01EF


def decode_dst(hex_string):
    """Return the Unicode text for a destination, or None if unsupported.

    A base character followed only by variation selectors is reduced to the
    base character. Other multi-character destinations cannot be represented
    by the one-rune mapping table and are skipped.
    """
    data = bytes.fromhex(hex_string)
    if len(data) % 2:
        raise ValueError("odd UTF-16BE destination")
    text = data.decode("utf-16-be")
    if len(text) == 1:
        return text
    if text and all(is_variation_selector(ord(c)) for c in text[1:]):
        return text[0]
    SKIPPED["multi"] += 1
    return None


def parse_unicode(text):
    out = {}
    for part in block(text, "bfrange"):
        for m in re.finditer(HEX + r"\s*" + HEX + r"\s*" + HEX, part):
            lo_hex, hi_hex, dst = m.groups()
            lo, hi = int(lo_hex, 16), int(hi_hex, 16)
            dst_text = decode_dst(dst)
            if dst_text is None:
                continue
            n, base = len(lo_hex) // 2, ord(dst_text[0])
            # A multi-character destination cannot be incremented safely.
            if len(dst_text) > 1 and hi != lo:
                SKIPPED["range"] += 1
                continue
            if hi < lo or hi - lo > 1 << 20:
                continue
            for code in range(lo, hi + 1):
                out[(n, code)] = base + code - lo
    for part in block(text, "bfchar"):
        for m in re.finditer(HEX + r"\s*" + HEX, part):
            src, dst = m.groups()
            dst_text = decode_dst(dst)
            if dst_text is not None:
                out[(len(src) // 2, int(src, 16))] = ord(dst_text[0])
    return out


def parse_file(path):
    with open(path, "r", encoding="latin1") as f:
        text = f.read()
    name = re.search(r"/CMapName\s+/([^\s]+)\s+def", text)
    uses = re.search(r"/([A-Za-z0-9_.-]+)\s+usecmap", text)
    wmode = re.search(r"/WMode\s+(\d+)\s+def", text)
    spaces = []
    for part in block(text, "codespacerange"):
        spaces.extend(parse_pairs(part))
    return {
        "name": name.group(1) if name else os.path.basename(path),
        "uses": uses.group(1) if uses else None,
        "wmode": int(wmode.group(1)) if wmode else 0,
        "spaces": spaces,
        "cid": parse_cidranges(text),
        "notdef": parse_notdef(text),
        "unicode": parse_unicode(text),
    }


def resolve(source_dir, collection, filename, cache):
    key = (collection, filename)
    if key in cache:
        return cache[key]
    path = os.path.join(source_dir, collection, filename)
    if not os.path.exists(path):
        raise FileNotFoundError(path)
    own = parse_file(path)
    if own["uses"]:
        base = resolve(source_dir, collection, own["uses"], cache)
        merged = {
            "name": own["name"],
            "uses": own["uses"],
            "wmode": own["wmode"],
            "spaces": base["spaces"],
            "cid": dict(base["cid"]),
            "notdef": dict(base["notdef"]),
            "unicode": dict(base["unicode"]),
        }
        merged["cid"].update(own["cid"])
        merged["notdef"].update(own["notdef"])
        merged["unicode"].update(own["unicode"])
    else:
        merged = own
    cache[key] = merged
    return merged


def compress(mapping):
    """Convert {(n, code): value} to disjoint (n, lo, hi, value) ranges."""
    items = sorted(mapping.items())
    out = []
    for (n, code), value in items:
        if out and out[-1][0] == n and out[-1][2] + 1 == code and out[-1][3] + (code - out[-1][1]) == value:
            _, lo, _, base = out[-1]
            out[-1] = (n, lo, code, base)
        else:
            out.append((n, code, code, value))
    return out


def u8(v):
    if not 0 <= v <= 0xFF:
        raise ValueError("u8 out of range")
    return struct.pack(">B", v)


def u16(v):
    if not 0 <= v <= 0xFFFF:
        raise ValueError("u16 out of range")
    return struct.pack(">H", v)


def u32(v):
    if not 0 <= v <= 0xFFFFFFFF:
        raise ValueError("u32 out of range")
    return struct.pack(">I", v)


def write_spaces(out, spaces):
    # Deduplicate while preserving order.
    seen, uniq = set(), []
    for n, lo, hi in spaces:
        if (n, lo, hi) not in seen:
            seen.add((n, lo, hi))
            uniq.append((n, lo, hi))
    out.write(u16(len(uniq)))
    for n, lo, hi in uniq:
        out.write(u8(n) + u32(lo) + u32(hi))


def write_ranges(out, ranges, unicode_values):
    out.write(u32(len(ranges)))
    for n, lo, hi, value in ranges:
        if unicode_values:
            out.write(u8(n) + u32(lo) + u32(hi) + u32(value))
        else:
            out.write(u8(n) + u32(lo) + u32(hi) + u32(value))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--source",
        default=os.environ.get("POPPLER_CMAP_DIR", "/usr/share/poppler/cMap"),
        help="directory containing Adobe CMap collection subdirectories",
    )
    parser.add_argument(
        "--output",
        default=os.path.join("internal", "font", "data", "cmap.bin"),
        help="output binary path",
    )
    args = parser.parse_args()

    cache = {}
    unicode_cache = {}
    entries = []
    for name, (collection, filename, kind, unicode_file) in SPECS.items():
        cmap = resolve(args.source, collection, filename, cache)
        if not cmap["spaces"]:
            raise ValueError(f"{filename}: no code space")
        if kind in ("table", "cidtable"):
            ukey = (collection, unicode_file)
            if ukey not in unicode_cache:
                unicode_cache[ukey] = resolve(args.source, collection, unicode_file, cache)["unicode"]
            source_map = unicode_cache[ukey]
            if not source_map:
                raise ValueError(f"{unicode_file}: no Unicode mappings")
            unicode_map = {}
            if kind == "table":
                unicode_map = dict(source_map)
            else:
                for (n, code), cid in cmap["cid"].items():
                    u = source_map.get((2, cid))
                    if u is not None:
                        unicode_map[(n, code)] = u
            if not unicode_map:
                raise ValueError(f"{unicode_file}: no Unicode mappings")
            kind_id = 3
        else:
            unicode_map = {}
            kind_id = 1 if kind == "ucs2" else 2

        entries.append(
            {
                "name": name,
                "wmode": cmap["wmode"],
                "kind": kind_id,
                "spaces": cmap["spaces"],
                "cid": compress(cmap["cid"]),
                "notdef": compress(cmap["notdef"]),
                "unicode": compress(unicode_map),
            }
        )

    data = io.BytesIO()
    data.write(b"GPDFCMAP")
    data.write(u16(1))
    data.write(u16(len(entries)))
    for e in entries:
        name = e["name"].encode("ascii")
        if len(name) > 255:
            raise ValueError("CMap name too long")
        data.write(u8(len(name)) + name)
        data.write(u8(e["wmode"]) + u8(e["kind"]))
        write_spaces(data, e["spaces"])
        write_ranges(data, e["cid"], False)
        write_ranges(data, e["notdef"], False)
        write_ranges(data, e["unicode"], True)

    os.makedirs(os.path.dirname(args.output), exist_ok=True)
    with open(args.output, "wb") as f:
        f.write(data.getvalue())
    print(f"wrote {args.output} ({len(data.getvalue())} bytes, {len(entries)} CMaps)", file=sys.stderr)
    if SKIPPED["multi"] or SKIPPED["range"]:
        print(
            f"skipped {SKIPPED['multi']} multi-character and {SKIPPED['range']} "
            "multi-character range destinations",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
