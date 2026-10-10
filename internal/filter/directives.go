package filter

import (
	"errors"
	"fmt"
	"strings"
)

// declared reports whether Zen identifies as the given product or platform in
// !#if conditions.
//
// Zen declares adguard, which every AdGuard product declares. Lists use it to
// guard rules in AdGuard syntax, which Zen parses: the Polish list, for one,
// includes its AdGuard supplement under !#if adguard.
//
// Zen declares no platform. Lists mostly use these to swap rules for platforms
// with small rule budgets (iOS, Safari, Android content blocker), often trading
// a cosmetic fix for an allowlist entry that lets the ads load. AdGuard's
// desktop apps filter through a proxy as Zen does, but the rules written for
// them mostly need $redirect, $replace or HTML filtering. Declaring as one
// would also drop the browser-extension rules that Zen can run. Revisit once
// those features exist.
//
// The AdGuard Base and Spyware lists are loaded differently: their unbuilt
// versions mark platform rules with !+ PLATFORM hints, which only AdGuard's
// compiler applies, so Zen has to load one of AdGuard's compiled builds. It
// loads the one for the Windows app, trading a few extension rules Zen can run
// today for the desktop rules it will run once it has the features above; see
// the v0.27.0 config migration.
func declared(id string) bool { return id == "adguard" }

// directives applies the !#if/!#else/!#endif blocks of one list stream.
//
// !+ PLATFORM hints are left as comments. Only AdGuard's compiler applies
// them, and Zen loads AdGuard's lists already built for its Windows app.
type directives struct {
	// appliedBlocks counts the open !#if blocks whose lines apply.
	// skippedBlocks counts the rest: the outermost open block whose branch is
	// false, and every block nested in it, since those are skipped too. The
	// current line applies when skippedBlocks is 0.
	appliedBlocks, skippedBlocks int
}

// skip reports whether line must not reach the parser: it is a directive, or
// it sits in a branch whose condition is false. A malformed !#if condition is
// returned as an error and counts as false, so its !#else branch applies.
func (d *directives) skip(line string) (bool, error) {
	switch {
	case isDirective(line, "!#if"):
		// Evaluated even under a skipped parent, so a malformed condition is
		// reported wherever it is.
		v, err := evalCondition(line[len("!#if"):])
		// Counted even under a skipped parent, or this block's !#endif would
		// close the parent.
		if d.skippedBlocks > 0 || !v {
			d.skippedBlocks++
		} else {
			d.appliedBlocks++
		}
		return true, err
	case isDirective(line, "!#else"):
		switch {
		case d.skippedBlocks == 1:
			// The innermost block was the first false one, so its parent
			// applies and so does its else branch.
			d.skippedBlocks = 0
			d.appliedBlocks++
		case d.skippedBlocks == 0 && d.appliedBlocks > 0:
			d.appliedBlocks--
			d.skippedBlocks = 1
		}
		// Otherwise the parent is skipped, so the else branch is too, or no
		// block is open.
		return true, nil
	case isDirective(line, "!#endif"):
		if d.skippedBlocks > 0 {
			d.skippedBlocks--
		} else if d.appliedBlocks > 0 {
			d.appliedBlocks--
		}
		return true, nil
	}
	return d.skippedBlocks > 0, nil
}

// open returns the number of !#if blocks not yet closed by an !#endif.
func (d *directives) open() int { return d.appliedBlocks + d.skippedBlocks }

// isDirective reports whether line starts with the directive name. Text after
// the name is allowed, as AdGuard allows "!#endif ! iOS only": a missed
// !#endif would hide the rest of the list. The name must not run on into a
// word, so that comments like "!#iframe" are not directives.
func isDirective(line, name string) bool {
	rest, ok := strings.CutPrefix(line, name)
	return ok && (rest == "" || !isIdentByte(rest[0]))
}

// maxConditionDepth caps the nesting of brackets and negations so a hostile
// list can't drive the recursion deep. Real conditions nest 2 levels at most.
const maxConditionDepth = 32

// evalCondition evaluates the condition of an !#if directive:
//
//	or    = and { "||" and }
//	and   = unary { "&&" unary }
//	unary = "!" unary | "(" or ")" | identifier
func evalCondition(expr string) (bool, error) {
	p := condParser{s: expr}
	v, err := p.or()
	p.skipSpaces()
	if err == nil && p.i < len(p.s) {
		err = errors.New("unexpected trailing input")
	}
	if err != nil {
		return false, fmt.Errorf("condition %q at offset %d: %w", expr, p.i, err)
	}
	return v, nil
}

type condParser struct {
	s     string
	i     int
	depth int
}

func (p *condParser) or() (bool, error) {
	v, err := p.and()
	for err == nil && p.consume("||") {
		var r bool
		r, err = p.and()
		v = v || r
	}
	return v, err
}

func (p *condParser) and() (bool, error) {
	v, err := p.unary()
	for err == nil && p.consume("&&") {
		var r bool
		r, err = p.unary()
		v = v && r
	}
	return v, err
}

func (p *condParser) unary() (bool, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxConditionDepth {
		return false, errors.New("nested too deep")
	}

	switch {
	case p.consume("!"):
		v, err := p.unary()
		return !v, err
	case p.consume("("):
		v, err := p.or()
		if err == nil && !p.consume(")") {
			err = errors.New("missing )")
		}
		return v, err
	}

	p.skipSpaces()
	start := p.i
	for p.i < len(p.s) && isIdentByte(p.s[p.i]) {
		p.i++
	}
	if p.i == start {
		return false, errors.New("expected identifier")
	}
	return declared(p.s[start:p.i]), nil
}

func (p *condParser) skipSpaces() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func (p *condParser) consume(tok string) bool {
	p.skipSpaces()
	if strings.HasPrefix(p.s[p.i:], tok) {
		p.i += len(tok)
		return true
	}
	return false
}

func isIdentByte(c byte) bool {
	return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}
