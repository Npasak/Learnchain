package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// QR Code Version 1..5 parameters for ECC Level L (all 1 block)
type qrVersionSpec struct {
	version      int
	size         int
	dataBytes    int
	ecBytes      int
	alignCenters []int
}

var qrSpecs = []qrVersionSpec{
	{version: 1, size: 21, dataBytes: 19, ecBytes: 7, alignCenters: nil},
	{version: 2, size: 25, dataBytes: 34, ecBytes: 10, alignCenters: []int{6, 18}},
	{version: 3, size: 29, dataBytes: 55, ecBytes: 15, alignCenters: []int{6, 22}},
	{version: 4, size: 33, dataBytes: 80, ecBytes: 20, alignCenters: []int{6, 26}},
	{version: 5, size: 37, dataBytes: 108, ecBytes: 26, alignCenters: []int{6, 30}},
}

// BuildQRMatrix builds a scannable QR code boolean matrix and returns (modules, size)
func BuildQRMatrix(text string) ([][]bool, int) {
	raw := []byte(text)
	spec := qrSpecs[len(qrSpecs)-1]
	for _, s := range qrSpecs {
		if len(raw)+2 <= s.dataBytes {
			spec = s
			break
		}
	}
	if len(raw)+2 > spec.dataBytes {
		raw = raw[:spec.dataBytes-2]
	}

	// 1. Build bit stream: Mode 0100 (Byte), 8-bit char count, data bytes, terminator, pad bytes
	bits := make([]int, 0, spec.dataBytes*8)
	appendBits := func(val, length int) {
		for i := length - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1)
		}
	}
	appendBits(0b0100, 4)
	appendBits(len(raw), 8)
	for _, b := range raw {
		appendBits(int(b), 8)
	}
	for i := 0; i < 4 && len(bits) < spec.dataBytes*8; i++ {
		bits = append(bits, 0)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, 0)
	}

	dataCodewords := make([]byte, len(bits)/8)
	for i := range dataCodewords {
		var v byte
		for b := 0; b < 8; b++ {
			v = (v << 1) | byte(bits[i*8+b])
		}
		dataCodewords[i] = v
	}
	padBytes := []byte{0xEC, 0x11}
	for i := 0; len(dataCodewords) < spec.dataBytes; i++ {
		dataCodewords = append(dataCodewords, padBytes[i%2])
	}

	// 2. Compute Reed-Solomon EC codewords over GF(256) with primitive polynomial 0x11D
	ecCodewords := rsComputeEC(dataCodewords, spec.ecBytes)
	allCodewords := append(dataCodewords, ecCodewords...)

	// 3. Initialize matrix
	n := spec.size
	modules := make([][]bool, n)
	isFunction := make([][]bool, n)
	for i := range modules {
		modules[i] = make([]bool, n)
		isFunction[i] = make([]bool, n)
	}

	setFunc := func(r, c int, dark bool) {
		if r >= 0 && r < n && c >= 0 && c < n {
			modules[r][c] = dark
			isFunction[r][c] = true
		}
	}

	// Finder patterns + separators
	drawFinder := func(row, col int) {
		for dr := -1; dr <= 7; dr++ {
			for dc := -1; dc <= 7; dc++ {
				r, c := row+dr, col+dc
				if r < 0 || r >= n || c < 0 || c >= n {
					continue
				}
				dark := false
				if dr >= 0 && dr <= 6 && dc >= 0 && dc <= 6 {
					if dr == 0 || dr == 6 || dc == 0 || dc == 6 || (dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4) {
						dark = true
					}
				}
				setFunc(r, c, dark)
			}
		}
	}
	drawFinder(0, 0)
	drawFinder(0, n-7)
	drawFinder(n-7, 0)

	// Alignment patterns
	if len(spec.alignCenters) > 0 {
		for _, ar := range spec.alignCenters {
			for _, ac := range spec.alignCenters {
				if (ar == 6 && ac == 6) || (ar == 6 && ac == n-7) || (ar == n-7 && ac == 6) {
					continue
				}
				for dr := -2; dr <= 2; dr++ {
					for dc := -2; dc <= 2; dc++ {
						dark := dr == -2 || dr == 2 || dc == -2 || dc == 2 || (dr == 0 && dc == 0)
						setFunc(ar+dr, ac+dc, dark)
					}
				}
			}
		}
	}

	// Timing patterns
	for i := 8; i < n-8; i++ {
		if !isFunction[6][i] {
			setFunc(6, i, i%2 == 0)
		}
		if !isFunction[i][6] {
			setFunc(i, 6, i%2 == 0)
		}
	}

	// Dark module & reserve format info areas
	setFunc(n-8, 8, true)
	for i := 0; i < 9; i++ {
		if i != 6 {
			isFunction[8][i] = true
			isFunction[i][8] = true
		}
	}
	for i := 0; i < 8; i++ {
		isFunction[8][n-1-i] = true
		isFunction[n-1-i][8] = true
	}

	// 4. Place data bits (upward/downward 2-column zigzag)
	bitIdx := 0
	totalBits := len(allCodewords) * 8
	upward := true
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			r := vert
			if upward {
				r = n - 1 - vert
			}
			for j := 0; j < 2; j++ {
				c := right - j
				if !isFunction[r][c] {
					dark := false
					if bitIdx < totalBits {
						dark = ((allCodewords[bitIdx/8] >> (7 - (bitIdx % 8))) & 1) == 1
						bitIdx++
					}
					// Apply Mask 0: (r + c) % 2 == 0
					if (r+c)%2 == 0 {
						dark = !dark
					}
					modules[r][c] = dark
				}
			}
		}
		upward = !upward
	}

	// 5. Draw Format Information for ECC Level L (01) and Mask 0 (000) -> 0x77C4 (30660)
	formatBits := 0x77C4
	getFmtBit := func(i int) bool {
		return ((formatBits >> i) & 1) == 1
	}
	for i := 0; i <= 5; i++ {
		modules[8][i] = getFmtBit(14 - i)
	}
	modules[8][7] = getFmtBit(8)
	modules[8][8] = getFmtBit(7)
	modules[7][8] = getFmtBit(6)
	for i := 0; i <= 5; i++ {
		modules[5-i][8] = getFmtBit(5 - i)
	}
	for i := 0; i <= 7; i++ {
		modules[8][n-1-i] = getFmtBit(i)
	}
	for i := 0; i <= 6; i++ {
		modules[n-7+i][8] = getFmtBit(14 - i)
	}
	return modules, n
}

