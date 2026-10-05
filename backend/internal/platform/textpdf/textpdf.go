// Package textpdf writes plain-text PDF documents: headings and wrapped paragraphs in the
// standard Helvetica fonts, on A4 pages. It exists so exports need no third-party PDF library;
// it draws nothing but text.
package textpdf

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Page geometry in points (A4) and type sizes.
const (
	pageWidth   = 595.0
	pageHeight  = 842.0
	margin      = 50.0
	bodySize    = 9.5
	headingSize = 13.0
	titleSize   = 18.0
	leading     = 1.35
)

// Document is a PDF being written: call Title, Heading and Paragraph in order, then Bytes.
type Document struct {
	pages [][]string // content stream operations per page
	y     float64
}

// New returns an empty document.
func New() *Document {
	d := &Document{}
	d.newPage()
	return d
}

func (d *Document) newPage() {
	d.pages = append(d.pages, nil)
	d.y = pageHeight - margin
}

// Title adds the document title.
func (d *Document) Title(s string) { d.block(s, "F2", titleSize, 8) }

// Heading adds a section heading.
func (d *Document) Heading(s string) { d.block(s, "F2", headingSize, 4) }

// Paragraph adds wrapped body text.
func (d *Document) Paragraph(s string) { d.block(s, "F1", bodySize, 5) }

// Label adds a bold label followed by its value, wrapped as one paragraph.
func (d *Document) Label(label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	d.block(label+": "+value, "F1", bodySize, 2)
}

func (d *Document) block(s, font string, size, after float64) {
	width := pageWidth - 2*margin
	for _, para := range strings.Split(s, "\n") {
		for _, line := range wrap(winAnsi(para), size, width) {
			if d.y-size*leading < margin {
				d.newPage()
			}
			d.y -= size * leading
			p := &d.pages[len(d.pages)-1]
			*p = append(*p, fmt.Sprintf("BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET", font, size, margin, d.y, escape(line)))
		}
	}
	d.y -= after
}

// Bytes returns the finished PDF.
func (d *Document) Bytes() []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	n := len(d.pages)
	// 1 catalog, 2 pages, 3 and 4 fonts, then a page and its contents per page.
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	kids := make([]string, n)
	for i := range n {
		kids[i] = fmt.Sprintf("%d 0 R", 5+2*i)
	}
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, ops := range d.pages {
		footer := fmt.Sprintf("BT /F1 8 Tf %.1f %.1f Td (Page %d of %d) Tj ET", margin, margin/2, i+1, n)
		content := strings.Join(append(ops, footer), "\n")
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			pageWidth, pageHeight, 6+2*i))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

// wrap breaks a line into lines no wider than width at size, at spaces where possible.
func wrap(s string, size, width float64) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	cur := ""
	for _, w := range words {
		next := w
		if cur != "" {
			next = cur + " " + w
		}
		if textWidth(next, size) <= width || cur == "" {
			cur = next
			continue
		}
		out = append(out, cur)
		cur = w
	}
	return append(out, cur)
}

func textWidth(s string, size float64) float64 {
	units := 0
	for i := 0; i < len(s); i++ {
		units += charWidth(s[i])
	}
	return float64(units) * size / 1000
}

// charWidth is Helvetica's advance width of a byte, in thousandths of the type size.
func charWidth(c byte) int {
	if c >= 32 && c <= 126 {
		return helvetica[c-32]
	}
	return 556
}

// helvetica holds the widths of printable ASCII (32 to 126) from the Helvetica metrics.
var helvetica = [95]int{ //nolint:gochecknoglobals // a fixed font metrics table
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

// winAnsi maps text to the fonts' encoding: Latin-1 letters keep their byte, common typographic
// characters become their WinAnsi bytes, currency signs without one are spelled out, and
// anything else becomes "?".
func winAnsi(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		switch {
		case r < 0x80 || (r >= 0xA0 && r <= 0xFF):
			b.WriteByte(byte(r)) //nolint:gosec // checked above to be at most 0xFF
		default:
			switch r {
			case '‘':
				b.WriteByte(0x91)
			case '’':
				b.WriteByte(0x92)
			case '“':
				b.WriteByte(0x93)
			case '”':
				b.WriteByte(0x94)
			case '–':
				b.WriteByte(0x96)
			case '—':
				b.WriteByte(0x97)
			case '…':
				b.WriteString("...")
			case '•':
				b.WriteByte(0x95)
			case '€':
				b.WriteByte(0x80)
			case '₦':
				b.WriteString("NGN ")
			case '¢':
				b.WriteString("c")
			default:
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

// escape escapes a string for a PDF literal string; bytes outside ASCII are written as octal.
func escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(' || c == ')' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 32 || c > 126:
			fmt.Fprintf(&b, "\\%03o", c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
