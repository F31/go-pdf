package pdf

import "testing"

func TestCapabilitiesProfiles(t *testing.T) {
	core, err := CapabilitiesFor(ProfileCore)
	if err != nil {
		t.Fatal(err)
	}
	if len(core) == 0 {
		t.Fatal("core capabilities empty")
	}
	for _, c := range core {
		switch c.Area {
		case "render", "color":
			t.Fatalf("core profile contains %s/%s", c.Area, c.Feature)
		}
	}

	preview, err := CapabilitiesFor(ProfilePreview)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != len(Capabilities()) {
		t.Fatalf("preview=%d capabilities=%d", len(preview), len(Capabilities()))
	}
	if !hasArea(preview, "render") {
		t.Fatal("preview has no render capabilities")
	}

	extended, err := CapabilitiesFor(ProfileExtended)
	if err != nil {
		t.Fatal(err)
	}
	if len(extended) != len(preview) {
		t.Fatalf("extended=%d preview=%d", len(extended), len(preview))
	}

	if _, err := CapabilitiesFor(Profile("bogus")); err == nil {
		t.Fatal("expected invalid profile error")
	}
	if _, err := ParseProfile("Extended"); err == nil {
		t.Fatal("expected case-sensitive profile error")
	}
}

func hasArea(caps []Capability, area string) bool {
	for _, c := range caps {
		if c.Area == area {
			return true
		}
	}
	return false
}
