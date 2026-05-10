package netinfo

import "testing"

func TestInspectLoopback(t *testing.T) {
	info := Inspect("127.0.0.1:8765")
	if info.Port != 8765 || info.LocalURL != "http://127.0.0.1:8765" {
		t.Fatalf("unexpected info: %#v", info)
	}
	if info.LANEnabled {
		t.Fatalf("loopback should not enable LAN: %#v", info)
	}
}

func TestInspectWildcard(t *testing.T) {
	info := Inspect("0.0.0.0:8765")
	if info.Port != 8765 || info.LocalURL != "http://127.0.0.1:8765" {
		t.Fatalf("unexpected info: %#v", info)
	}
	if !info.LANEnabled {
		t.Fatalf("wildcard should enable LAN: %#v", info)
	}
}
