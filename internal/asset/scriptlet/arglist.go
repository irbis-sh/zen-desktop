package scriptlet

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// argList is the canonical form of a scriptlet's argument list, excluding the
// function call expression: the decoded arguments, each JSON-encoded, joined
// with commas. JSON string literals are valid JS string literals, so the value
// can be embedded verbatim in generated code and the browser recovers the
// decoded arguments byte-exactly. Because rules from both supported syntaxes
// are decoded before encoding, equal argument lists compare equal regardless
// of the syntax or quote style they were written in, which is what exception
// rule matching relies on.
type argList string

// newArgList encodes decoded scriptlet arguments into their canonical form.
func newArgList(args []string) (argList, error) {
	encoded := make([]string, len(args))
	for i, arg := range args {
		// json.Marshal escapes everything that could terminate the string or
		// the surrounding script early, including quotes, backslashes, control
		// characters and angle brackets. It cannot fail for strings today, but
		// arguments come from untrusted filter lists, so treat a failure as a
		// bad rule rather than assuming this holds across Go releases
		// (encoding/json/v2 already rejects invalid UTF-8, for one).
		b, err := json.Marshal(arg)
		if err != nil {
			return "", fmt.Errorf("marshal argument %d: %v", i, err)
		}
		encoded[i] = string(b)
	}
	return argList(strings.Join(encoded, ",")), nil
}

func (al argList) GenerateInjection(w io.Writer) error {
	_, err := fmt.Fprintf(w, `try{scriptlet(%s)}catch(ex){console.error(ex);}`, al)
	return err
}
