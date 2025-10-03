package htmltomarkdown

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

var (
	scriptPattern  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	commentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
)

type Converter struct {
	base *url.URL
}

type Option interface{}

type OptionFunc func(*Converter)

type Options struct{}

type Rule struct{}

func NewConverter(baseURL string, _ bool, _ interface{}) *Converter {
	var base *url.URL
	if baseURL != "" {
		if parsed, err := url.Parse(baseURL); err == nil {
			base = parsed
		}
	}
	return &Converter{base: base}
}

func (c *Converter) AddRules(_ ...Rule) {}

func (c *Converter) AddOptions(_ ...Option) {}

func (c *Converter) ConvertString(input string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("converter is nil")
	}
	markdown := convertHTMLToMarkdown(c.base, []byte(input))
	return strings.TrimSpace(markdown), nil
}

type htmlTokenType int

const (
	tokenText htmlTokenType = iota
	tokenStartTag
	tokenEndTag
)

type htmlToken struct {
	typ   htmlTokenType
	tag   string
	attrs map[string]string
	data  string
}

type nodeContext struct {
	tag                string
	attrs              map[string]string
	content            strings.Builder
	parent             *nodeContext
	listIndex          int
	preserveWhitespace bool
}

func convertHTMLToMarkdown(base *url.URL, body []byte) string {
	cleaned := cleanupHTMLInput(body)
	tokens := tokenizeHTML(cleaned)
	if len(tokens) == 0 {
		return ""
	}

	root := &nodeContext{tag: "", attrs: nil, parent: nil}
	stack := []*nodeContext{root}
	for _, tok := range tokens {
		switch tok.typ {
		case tokenText:
			ctx := stack[len(stack)-1]
			writeText(ctx, tok.data)
		case tokenStartTag:
			ctx := stack[len(stack)-1]
			child := &nodeContext{
				tag:                tok.tag,
				attrs:              tok.attrs,
				parent:             ctx,
				preserveWhitespace: ctx.preserveWhitespace || isPreformattedTag(tok.tag),
			}
			stack = append(stack, child)
		case tokenEndTag:
			if len(stack) <= 1 {
				continue
			}
			ctx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			parent := stack[len(stack)-1]
			rendered := renderMarkdownNode(ctx, base)
			parent.content.WriteString(rendered)
		}
	}

	for len(stack) > 1 {
		ctx := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		parent := stack[len(stack)-1]
		rendered := renderMarkdownNode(ctx, base)
		parent.content.WriteString(rendered)
	}

	output := normalizeMarkdown(root.content.String())
	return strings.TrimSpace(output)
}

func cleanupHTMLInput(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}
	cleaned := scriptPattern.ReplaceAll(body, nil)
	cleaned = stylePattern.ReplaceAll(cleaned, nil)
	cleaned = commentPattern.ReplaceAll(cleaned, nil)
	return cleaned
}

func tokenizeHTML(input []byte) []htmlToken {
	var tokens []htmlToken
	reader := bytes.NewReader(input)
	var buffer bytes.Buffer

	for {
		r, err := reader.ReadByte()
		if err != nil {
			break
		}
		if r == '<' {
			if buffer.Len() > 0 {
				tokens = append(tokens, htmlToken{typ: tokenText, data: buffer.String()})
				buffer.Reset()
			}
			tagContent, ok := readUntil(reader, '>')
			if !ok {
				buffer.WriteByte(r)
				buffer.Write(tagContent)
				break
			}
			tagString := string(tagContent)
			lower := strings.ToLower(strings.TrimSpace(tagString))
			if strings.HasPrefix(lower, "!--") {
				skipUntil(reader, "-->")
				continue
			}
			if strings.HasPrefix(lower, "!doctype") || strings.HasPrefix(lower, "![cdata[") {
				continue
			}
			closing := strings.HasPrefix(lower, "/")
			selfClosing := strings.HasSuffix(lower, "/")
			tagName, attrs := parseTag(strings.TrimSpace(tagString))
			if tagName == "" {
				continue
			}
			tagNameLower := strings.ToLower(tagName)
			if closing {
				tokens = append(tokens, htmlToken{typ: tokenEndTag, tag: tagNameLower})
				continue
			}
			if tagNameLower == "script" || tagNameLower == "style" {
				skipUntil(reader, fmt.Sprintf("</%s>", tagNameLower))
				continue
			}
			tokens = append(tokens, htmlToken{typ: tokenStartTag, tag: tagNameLower, attrs: attrs})
			if selfClosing {
				tokens = append(tokens, htmlToken{typ: tokenEndTag, tag: tagNameLower})
			}
		} else {
			buffer.WriteByte(r)
		}
	}
	if buffer.Len() > 0 {
		tokens = append(tokens, htmlToken{typ: tokenText, data: buffer.String()})
	}
	return tokens
}

