package binaries

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type VDFNode struct {
	Name     string
	Value    string
	children []*VDFNode
}

func (n *VDFNode) Get(key string) *VDFNode {
	if n == nil {
		return nil
	}
	for _, child := range n.children {
		if strings.EqualFold(child.Name, key) {
			return child
		}
	}
	return nil
}

func (n *VDFNode) GetString(key, defaultVal string) string {
	child := n.Get(key)
	if child == nil || child.Value == "" {
		return defaultVal
	}
	return child.Value
}

func (n *VDFNode) GetInt(key string, defaultVal int) int {
	child := n.Get(key)
	if child == nil || child.Value == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(strings.TrimSpace(child.Value))
	if err != nil {
		return defaultVal
	}
	return v
}

func (n *VDFNode) ChildrenOrdered() []*VDFNode {
	if n == nil {
		return nil
	}
	out := make([]*VDFNode, len(n.children))
	copy(out, n.children)
	return out
}

func ParseVDF(text string) (*VDFNode, error) {
	tokens, err := tokenizeVDF(text)
	if err != nil {
		return nil, err
	}
	root := &VDFNode{Name: "root"}
	idx, err := parseVDFBlock(tokens, 0, root)
	if err != nil {
		return nil, err
	}
	if idx != len(tokens) {
		return nil, fmt.Errorf("unexpected trailing VDF token %q", tokens[idx].val)
	}
	return root, nil
}

type vdfTokenKind int

const (
	vdfTokenString vdfTokenKind = iota
	vdfTokenLBrace
	vdfTokenRBrace
)

type vdfToken struct {
	kind vdfTokenKind
	val  string
}

func parseVDFBlock(tokens []vdfToken, idx int, parent *VDFNode) (int, error) {
	for idx < len(tokens) {
		tok := tokens[idx]
		if tok.kind == vdfTokenRBrace {
			return idx, nil
		}
		if tok.kind != vdfTokenString {
			return idx, fmt.Errorf("expected key string in VDF, got %q", tok.val)
		}
		key := tok.val
		idx++
		if idx >= len(tokens) {
			return idx, fmt.Errorf("unexpected EOF after VDF key %q", key)
		}
		next := tokens[idx]
		switch next.kind {
		case vdfTokenString:
			parent.children = append(parent.children, &VDFNode{
				Name:  key,
				Value: next.val,
			})
			idx++
		case vdfTokenLBrace:
			child := &VDFNode{Name: key}
			nextIdx, err := parseVDFBlock(tokens, idx+1, child)
			if err != nil {
				return nextIdx, err
			}
			if nextIdx >= len(tokens) || tokens[nextIdx].kind != vdfTokenRBrace {
				return nextIdx, fmt.Errorf("unclosed '{' for VDF block %q", key)
			}
			parent.children = append(parent.children, child)
			idx = nextIdx + 1
		default:
			return idx, fmt.Errorf("unexpected token %q after VDF key %q", next.val, key)
		}
	}
	return idx, nil
}

func tokenizeVDF(text string) ([]vdfToken, error) {
	var tokens []vdfToken
	i := 0
	if strings.HasPrefix(text, "\xef\xbb\xbf") {
		i = 3
	}
	for i < len(text) {
		ch := text[i]
		if unicode.IsSpace(rune(ch)) {
			i++
			continue
		}
		if ch == '/' && i+1 < len(text) && text[i+1] == '/' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		switch ch {
		case '{':
			tokens = append(tokens, vdfToken{kind: vdfTokenLBrace, val: "{"})
			i++
		case '}':
			tokens = append(tokens, vdfToken{kind: vdfTokenRBrace, val: "}"})
			i++
		case '"':
			val, nextIdx, err := readQuotedVDFString(text, i+1)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, vdfToken{kind: vdfTokenString, val: val})
			i = nextIdx
		default:
			start := i
			for i < len(text) && !unicode.IsSpace(rune(text[i])) && text[i] != '{' && text[i] != '}' && text[i] != '"' {
				if text[i] == '/' && i+1 < len(text) && text[i+1] == '/' {
					break
				}
				i++
			}
			tokens = append(tokens, vdfToken{kind: vdfTokenString, val: text[start:i]})
		}
	}
	return tokens, nil
}

func readQuotedVDFString(text string, i int) (string, int, error) {
	var sb strings.Builder
	for i < len(text) {
		ch := text[i]
		if ch == '"' {
			return sb.String(), i + 1, nil
		}
		if ch == '\\' && i+1 < len(text) {
			esc := text[i+1]
			switch esc {
			case '\\', '"':
				sb.WriteByte(esc)
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(esc)
			}
			i += 2
			continue
		}
		sb.WriteByte(ch)
		i++
	}
	return "", i, fmt.Errorf("unterminated quoted string in VDF")
}
