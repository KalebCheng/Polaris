package service

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	bizerrors "polaris/internal/errors"
)

var emailHTMLTagPattern = regexp.MustCompile(`(?i)<[a-z][^>]*>`)

var allowedEmailTags = map[string]bool{
	"a": true, "b": true, "blockquote": true, "br": true, "div": true,
	"em": true, "h1": true, "h2": true, "h3": true, "hr": true,
	"i": true, "li": true, "ol": true, "p": true, "s": true,
	"span": true, "strong": true, "u": true, "ul": true,
}

var droppedEmailTags = map[string]bool{
	"base": true, "button": true, "embed": true, "form": true, "head": true,
	"iframe": true, "input": true, "link": true, "math": true, "meta": true,
	"object": true, "option": true, "script": true, "select": true, "style": true,
	"svg": true, "textarea": true, "title": true,
}

var allowedEmailAttrs = map[string]map[string]bool{
	"a": {"href": true, "rel": true, "target": true, "title": true},
}

// sanitizeEmailHTML 把编辑器输出收敛到邮件正文允许的标签与属性。
//
// 富文本内容会进入邮件正文，也会在控制台预览；这里统一剥离脚本、事件属性与非安全协议，
// 避免把"模板编辑器"变成存储型 XSS 的入口。纯文本历史数据会被转换为段落，保证升级后仍可编辑/发送。
func sanitizeEmailHTML(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", nil
	}
	if !emailHTMLTagPattern.MatchString(content) {
		return plainTextEmailHTML(content), nil
	}
	context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(content), context)
	if err != nil {
		return "", bizerrors.ErrInvalidParams.WithDetail("content", "invalid_html")
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	for child := root.FirstChild; child != nil; {
		next := child.NextSibling
		sanitizeEmailNode(child)
		child = next
	}
	var buf bytes.Buffer
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := html.Render(&buf, node); err != nil {
			return "", bizerrors.ErrInvalidParams.WithDetail("content", "invalid_html")
		}
	}
	return strings.TrimSpace(buf.String()), nil
}

func sanitizeEmailNode(node *html.Node) {
	if node == nil {
		return
	}
	if node.Parent == nil {
		return
	}
	if node.Type == html.CommentNode {
		node.Parent.RemoveChild(node)
		return
	}
	if node.Type != html.ElementNode {
		return
	}
	tag := strings.ToLower(node.Data)
	if droppedEmailTags[tag] {
		node.Parent.RemoveChild(node)
		return
	}
	if !allowedEmailTags[tag] {
		parent := node.Parent
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			parent.InsertBefore(child, node)
			sanitizeEmailNode(child)
			child = next
		}
		parent.RemoveChild(node)
		return
	}
	filterEmailAttrs(node, tag)
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		sanitizeEmailNode(child)
		child = next
	}
}

func filterEmailAttrs(node *html.Node, tag string) {
	allowed := allowedEmailAttrs[tag]
	attrs := node.Attr[:0]
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		if !allowed[key] {
			continue
		}
		if key == "href" && !safeEmailURL(attr.Val) {
			continue
		}
		if key == "target" && attr.Val != "_blank" {
			continue
		}
		attr.Key = key
		attrs = append(attrs, attr)
	}
	node.Attr = attrs
	if tag == "a" && hasEmailAttr(node, "target") && !hasEmailAttr(node, "rel") {
		node.Attr = append(node.Attr, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
	}
}

func hasEmailAttr(node *html.Node, key string) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return true
		}
	}
	return false
}

func safeEmailURL(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "{{") && strings.HasSuffix(value, "}}") {
		return true
	}
	for _, prefix := range []string{"http://", "https://", "mailto:", "tel:"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func plainTextEmailHTML(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	blocks := strings.Split(normalized, "\n\n")
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		for i, line := range lines {
			lines[i] = html.EscapeString(line)
		}
		lineHTML := strings.Join(lines, "<br>")
		if strings.TrimSpace(lineHTML) == "" {
			continue
		}
		parts = append(parts, "<p>"+lineHTML+"</p>")
	}
	return strings.Join(parts, "")
}