func readUntil(reader *bytes.Reader, delim byte) ([]byte, bool) {
	var buf bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return buf.Bytes(), false
		}
		if b == delim {
			return buf.Bytes(), true
		}
		buf.WriteByte(b)
	}
}

func skipUntil(reader *bytes.Reader, terminator string) {
	if terminator == "" {
		return
	}
	lowerTerm := strings.ToLower(terminator)
	matchIndex := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}
		lowerChar := strings.ToLower(string([]byte{b}))
		if lowerChar[0] == lowerTerm[matchIndex] {
			matchIndex++
			if matchIndex == len(lowerTerm) {
				return
			}
			continue
		}
		if matchIndex > 0 {
			if lowerChar[0] == lowerTerm[0] {
				matchIndex = 1
			} else {
				matchIndex = 0
			}
		}
	}
}

func parseTag(input string) (string, map[string]string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}
	if trimmed[0] == '/' {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	if trimmed == "" {
		return "", nil
	}
	if trimmed[len(trimmed)-1] == '/' {
		trimmed = strings.TrimSpace(trimmed[:len(trimmed)-1])
	}

	var nameBuilder strings.Builder
	i := 0
	for i < len(trimmed) {
		ch := trimmed[i]
		if isSpace(ch) {
			break
		}
		nameBuilder.WriteByte(ch)
		i++
	}
	name := strings.ToLower(nameBuilder.String())
	attrs := map[string]string{}
	rest := strings.TrimSpace(trimmed[i:])
	for len(rest) > 0 {
		rest = strings.TrimLeftFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
		if rest == "" {
			break
		}
		eqIndex := strings.IndexByte(rest, '=')
		spaceIndex := strings.IndexFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
		if eqIndex == -1 || (spaceIndex != -1 && spaceIndex < eqIndex) {
			attrName := strings.ToLower(strings.TrimSpace(rest))
			attrs[attrName] = ""
			break
		}
		attrName := strings.ToLower(strings.TrimSpace(rest[:eqIndex]))
		rest = strings.TrimSpace(rest[eqIndex+1:])
		if rest == "" {
			attrs[attrName] = ""
			break
		}
		var value string
		if rest[0] == '\'' || rest[0] == '"' {
			quote := rest[0]
			rest = rest[1:]
			closing := strings.IndexByte(rest, quote)
			if closing == -1 {
				value = rest
				rest = ""
			} else {
				value = rest[:closing]
				rest = rest[closing+1:]
			}
		} else {
			nextSpace := strings.IndexFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
			if nextSpace == -1 {
				value = rest
				rest = ""
			} else {
				value = rest[:nextSpace]
				rest = rest[nextSpace+1:]
			}
		}
		attrs[attrName] = html.UnescapeString(strings.TrimSpace(value))
	}
	return name, attrs
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

func writeText(ctx *nodeContext, text string) {
	if text == "" {
		return
	}
	data := html.UnescapeString(text)
	if !ctx.preserveWhitespace {
		leading := hasLeadingWhitespace(data)
		trailing := hasTrailingWhitespace(data)
		data = collapseSpaces(data)
		data = strings.TrimSpace(data)
		if leading && ctx.content.Len() > 0 {
			ctx.content.WriteRune(' ')
		}
		if data != "" {
			ctx.content.WriteString(data)
			if trailing {
				ctx.content.WriteRune(' ')
			}
		} else if trailing && ctx.content.Len() > 0 {
			ctx.content.WriteRune(' ')
		}
		return
	}
	ctx.content.WriteString(data)
}

func collapseSpaces(input string) string {
	hasSpace := false
	var builder strings.Builder
	for _, r := range input {
		switch r {
		case '\n', '\t', '\r':
			r = ' '
		}
		if r == ' ' {
			if builder.Len() == 0 || hasSpace {
				continue
			}
			hasSpace = true
			builder.WriteRune(r)
			continue
		}
		hasSpace = false
		builder.WriteRune(r)
	}
	return builder.String()
}

func hasLeadingWhitespace(input string) bool {
	for _, r := range input {
		switch r {
		case ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	}
	return false
}

func hasTrailingWhitespace(input string) bool {
	for i := len(input) - 1; i >= 0; i-- {
		switch input[i] {
		case ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	}
	return false
}

func isPreformattedTag(tag string) bool {
	switch tag {
	case "pre", "code", "textarea":
		return true
	default:
		return false
	}
}

func renderMarkdownNode(ctx *nodeContext, base *url.URL) string {
	tag := ctx.tag
	inner := ctx.content.String()
	switch tag {
	case "style", "script", "head":
		return ""
	case "br":
		return "\n"
	case "hr":
		return "\n\n---\n\n"
	case "strong", "b":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		return "**" + trimmed + "**"
	case "em", "i":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		return "_" + trimmed + "_"
	case "code":
		if ctx.parent != nil && ctx.parent.tag == "pre" {
			return inner
		}
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		trimmed = strings.ReplaceAll(trimmed, "`", "\u0060")
		return "`" + trimmed + "`"
	case "pre":
		content := strings.Trim(inner, "\n")
		if content == "" {
			return ""
		}
		return "\n\n```\n" + content + "\n```\n\n"
	case "p", "div", "section", "article", "main":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		return "\n\n" + trimmed + "\n\n"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		level := 1
		if len(tag) > 1 {
			switch tag[1] {
			case '2':
				level = 2
			case '3':
				level = 3
			case '4':
				level = 4
			case '5':
				level = 5
			case '6':
				level = 6
			}
		}
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return "\n\n" + strings.Repeat("#", level) + " " + trimmed + "\n\n"
	case "ul", "ol":
		content := strings.Trim(inner, "\n")
		if content == "" {
			return ""
		}
		return "\n" + content + "\n"
	case "li":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		marker := "-"
		if ctx.parent != nil && ctx.parent.tag == "ol" {
			ctx.parent.listIndex++
			marker = fmt.Sprintf("%d.", ctx.parent.listIndex)
		}
		lines := strings.Split(trimmed, "\n")
		var builder strings.Builder
		builder.WriteString(marker)
		builder.WriteByte(' ')
		builder.WriteString(strings.TrimSpace(lines[0]))
		builder.WriteByte('\n')
		for _, line := range lines[1:] {
			stripped := strings.TrimSpace(line)
			if stripped == "" {
				builder.WriteByte('\n')
				continue
			}
			builder.WriteString("  ")
			builder.WriteString(stripped)
			builder.WriteByte('\n')
		}
		return builder.String()
	case "blockquote":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		lines := strings.Split(trimmed, "\n")
		for i, line := range lines {
			lines[i] = "> " + strings.TrimSpace(line)
		}
		return "\n" + strings.Join(lines, "\n") + "\n\n"
	case "a":
		href := strings.TrimSpace(ctx.attrs["href"])
		text := strings.TrimSpace(inner)
		if text == "" {
			return ""
		}
		if href == "" {
			return text
		}
		resolved := resolveURL(base, href)
		return fmt.Sprintf("[%s](%s)", text, resolved)
	case "img":
		src := strings.TrimSpace(ctx.attrs["src"])
		if src == "" {
			return ""
		}
		alt := strings.TrimSpace(ctx.attrs["alt"])
		resolved := resolveURL(base, src)
		if alt == "" {
			alt = "image"
		}
		return fmt.Sprintf("![%s](%s)", alt, resolved)
	case "table", "tbody", "thead", "tr":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		return "\n" + trimmed + "\n"
	case "th":
		trimmed := strings.TrimSpace(inner)
		if trimmed == "" {
			return ""
		}
		return "**" + trimmed + "**\t"
	case "td":
		return strings.TrimSpace(inner) + "\t"
	default:
		return inner
	}
}

func resolveURL(base *url.URL, raw string) string {
	if base == nil {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if !parsed.IsAbs() {
		parsed = base.ResolveReference(parsed)
	}
	return parsed.String()
}

func normalizeMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	previousEmpty := false
	for _, line := range lines {
		trimmed := strings.TrimRightFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		if strings.TrimSpace(trimmed) == "" {
			if !previousEmpty {
				result = append(result, "")
			}
			previousEmpty = true
			continue
		}
		previousEmpty = false
		result = append(result, trimmed)
	}
	collapsed := make([]string, 0, len(result))
	blankCount := 0
	for _, line := range result {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount > 2 {
				continue
			}
		} else {
			blankCount = 0
		}
		collapsed = append(collapsed, line)
	}
	return strings.Join(collapsed, "\n")
}
