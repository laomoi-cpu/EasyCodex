package qr

import (
	"fmt"
	"html"
	"strings"

	goqrcode "github.com/skip2/go-qrcode"
)

func SVG(text string, scale int) (string, error) {
	if scale <= 0 {
		scale = 8
	}
	code, err := goqrcode.New(text, goqrcode.Low)
	if err != nil {
		return "", err
	}
	return matrixSVG(code.Bitmap(), scale, text), nil
}

func matrixSVG(matrix [][]bool, scale int, title string) string {
	size := len(matrix)
	full := size * scale
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img"><title>%s</title><rect width="100%%" height="100%%" fill="#fff"/>`, full, full, full, full, html.EscapeString(title)))
	for y, row := range matrix {
		for x, black := range row {
			if black {
				b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#000"/>`, x*scale, y*scale, scale, scale))
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}
