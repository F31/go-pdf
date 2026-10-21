package font

import "testing"

func TestPredefinedCMapsLoaded(t *testing.T) {
	if err := predefinedDataError(); err != nil {
		t.Fatal(err)
	}
	names := []string{
		"UniGB-UCS2-H", "UniGB-UCS2-V",
		"UniGB-UTF16-H", "UniGB-UTF16-V",
		"GBK-EUC-H", "GBK-EUC-V",
		"GB-EUC-H", "GB-EUC-V",
		"UniCNS-UCS2-H", "UniCNS-UCS2-V",
		"UniCNS-UTF16-H", "UniCNS-UTF16-V",
		"B5-H", "B5-V",
		"B5pc-H", "B5pc-V",
		"ETen-B5-H", "ETen-B5-V",
		"ETenms-B5-H", "ETenms-B5-V",
		"UniJIS-UCS2-H", "UniJIS-UCS2-V",
		"UniJIS-UTF16-H", "UniJIS-UTF16-V",
		"90ms-RKSJ-H", "90ms-RKSJ-V",
		"90msp-RKSJ-H", "90msp-RKSJ-V",
		"90pv-RKSJ-H", "90pv-RKSJ-V",
		"EUC-H", "EUC-V",
		"H", "V",
		"UniKS-UCS2-H", "UniKS-UCS2-V",
		"UniKS-UTF16-H", "UniKS-UTF16-V",
		"KSCms-UHC-H", "KSCms-UHC-V",
		"KSCms-UHC-HW-H", "KSCms-UHC-HW-V",
		"KSC-EUC-H", "KSC-EUC-V",
		"KSCpc-EUC-H", "KSCpc-EUC-V",
		"KSC-Johab-H", "KSC-Johab-V",
	}
	for _, name := range names {
		cm := predefinedCMapByName(name)
		if cm == nil {
			t.Fatalf("%s not loaded", name)
		}
		if len(cm.spaces) == 0 || cm.maxBytes == 0 || !cm.hasUnicode() {
			t.Fatalf("%s incomplete: spaces=%d max=%d unicode=%v", name, len(cm.spaces), cm.maxBytes, cm.hasUnicode())
		}
	}
}

func TestPredefinedGBEUC(t *testing.T) {
	cm := predefinedCMapByName("GB-EUC-H")
	if cm == nil {
		t.Fatal("GB-EUC-H not loaded")
	}
	codes, used := cm.split([]byte{0xA1, 0xA1})
	if used != 2 || len(codes) != 1 {
		t.Fatalf("used=%d codes=%#v", used, codes)
	}
	if codes[0].CID != 96 {
		t.Fatalf("cid %d", codes[0].CID)
	}
	if s, ok := cm.decode(codes[0]); !ok || s != "　" {
		t.Fatalf("decode %q %v", s, ok)
	}
}

func TestPredefinedBig5(t *testing.T) {
	cm := predefinedCMapByName("B5-H")
	if cm == nil {
		t.Fatal("B5-H not loaded")
	}
	codes, used := cm.split([]byte{0xA4, 0xA4})
	if used != 2 || len(codes) != 1 || codes[0].CID != 661 {
		t.Fatalf("used=%d codes=%#v", used, codes)
	}
	if s, ok := cm.decode(codes[0]); !ok || s != "中" {
		t.Fatalf("decode %q %v", s, ok)
	}

	ucs2 := predefinedCMapByName("UniCNS-UCS2-H")
	codes, used = ucs2.split([]byte{0x4E, 0x2D})
	if used != 2 || len(codes) != 1 || codes[0].CID != 661 {
		t.Fatalf("ucs2 used=%d codes=%#v", used, codes)
	}
	if s, ok := ucs2.decode(codes[0]); !ok || s != "中" {
		t.Fatalf("ucs2 decode %q %v", s, ok)
	}
}

func TestPredefinedJapaneseKorean(t *testing.T) {
	tests := []struct {
		cmap string
		data []byte
		cid  uint32
		text string
	}{
		{"90ms-RKSJ-H", []byte{0x82, 0xA0}, 843, "あ"},
		{"90ms-RKSJ-V", []byte{0x82, 0xA0}, 843, "あ"},
		{"90pv-RKSJ-H", []byte{0x82, 0xA0}, 843, "あ"},
		{"EUC-H", []byte{0xA4, 0xA2}, 843, "あ"},
		{"H", []byte{0x21, 0x21}, 633, "　"},
		{"UniJIS-UCS2-H", []byte{0x30, 0x42}, 843, "あ"},
		{"UniJIS-UTF16-H", []byte{0x30, 0x42}, 843, "あ"},
		{"KSCms-UHC-H", []byte{0xB0, 0xA1}, 1086, "가"},
		{"KSCms-UHC-HW-H", []byte{0xB0, 0xA1}, 1086, "가"},
		{"KSC-EUC-H", []byte{0xB0, 0xA1}, 1086, "가"},
		{"KSCpc-EUC-H", []byte{0xB0, 0xA1}, 1086, "가"},
		{"KSC-Johab-H", []byte{0x88, 0x61}, 1086, "가"},
		{"UniKS-UCS2-H", []byte{0xAC, 0x00}, 1086, "가"},
		{"UniKS-UTF16-H", []byte{0xAC, 0x00}, 1086, "가"},
	}
	for _, tt := range tests {
		t.Run(tt.cmap, func(t *testing.T) {
			cm := predefinedCMapByName(tt.cmap)
			if cm == nil {
				t.Fatalf("%s not loaded", tt.cmap)
			}
			codes, used := cm.split(tt.data)
			if used != len(tt.data) || len(codes) != 1 || codes[0].CID != tt.cid {
				t.Fatalf("used=%d codes=%#v", used, codes)
			}
			if s, ok := cm.decode(codes[0]); !ok || s != tt.text {
				t.Fatalf("decode %q %v", s, ok)
			}
		})
	}
	if cm := predefinedCMapByName("90ms-RKSJ-V"); cm == nil || cm.wmode != 1 {
		t.Fatal("90ms-RKSJ-V has no vertical writing mode")
	}
}

func FuzzPredefinedSplit(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xD6, 0xD0})
	f.Add([]byte{0x81})
	f.Add([]byte{0x81, 0x40, 0x20})
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, name := range []string{"GBK-EUC-H", "UniGB-UTF16-H", "B5-H", "UniCNS-UTF16-H", "90ms-RKSJ-H", "KSCms-UHC-H"} {
			cm := predefinedCMapByName(name)
			if cm == nil {
				t.Fatalf("%s not loaded", name)
			}
			codes, used := cm.split(data)
			if used < 0 || used > len(data) {
				t.Fatalf("%s consumed %d of %d", name, used, len(data))
			}
			total := 0
			for _, c := range codes {
				if c.Bytes <= 0 || c.Bytes > 4 {
					t.Fatalf("%s bad code length %d", name, c.Bytes)
				}
				total += c.Bytes
			}
			if total != used {
				t.Fatalf("%s lengths %d consumed %d", name, total, used)
			}
		}
	})
}
