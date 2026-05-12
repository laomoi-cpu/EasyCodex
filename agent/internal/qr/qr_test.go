package qr

import (
	"strings"
	"testing"
)

func TestSVGLongPairingPayload(t *testing.T) {
	payload := "easycodex://pair?u=https%3A%2F%2F7fb07pk68535.vicp.fun%2Fapi%2Fmobile-pair%3Fcode%3Dfd44d8bc713c%26baseUrl%3Dhttps%253A%252F%252F7fb07pk68535.vicp.fun"

	svg, err := SVG(payload, 8)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, "<svg ") || !strings.Contains(svg, "<rect ") {
		t.Fatalf("unexpected svg: %s", svg)
	}
}
