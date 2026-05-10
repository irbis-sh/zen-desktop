package removejsconstant

import (
	"bytes"
	"fmt"
	"log"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// stripKeys removes the specified key definitions from the JavaScript code.
func stripKeys(script []byte, keys [][]string) ([]byte, error) {
	// TODO performance optimizations:
	// - prune multiple keys in one pass
	// - check whether at least one key is present before parsing (trie, Aho-Corasick)

	ast, err := js.Parse(parse.NewInput(bytes.NewReader(script)), js.Options{})
	if err != nil {
		return nil, fmt.Errorf("parse JS: %w", err)
	}

	for _, key := range keys {
		prune(ast, key)
	}

	var buf bytes.Buffer
	ast.JS(&buf)
	return buf.Bytes(), nil
}

// prune mutates the AST in-place.
//
// The path is a list of property names to traverse the object literal, where
//   - path[0] is top-level binding name
//   - path[1:] is property trail inside object literals
func prune(root *js.AST, path []string) {
	if len(path) == 0 {
		return
	}

	block := root.BlockStmt
	for i := 0; i < len(block.List); {
		vd, ok := block.List[i].(*js.VarDecl)
		if !ok {
			i++
			continue
		}

		if vd.TokenType != js.VarToken {
			// Currently we only prune var declarations.
			i++
			continue
		}

		pruneVarDecl(vd, path)

		// If the declaration lost all bindings, drop the whole stmt.
		if len(vd.List) == 0 {
			block.List = append(block.List[:i], block.List[i+1:]...)
			continue
		}
		i++
	}
}

func pruneVarDecl(vd *js.VarDecl, path []string) {
	name := path[0]

	for i := 0; i < len(vd.List); {
		be := &vd.List[i]

		if bindingName(be.Binding) != name {
			i++
			continue
		}

		// Case 1: remove the whole binding.
		if len(path) == 1 {
			vd.List = append(vd.List[:i], vd.List[i+1:]...)
			continue
		}

		// Case 2: recurse into object literal.
		obj, ok := be.Default.(*js.ObjectExpr)
		if !ok {
			i++
			continue
		}
		pruneObject(obj, path[1:])
		i++
	}
}

func pruneObject(obj *js.ObjectExpr, path []string) {
	key := path[0]

	for i := 0; i < len(obj.List); {
		prop := &obj.List[i]
		if propertyName(*prop) != key {
			i++
			continue
		}

		// Final segment: remove the whole property.
		if len(path) == 1 {
			obj.List = append(obj.List[:i], obj.List[i+1:]...)
			log.Printf("remove-js-constant: removed property %q", key)
			continue
		}

		// Recurse into nested object literal.
		if nested, ok := prop.Value.(*js.ObjectExpr); ok {
			pruneObject(nested, path[1:])
		}
		i++
	}
}

func bindingName(b js.IBinding) string {
	if v, ok := b.(*js.Var); ok {
		return string(v.Data)
	}
	return ""
}

func propertyName(p js.Property) string {
	lit := &p.Name.Literal
	switch lit.TokenType {
	case js.StringToken:
		// strip quotes: "foo" → foo
		if l := len(lit.Data); l >= 2 {
			return string(lit.Data[1 : l-1])
		}
	default: // IdentifierToken, NumericToken, …
	}
	return string(lit.Data)
}
