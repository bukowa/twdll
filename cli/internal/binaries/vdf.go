package binaries

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// VDFNode represents a KeyValues 1 node (either a section with children or a key-value property).
type VDFNode struct {
	Name     string
	Value    string
	Children map[string]*VDFNode // keys are normalized to lowercase for case-insensitive lookup
	Keys     []string            // preserved insertion order of child keys
}

// NewVDFNode creates an empty VDF node.
func NewVDFNode(name string) *VDFNode {
	return &VDFNode{
		Name:     name,
		Children: make(map[string]*VDFNode),
		Keys:     make([]string, 0),
	}
}

// Get returns the child node matching key (case-insensitive), or nil if absent.
func (n *VDFNode) Get(key string) *VDFNode {
	if n == nil || n.Children == nil {
		return nil
	}
	return n.Children[strings.ToLower(key)]
}

// ChildrenOrdered returns the children in the exact order they appeared in the VDF file.
func (n *VDFNode) ChildrenOrdered() []*VDFNode {
	if n == nil || len(n.Keys) == 0 {
		return nil
	}
	ordered := make([]*VDFNode, 0, len(n.Keys))
	for _, k := range n.Keys {
		if child := n.Children[k]; child != nil {
			ordered = append(ordered, child)
		}
	}
	return ordered
}

// GetString returns the string value for key (case-insensitive), or defaultValue if absent.
func (n *VDFNode) GetString(key string, defaultValue string) string {
	child := n.Get(key)
	if child != nil && child.Value != "" {
		return child.Value
	}
	return defaultValue
}

// GetInt returns the integer value for key (case-insensitive), or defaultValue if absent or invalid.
func (n *VDFNode) GetInt(key string, defaultValue int) int {
	str := n.GetString(key, "")
	if str == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return defaultValue
	}
	return val
}

// ParseVDF parses Valve KeyValues 1 text into a root VDFNode.
func ParseVDF(text string) (*VDFNode, error) {
	tokens, err := tokenizeVDF(text)
	if err != nil {
		return nil, fmt.Errorf("tokenize VDF: %w", err)
	}

	root := NewVDFNode("root")
	idx := 0
	for idx < len(tokens) {
		key := tokens[idx]
		idx++
		if idx >= len(tokens) {
			break
		}
		if tokens[idx] == "{" {
			idx++
			child, newIdx, err := parseVDFSection(key, tokens, idx)
			if err != nil {
				return nil, err
			}
			idx = newIdx
			k := strings.ToLower(key)
			if _, exists := root.Children[k]; !exists {
				root.Keys = append(root.Keys, k)
			}
			root.Children[k] = child
		} else {
			// Key-value pair at root
			val := tokens[idx]
			idx++
			child := NewVDFNode(key)
			child.Value = val
			k := strings.ToLower(key)
			if _, exists := root.Children[k]; !exists {
				root.Keys = append(root.Keys, k)
			}
			root.Children[k] = child
		}
	}
	return root, nil
}

func parseVDFSection(name string, tokens []string, startIdx int) (*VDFNode, int, error) {
	node := NewVDFNode(name)
	idx := startIdx
	for idx < len(tokens) {
		token := tokens[idx]
		if token == "}" {
			return node, idx + 1, nil
		}
		idx++
		if idx >= len(tokens) {
			return nil, idx, errors.New("unexpected end of file inside section " + name)
		}
		if tokens[idx] == "{" {
			idx++
			child, newIdx, err := parseVDFSection(token, tokens, idx)
			if err != nil {
				return nil, newIdx, err
			}
			idx = newIdx
			k := strings.ToLower(token)
			if _, exists := node.Children[k]; !exists {
				node.Keys = append(node.Keys, k)
			}
			node.Children[k] = child
		} else {
			val := tokens[idx]
			idx++
			child := NewVDFNode(token)
			child.Value = val
			k := strings.ToLower(token)
			if _, exists := node.Children[k]; !exists {
				node.Keys = append(node.Keys, k)
			}
			node.Children[k] = child
		}
	}
	return nil, idx, errors.New("unclosed section " + name)
}

func tokenizeVDF(text string) ([]string, error) {
	var tokens []string
	chars := []rune(text)
	n := len(chars)
	i := 0

	for i < n {
		ch := chars[i]

		// Skip whitespace
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Single line comments //
		if ch == '/' && i+1 < n && chars[i+1] == '/' {
			i += 2
			for i < n && chars[i] != '\n' {
				i++
			}
			continue
		}

		// Structural braces
		if ch == '{' || ch == '}' {
			tokens = append(tokens, string(ch))
			i++
			continue
		}

		// Quoted string
		if ch == '"' {
			i++
			var sb strings.Builder
			for i < n {
				c := chars[i]
				if c == '\\' && i+1 < n {
					next := chars[i+1]
					switch next {
					case '\\':
						sb.WriteRune('\\')
					case '"':
						sb.WriteRune('"')
					case 'n':
						sb.WriteRune('\n')
					case 't':
						sb.WriteRune('\t')
					default:
						sb.WriteRune('\\')
						sb.WriteRune(next)
					}
					i += 2
					continue
				}
				if c == '"' {
					i++ // consume closing quote
					break
				}
				sb.WriteRune(c)
				i++
			}
			tokens = append(tokens, sb.String())
			continue
		}

		// Bare word
		start := i
		for i < n && !unicode.IsSpace(chars[i]) && chars[i] != '{' && chars[i] != '}' && chars[i] != '"' {
			i++
		}
		tokens = append(tokens, string(chars[start:i]))
	}

	return tokens, nil
}
