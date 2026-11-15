package main

// A forgiving HTML parser, good enough for text extraction. It never fails:
// unknown or broken markup degrades to text, stray end tags are ignored and
// unclosed elements are closed when an ancestor closes.

import (
	"html"
	"strings"
)

type nodeType int

const (
	docNode nodeType = iota
	elemNode
	textNode
)

type node struct {
	typ      nodeType
	tag      string // lower-case tag name for elements
	text     string // decoded text for text nodes
	attrs    map[string]string
	parent   *node
	children []*node
}

func (n *node) attr(k string) string { return n.attrs[k] }

var voidTags = set("area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr")
var rawTags = set("script", "style", "textarea", "title", "xmp", "noscript")

// Elements that implicitly close an open sibling of the same kind.
var autoClose = map[string][]string{
	"p":  {"p"},
	"li": {"li"},
	"dt": {"dt", "dd"}, "dd": {"dt", "dd"},
	"tr": {"tr", "td", "th"}, "td": {"td", "th"}, "th": {"td", "th"},
	"option": {"option"},
}

// Block elements that close an open <p>.
var closesP = set("address", "article", "aside", "blockquote", "div", "dl", "fieldset", "figure",
	"footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "main", "nav", "ol",
	"pre", "section", "table", "ul")

func set(s ...string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

func parseHTML(src string) *node {
	doc := &node{typ: docNode}
	cur := doc

	appendText := func(t string) {
		if t == "" {
			return
		}
		cur.children = append(cur.children, &node{typ: textNode, text: html.UnescapeString(t), parent: cur})
	}
	open := func(tag string) *node { // nearest open ancestor with this tag
		for n := cur; n != nil && n.typ == elemNode; n = n.parent {
			if n.tag == tag {
				return n
			}
		}
		return nil
	}

	i := 0
	for i < len(src) {
		lt := strings.IndexByte(src[i:], '<')
		if lt < 0 {
			appendText(src[i:])
			break
		}
		appendText(src[i : i+lt])
		i += lt

		rest := src[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			if end := strings.Index(rest[4:], "-->"); end >= 0 {
				i += 4 + end + 3
			} else {
				i = len(src)
			}
			continue
		case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
			if end := strings.IndexByte(rest, '>'); end >= 0 {
				i += end + 1
			} else {
				i = len(src)
			}
			continue
		}

		closing := len(rest) > 1 && rest[1] == '/'
		j := 1
		if closing {
			j = 2
		}
		k := j
		for k < len(rest) && isTagChar(rest[k]) {
			k++
		}
		if k == j { // "<" not starting a tag
			appendText("<")
			i++
			continue
		}
		tag := strings.ToLower(rest[j:k])
		attrs, n, selfClose := parseAttrs(rest, k)
		i += n

		if closing {
			if target := open(tag); target != nil {
				cur = target.parent
			}
			continue
		}

		if closesP[tag] {
			if p := open("p"); p != nil {
				cur = p.parent
			}
		}
		for _, sib := range autoClose[tag] {
			if cur.typ == elemNode && cur.tag == sib {
				cur = cur.parent
			}
		}

		el := &node{typ: elemNode, tag: tag, attrs: attrs, parent: cur}
		cur.children = append(cur.children, el)
		if voidTags[tag] || selfClose {
			continue
		}
		if rawTags[tag] { // content up to the matching end tag is plain text
			endTag := "</" + tag
			end := strings.Index(strings.ToLower(src[i:]), endTag)
			if end < 0 {
				end = len(src) - i
			}
			text := src[i : i+end]
			if tag == "title" || tag == "textarea" {
				text = html.UnescapeString(text)
			}
			el.children = append(el.children, &node{typ: textNode, text: text, parent: el})
			i += end
			if gt := strings.IndexByte(src[i:], '>'); gt >= 0 {
				i += gt + 1
			} else {
				i = len(src)
			}
			continue
		}
		cur = el
	}
	return doc
}

func isTagChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':'
}

// parseAttrs reads attributes starting at s[i] up to '>' and returns them,
// the total length consumed from the start of s, and whether it ended in "/>".
func parseAttrs(s string, i int) (map[string]string, int, bool) {
	attrs := map[string]string{}
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '/') {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == '>' {
			return attrs, i + 1, s[i-1] == '/'
		}
		start := i
		for i < len(s) && s[i] != '=' && s[i] != '>' && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			if s[i] == '/' && i+1 < len(s) && s[i+1] == '>' {
				break
			}
			i++
		}
		name := strings.ToLower(s[start:i])
		val := ""
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i < len(s) && s[i] == '=' {
			i++
			for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
				i++
			}
			if i < len(s) && (s[i] == '"' || s[i] == '\'') {
				q := s[i]
				end := strings.IndexByte(s[i+1:], q)
				if end < 0 {
					val, i = s[i+1:], len(s)
				} else {
					val, i = s[i+1:i+1+end], i+1+end+1
				}
			} else {
				start := i
				for i < len(s) && s[i] != '>' && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' {
					i++
				}
				val = s[start:i]
			}
		}
		if name != "" {
			attrs[name] = html.UnescapeString(val)
		}
	}
	return attrs, len(s), false
}

// latin1ToUTF8 decodes ISO-8859-1 / windows-1252 bodies (the only legacy
// charsets still common on the web).
func latin1ToUTF8(b []byte) string {
	cp1252 := map[byte]rune{0x80: '€', 0x82: '‚', 0x84: '„', 0x85: '…', 0x91: '\'', 0x92: '\'', 0x93: '"', 0x94: '"', 0x96: '–', 0x97: '—', 0x99: '™'}
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		if r, ok := cp1252[c]; ok {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}
