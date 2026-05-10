package qr

import (
	"errors"
	"fmt"
	"html"
	"strings"
)

const version = 5
const size = 4*version + 17
const dataCodewords = 108
const eccCodewords = 26

func SVG(text string, scale int) (string, error) {
	if scale <= 0 {
		scale = 8
	}
	data := []byte(text)
	if len(data) > 106 {
		return "", errors.New("QR payload is too long for version 5-L")
	}
	bits := encodeBits(data)
	codewords := bitsToCodewords(bits)
	parity := reedSolomon(codewords, eccCodewords)
	all := append(codewords, parity...)
	matrix := newMatrix()
	drawFunctionPatterns(matrix)
	placeData(matrix, codewordsToBits(all), 0)
	drawFormatBits(matrix, 0)
	return matrixSVG(matrix, scale, text), nil
}

func encodeBits(data []byte) []bool {
	bits := []bool{}
	appendBits := func(value, count int) {
		for i := count - 1; i >= 0; i-- {
			bits = append(bits, ((value>>i)&1) != 0)
		}
	}
	appendBits(0x4, 4)
	appendBits(len(data), 8)
	for _, b := range data {
		appendBits(int(b), 8)
	}
	remaining := dataCodewords*8 - len(bits)
	terminator := 4
	if remaining < terminator {
		terminator = remaining
	}
	appendBits(0, terminator)
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	pad := []int{0xec, 0x11}
	for i := 0; len(bits) < dataCodewords*8; i++ {
		appendBits(pad[i%2], 8)
	}
	return bits
}

func bitsToCodewords(bits []bool) []byte {
	out := make([]byte, len(bits)/8)
	for i, bit := range bits {
		if bit {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}

func codewordsToBits(codewords []byte) []bool {
	bits := make([]bool, 0, len(codewords)*8)
	for _, b := range codewords {
		for i := 7; i >= 0; i-- {
			bits = append(bits, ((b>>uint(i))&1) != 0)
		}
	}
	return bits
}

func reedSolomon(data []byte, degree int) []byte {
	gen := rsGenerator(degree)
	rem := make([]byte, degree)
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[degree-1] = 0
		for i := 0; i < degree; i++ {
			rem[i] ^= gfMul(gen[i], factor)
		}
	}
	return rem
}

func rsGenerator(degree int) []byte {
	poly := []byte{1}
	for i := 0; i < degree; i++ {
		next := make([]byte, len(poly)+1)
		for j, coeff := range poly {
			next[j] ^= gfMul(coeff, 1)
			next[j+1] ^= gfMul(coeff, gfPow(2, i))
		}
		poly = next
	}
	return poly[1:]
}

func gfPow(x byte, power int) byte {
	result := byte(1)
	for i := 0; i < power; i++ {
		result = gfMul(result, x)
	}
	return result
}

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11d)
		if ((int(y) >> uint(i)) & 1) != 0 {
			z ^= int(x)
		}
	}
	return byte(z)
}

func newMatrix() [][]int {
	m := make([][]int, size)
	for y := range m {
		m[y] = make([]int, size)
		for x := range m[y] {
			m[y][x] = -1
		}
	}
	return m
}

func drawFunctionPatterns(m [][]int) {
	drawFinder(m, 0, 0)
	drawFinder(m, size-7, 0)
	drawFinder(m, 0, size-7)
	for i := 8; i < size-8; i++ {
		set(m, i, 6, boolInt(i%2 == 0))
		set(m, 6, i, boolInt(i%2 == 0))
	}
	drawAlignment(m, 30, 30)
	reserveFormat(m)
	set(m, 8, 4*version+9, 1)
}

func drawFinder(m [][]int, left, top int) {
	for dy := -1; dy <= 7; dy++ {
		for dx := -1; dx <= 7; dx++ {
			x, y := left+dx, top+dy
			if x < 0 || y < 0 || x >= size || y >= size {
				continue
			}
			black := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 && (dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4))
			set(m, x, y, boolInt(black))
		}
	}
}

func drawAlignment(m [][]int, cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			d := max(abs(dx), abs(dy))
			set(m, cx+dx, cy+dy, boolInt(d != 1))
		}
	}
}

func reserveFormat(m [][]int) {
	for i := 0; i <= 8; i++ {
		if i != 6 {
			set(m, 8, i, 0)
			set(m, i, 8, 0)
		}
	}
	for i := 0; i < 8; i++ {
		set(m, size-1-i, 8, 0)
		set(m, 8, size-1-i, 0)
	}
}

func placeData(m [][]int, bits []bool, mask int) {
	bitIndex := 0
	up := true
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right--
		}
		for vert := 0; vert < size; vert++ {
			y := vert
			if up {
				y = size - 1 - vert
			}
			for dx := 0; dx < 2; dx++ {
				x := right - dx
				if m[y][x] != -1 {
					continue
				}
				bit := false
				if bitIndex < len(bits) {
					bit = bits[bitIndex]
					bitIndex++
				}
				if maskBit(mask, x, y) {
					bit = !bit
				}
				set(m, x, y, boolInt(bit))
			}
		}
		up = !up
	}
}

func maskBit(mask, x, y int) bool {
	return mask == 0 && (x+y)%2 == 0
}

func drawFormatBits(m [][]int, mask int) {
	bits := formatBits(mask)
	for i := 0; i <= 5; i++ {
		set(m, 8, i, (bits>>uint(i))&1)
	}
	set(m, 8, 7, (bits>>6)&1)
	set(m, 8, 8, (bits>>7)&1)
	set(m, 7, 8, (bits>>8)&1)
	for i := 9; i < 15; i++ {
		set(m, 14-i, 8, (bits>>uint(i))&1)
	}
	for i := 0; i < 8; i++ {
		set(m, size-1-i, 8, (bits>>uint(i))&1)
	}
	for i := 8; i < 15; i++ {
		set(m, 8, size-15+i, (bits>>uint(i))&1)
	}
	set(m, 8, size-8, 1)
}

func formatBits(mask int) int {
	data := (1 << 3) | mask
	rem := data << 10
	for bit := 14; bit >= 10; bit-- {
		if ((rem >> uint(bit)) & 1) != 0 {
			rem ^= 0x537 << uint(bit-10)
		}
	}
	return ((data << 10) | (rem & 0x3ff)) ^ 0x5412
}

func matrixSVG(m [][]int, scale int, title string) string {
	quiet := 4
	full := (size + quiet*2) * scale
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img"><title>%s</title><rect width="100%%" height="100%%" fill="#fff"/>`, full, full, full, full, html.EscapeString(title)))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if m[y][x] == 1 {
				b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#000"/>`, (x+quiet)*scale, (y+quiet)*scale, scale, scale))
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func set(m [][]int, x, y, value int) {
	if x >= 0 && y >= 0 && x < size && y < size {
		m[y][x] = value
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