// GenerateQRSVG returns an SVG string of a scannable QR code for the given text
func GenerateQRSVG(text string) string {
	modules, n := BuildQRMatrix(text)

	// 6. Render SVG with 4-module quiet zone
	border := 3
	totalSize := n + border*2
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="180" height="180" shape-rendering="crispEdges">`, totalSize, totalSize))
	sb.WriteString(`<rect width="100%" height="100%" fill="#ffffff" rx="2"/>`)
	sb.WriteString(`<path fill="#0f172a" d="`)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			if modules[r][c] {
				sb.WriteString(fmt.Sprintf("M%d,%dh1v1h-1z", c+border, r+border))
			}
		}
	}
	sb.WriteString(`"/>`)
	sb.WriteString(`</svg>`)
	return sb.String()
}

func rsComputeEC(data []byte, ecLen int) []byte {
	exp := make([]byte, 512)
	logTbl := make([]int, 256)
	x := 1
	for i := 0; i < 255; i++ {
		exp[i] = byte(x)
		exp[i+255] = byte(x)
		logTbl[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	gfMul := func(a, b byte) byte {
		if a == 0 || b == 0 {
			return 0
		}
		return exp[logTbl[a]+logTbl[b]]
	}

	gen := []byte{1}
	for i := 0; i < ecLen; i++ {
		next := make([]byte, len(gen)+1)
		factor := exp[i]
		for j, g := range gen {
			next[j] ^= g
			next[j+1] ^= gfMul(g, factor)
		}
		gen = next
	}

	rem := make([]byte, ecLen)
	for _, b := range data {
		lead := b ^ rem[0]
		copy(rem, rem[1:])
		rem[ecLen-1] = 0
		if lead != 0 {
			for j := 0; j < ecLen; j++ {
				rem[j] ^= gfMul(gen[j+1], lead)
			}
		}
	}
	return rem
}

// GetQRCodeSVG handles GET /api/sync/qr?url=...
func (h *SyncHandler) GetQRCodeSVG(c *fiber.Ctx) error {
	targetURL := strings.TrimSpace(c.Query("url"))
	if targetURL == "" {
		targetURL = "http://localhost"
	}
	svg := GenerateQRSVG(targetURL)
	c.Set("Content-Type", "image/svg+xml; charset=utf-8")
	return c.SendString(svg)
}
