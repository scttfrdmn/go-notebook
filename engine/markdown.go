package engine

import (
	"strings"
)

// renderMarkdown converts a small, safe subset of Markdown to HTML. It exists so a
// cell that returns text/markdown (every notebook's intro() does) is shown as
// formatted prose rather than raw source — the client injects text/html but paints
// everything else as plain text, so the conversion has to happen here, at the one
// Go chokepoint every rendered output passes through (AsRendered).
//
// It is deliberately a SUBSET, not a full CommonMark parser:
//
//   - stdlib only. The engine ships pure stdlib so a notebook cross-compiles to a
//     static binary and to wasm; pulling in goldmark/blackfriday would forfeit
//     that. The intros need headings, emphasis, code, lists, links, and
//     paragraphs, and that is what this does.
//   - safe by construction. Every run of literal text is HTML-escaped before any
//     tag is emitted, and the only attribute ever written is an href that must
//     start with http/https/# (see safeURL). There is no raw-HTML passthrough, so
//     a notebook's prose cannot inject script — the reason the client refused to
//     innerHTML text/markdown in the first place is handled here instead.
//
// Supported: # / ## / ### headings; - and * bullet lists; **bold**, *italic*,
// `code`; [text](url) links; blank-line-separated paragraphs. Anything else is
// passed through as escaped text, so unknown syntax degrades to plain words rather
// than breaking — the same degradation-ladder discipline the rest of the view uses.
func renderMarkdown(src string) string {
	var b strings.Builder
	lines := strings.Split(src, "\n")

	var para []string // accumulating lines of the current paragraph
	var inList bool   // currently inside a <ul>
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		b.WriteString("<p>")
		b.WriteString(inline(strings.Join(para, " ")))
		b.WriteString("</p>\n")
		para = para[:0]
	}
	closeList := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
	}

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)

		// Blank line: paragraph / list break.
		if trimmed == "" {
			flushPara()
			closeList()
			continue
		}

		// Heading: one to three leading '#'.
		if h := headingLevel(trimmed); h > 0 {
			flushPara()
			closeList()
			text := strings.TrimSpace(trimmed[h:])
			tag := "h" + string(rune('0'+h))
			b.WriteString("<" + tag + ">")
			b.WriteString(inline(text))
			b.WriteString("</" + tag + ">\n")
			continue
		}

		// Bullet list item: "- " or "* ".
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushPara()
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			b.WriteString("<li>")
			b.WriteString(inline(strings.TrimSpace(trimmed[2:])))
			b.WriteString("</li>\n")
			continue
		}

		// Otherwise accumulate into the current paragraph.
		closeList()
		para = append(para, trimmed)
	}
	flushPara()
	closeList()
	return b.String()
}

// headingLevel returns 1..3 for a leading run of that many '#' followed by a
// space, else 0. (Only the levels the intros use; deeper ones degrade to text.)
func headingLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n >= 1 && n <= 3 && n < len(s) && s[n] == ' ' {
		return n
	}
	return 0
}

// inline renders the span-level syntax (**bold**, *italic*, `code`, [text](url))
// within one already-line-joined string. It escapes as it goes: literal text is
// always escaped, emitted tags are fixed strings, and a link's href is validated
// by safeURL — so nothing a notebook writes reaches innerHTML unescaped.
//
// The syntax bytes it dispatches on are all ASCII; any other byte (including a
// UTF-8 continuation byte of a multi-byte rune like an em-dash) is copied through
// verbatim by WriteByte, so multi-byte characters survive intact — writing them as
// string(byte) would mangle them into replacement characters.
func inline(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		switch {
		case s[i] == '`':
			// Inline code up to the next backtick; contents escaped, not parsed.
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				b.WriteString("<code>")
				b.WriteString(escape(s[i+1 : i+1+j]))
				b.WriteString("</code>")
				i += j + 2
				continue
			}
		case strings.HasPrefix(s[i:], "**"):
			if j := strings.Index(s[i+2:], "**"); j >= 0 {
				b.WriteString("<strong>")
				b.WriteString(inline(s[i+2 : i+2+j]))
				b.WriteString("</strong>")
				i += j + 4
				continue
			}
		case s[i] == '*':
			if j := strings.IndexByte(s[i+1:], '*'); j >= 0 {
				b.WriteString("<em>")
				b.WriteString(inline(s[i+1 : i+1+j]))
				b.WriteString("</em>")
				i += j + 2
				continue
			}
		case s[i] == '[':
			// [text](url) — only if the whole shape is present and the url is safe.
			if text, url, n, ok := parseLink(s[i:]); ok {
				if safe := safeURL(url); safe != "" {
					b.WriteString(`<a href="`)
					b.WriteString(escape(safe))
					b.WriteString(`">`)
					b.WriteString(inline(text))
					b.WriteString("</a>")
					i += n
					continue
				}
			}
		}
		// Default: a literal byte. escape() maps the four HTML-special ASCII bytes to
		// entities and passes every other byte (ASCII or a UTF-8 continuation byte)
		// through unchanged, so multi-byte runes stay intact.
		writeEscapedByte(&b, s[i])
		i++
	}
	return b.String()
}

// parseLink parses a leading "[text](url)" and returns text, url, the number of
// bytes consumed, and ok. It does not allow nested brackets in the text, which the
// intros never use.
func parseLink(s string) (text, url string, n int, ok bool) {
	if len(s) == 0 || s[0] != '[' {
		return "", "", 0, false
	}
	close := strings.IndexByte(s, ']')
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return "", "", 0, false
	}
	end := strings.IndexByte(s[close+2:], ')')
	if end < 0 {
		return "", "", 0, false
	}
	text = s[1:close]
	url = s[close+2 : close+2+end]
	return text, url, close + 2 + end + 1, true
}

// safeURL returns the URL if it is a safe link target (http, https, or an in-page
// anchor), else "". This is the one place a notebook-supplied string becomes an
// attribute, so it must reject javascript:, data:, and other script vectors.
func safeURL(u string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"), strings.HasPrefix(u, "#"):
		return u
	default:
		return ""
	}
}

// escape HTML-escapes a whole string, byte by byte. Non-special bytes (including
// UTF-8 continuation bytes) are copied verbatim, so multi-byte runes are preserved.
func escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		writeEscapedByte(&b, s[i])
	}
	return b.String()
}

// writeEscapedByte writes one byte to b, mapping the four HTML-special ASCII bytes
// (&, <, >, ") to entities and every other byte through unchanged via WriteByte.
// WriteByte (not WriteString(string(c))) is what keeps a multi-byte rune's bytes
// intact — string(aContinuationByte) would produce U+FFFD.
func writeEscapedByte(b *strings.Builder, c byte) {
	switch c {
	case '&':
		b.WriteString("&amp;")
	case '<':
		b.WriteString("&lt;")
	case '>':
		b.WriteString("&gt;")
	case '"':
		b.WriteString("&#34;")
	default:
		b.WriteByte(c)
	}
}
