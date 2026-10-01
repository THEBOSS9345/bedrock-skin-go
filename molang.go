package skinapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// molang is a compiled Molang expression or script: the small expression
// language Bedrock animations use for values that change over time, e.g.
// "math.sin(query.anim_time * 360) * 30". See docs/animation.md#molang for
// what is supported.
type molang struct {
	stmts []mStmt
	src   string
}

// molangEnv is what an expression can read: queries by name (lower case,
// without the "query." prefix) and the script's variables, which it can also
// set.
type molangEnv struct {
	queries   map[string]float64
	variables map[string]float64
}

func (e *molangEnv) get(name string) float64 {
	switch {
	case strings.HasPrefix(name, "query."):
		return e.queries[name[len("query."):]]
	case strings.HasPrefix(name, "variable."), strings.HasPrefix(name, "temp."), strings.HasPrefix(name, "context."):
		return e.variables[name]
	}
	return 0
}

// compileMolang parses src. A number or empty string is a constant.
func compileMolang(src string) (*molang, error) {
	p := &mParser{src: src}
	if err := p.lex(); err != nil {
		return nil, err
	}
	m := &molang{src: src}
	for !p.at(mEOF, "") {
		if p.at(mPunct, ";") {
			p.next()
			continue
		}
		st, err := p.statement()
		if err != nil {
			return nil, err
		}
		m.stmts = append(m.stmts, st)
		if !p.at(mEOF, "") && !p.at(mPunct, ";") {
			return nil, p.errorf("expected ; or the end")
		}
	}
	return m, nil
}

// eval runs the expression. A single expression is its own value; a script
// of statements is the value of its return statement, or 0.
func (m *molang) eval(env *molangEnv) float64 {
	if m == nil {
		return 0
	}
	if len(m.stmts) == 1 && m.stmts[0].kind == mExprStmt {
		return finite(m.stmts[0].expr.eval(env))
	}
	for _, st := range m.stmts {
		switch st.kind {
		case mAssign:
			env.variables[st.name] = finite(st.expr.eval(env))
		case mReturn:
			return finite(st.expr.eval(env))
		case mExprStmt:
			st.expr.eval(env)
		}
	}
	return 0
}

// finite keeps a NaN or infinity (a division by zero, say) from reaching the
// renderer, where it would make a vertex vanish.
func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// ---- lexing ----

type mTokKind int

const (
	mEOF mTokKind = iota
	mNum
	mIdent
	mPunct
)

type mTok struct {
	kind mTokKind
	text string
	num  float64
	pos  int
}

type mParser struct {
	src  string
	toks []mTok
	i    int
}

func (p *mParser) errorf(format string, args ...any) error {
	pos := len(p.src)
	if p.i < len(p.toks) {
		pos = p.toks[p.i].pos
	}
	return fmt.Errorf("molang %q at %d: %s", p.src, pos, fmt.Sprintf(format, args...))
}

func (p *mParser) lex() error {
	s := p.src
	for i := 0; i < len(s); {
		c := rune(s[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsDigit(c) || c == '.' && i+1 < len(s) && unicode.IsDigit(rune(s[i+1])):
			j := i
			for j < len(s) && (unicode.IsDigit(rune(s[j])) || s[j] == '.') {
				j++
			}
			if j < len(s) && (s[j] == 'f' || s[j] == 'F') { // 1.5f, as some files write
				j++
			}
			v, err := strconv.ParseFloat(strings.TrimRight(s[i:j], "fF"), 64)
			if err != nil {
				return fmt.Errorf("molang %q at %d: bad number %q", s, i, s[i:j])
			}
			p.toks = append(p.toks, mTok{kind: mNum, num: v, pos: i})
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(s) && (unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || s[j] == '_' || s[j] == '.') {
				j++
			}
			p.toks = append(p.toks, mTok{kind: mIdent, text: normalizeName(s[i:j]), pos: i})
			i = j
		default:
			two := ""
			if i+1 < len(s) {
				two = s[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||", "??", "->":
				p.toks = append(p.toks, mTok{kind: mPunct, text: two, pos: i})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/()<>!?:,;=", c) {
				p.toks = append(p.toks, mTok{kind: mPunct, text: string(c), pos: i})
				i++
				continue
			}
			return fmt.Errorf("molang %q at %d: unexpected %q", s, i, string(c))
		}
	}
	p.toks = append(p.toks, mTok{kind: mEOF, pos: len(s)})
	return nil
}

// normalizeName lower-cases a name and expands Molang's short prefixes (q.,
// v., t., c.), so "Math.Cos" and "math.cos", "q.anim_time" and
// "query.anim_time" are one name each.
func normalizeName(s string) string {
	s = strings.ToLower(s)
	for short, long := range map[string]string{"q.": "query.", "v.": "variable.", "t.": "temp.", "c.": "context."} {
		if strings.HasPrefix(s, short) {
			return long + s[len(short):]
		}
	}
	return s
}

func (p *mParser) peek() mTok { return p.toks[p.i] }
func (p *mParser) next() mTok { t := p.toks[p.i]; p.i++; return t }
func (p *mParser) at(k mTokKind, text string) bool {
	t := p.peek()
	return t.kind == k && (text == "" || t.text == text)
}

// ---- parsing ----

type mStmtKind int

const (
	mExprStmt mStmtKind = iota
	mAssign
	mReturn
)

type mStmt struct {
	kind mStmtKind
	name string // for mAssign
	expr mExpr
}

func (p *mParser) statement() (mStmt, error) {
	if p.at(mIdent, "return") {
		p.next()
		e, err := p.expr()
		return mStmt{kind: mReturn, expr: e}, err
	}
	if p.at(mIdent, "") && p.toks[p.i+1].kind == mPunct && p.toks[p.i+1].text == "=" {
		name := p.next().text
		p.next()
		e, err := p.expr()
		return mStmt{kind: mAssign, name: name, expr: e}, err
	}
	e, err := p.expr()
	return mStmt{kind: mExprStmt, expr: e}, err
}

// Precedence, loosest first: ?? then ?: then || && then comparisons, + -,
// * /, unary.
func (p *mParser) expr() (mExpr, error) { return p.coalesce() }

func (p *mParser) coalesce() (mExpr, error) {
	l, err := p.ternary()
	if err != nil {
		return nil, err
	}
	for p.at(mPunct, "??") {
		p.next()
		r, err := p.ternary()
		if err != nil {
			return nil, err
		}
		l = mBinary{"??", l, r}
	}
	return l, nil
}

func (p *mParser) ternary() (mExpr, error) {
	cond, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if !p.at(mPunct, "?") {
		return cond, nil
	}
	p.next()
	yes, err := p.ternary()
	if err != nil {
		return nil, err
	}
	// "a ? b" with no ":" is a conditional that is 0 when false.
	var no mExpr = mNumber(0)
	if p.at(mPunct, ":") {
		p.next()
		if no, err = p.ternary(); err != nil {
			return nil, err
		}
	}
	return mTernary{cond, yes, no}, nil
}

var mLevels = [][]string{{"||"}, {"&&"}, {"==", "!="}, {"<", ">", "<=", ">="}, {"+", "-"}, {"*", "/"}}

func (p *mParser) binary(level int) (mExpr, error) {
	if level == len(mLevels) {
		return p.unary()
	}
	l, err := p.binary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		matched := false
		for _, op := range mLevels[level] {
			if t.kind == mPunct && t.text == op {
				matched = true
			}
		}
		if !matched {
			return l, nil
		}
		p.next()
		r, err := p.binary(level + 1)
		if err != nil {
			return nil, err
		}
		l = mBinary{t.text, l, r}
	}
}

func (p *mParser) unary() (mExpr, error) {
	if p.at(mPunct, "-") || p.at(mPunct, "!") || p.at(mPunct, "+") {
		op := p.next().text
		e, err := p.unary()
		if err != nil {
			return nil, err
		}
		return mUnary{op, e}, nil
	}
	return p.primary()
}

func (p *mParser) primary() (mExpr, error) {
	t := p.next()
	switch {
	case t.kind == mNum:
		return mNumber(t.num), nil
	case t.kind == mPunct && t.text == "(":
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		if !p.at(mPunct, ")") {
			return nil, p.errorf("expected )")
		}
		p.next()
		return e, nil
	case t.kind == mIdent:
		switch t.text {
		case "true":
			return mNumber(1), nil
		case "false":
			return mNumber(0), nil
		case "math.pi":
			return mNumber(math.Pi), nil
		}
		if p.at(mPunct, "(") {
			p.next()
			var args []mExpr
			for !p.at(mPunct, ")") {
				a, err := p.expr()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if p.at(mPunct, ",") {
					p.next()
				} else if !p.at(mPunct, ")") {
					return nil, p.errorf("expected , or )")
				}
			}
			p.next()
			if strings.HasPrefix(t.text, "math.") {
				if _, ok := mathFuncs[t.text]; !ok {
					return nil, p.errorf("unknown function %s", t.text)
				}
			}
			return mCall{t.text, args}, nil
		}
		return mName(t.text), nil
	}
	p.i--
	return nil, p.errorf("unexpected %q", t.text)
}

// ---- evaluation ----

type mExpr interface{ eval(env *molangEnv) float64 }

type (
	mNumber float64
	mName   string
	mUnary  struct {
		op string
		e  mExpr
	}
	mBinary struct {
		op   string
		l, r mExpr
	}
	mTernary struct{ cond, yes, no mExpr }
	mCall    struct {
		name string
		args []mExpr
	}
)

func (n mNumber) eval(*molangEnv) float64   { return float64(n) }
func (n mName) eval(env *molangEnv) float64 { return env.get(string(n)) }

func truth(v float64) bool { return v != 0 }
func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (u mUnary) eval(env *molangEnv) float64 {
	v := u.e.eval(env)
	switch u.op {
	case "-":
		return -v
	case "!":
		return b2f(!truth(v))
	}
	return v
}

func (b mBinary) eval(env *molangEnv) float64 {
	switch b.op { // short-circuiting
	case "&&":
		return b2f(truth(b.l.eval(env)) && truth(b.r.eval(env)))
	case "||":
		return b2f(truth(b.l.eval(env)) || truth(b.r.eval(env)))
	case "??":
		// Every name here has a value (unknown ones are 0), so the left
		// side always stands.
		return b.l.eval(env)
	}
	l, r := b.l.eval(env), b.r.eval(env)
	switch b.op {
	case "+":
		return l + r
	case "-":
		return l - r
	case "*":
		return l * r
	case "/":
		if r == 0 {
			return 0
		}
		return l / r
	case "==":
		return b2f(l == r)
	case "!=":
		return b2f(l != r)
	case "<":
		return b2f(l < r)
	case ">":
		return b2f(l > r)
	case "<=":
		return b2f(l <= r)
	case ">=":
		return b2f(l >= r)
	}
	return 0
}

func (t mTernary) eval(env *molangEnv) float64 {
	if truth(t.cond.eval(env)) {
		return t.yes.eval(env)
	}
	return t.no.eval(env)
}

func (c mCall) eval(env *molangEnv) float64 {
	args := make([]float64, len(c.args))
	for i, a := range c.args {
		args[i] = a.eval(env)
	}
	if f, ok := mathFuncs[c.name]; ok {
		return f(args)
	}
	return 0 // a query function scout does not model
}

const deg = math.Pi / 180

func arg(a []float64, i int) float64 {
	if i < len(a) {
		return a[i]
	}
	return 0
}

// mathFuncs are Molang's math functions. Trigonometry is in degrees, as in
// Bedrock. random is the middle of its range, so a render is repeatable.
var mathFuncs = map[string]func([]float64) float64{
	"math.sin":   func(a []float64) float64 { return math.Sin(arg(a, 0) * deg) },
	"math.cos":   func(a []float64) float64 { return math.Cos(arg(a, 0) * deg) },
	"math.asin":  func(a []float64) float64 { return math.Asin(arg(a, 0)) / deg },
	"math.acos":  func(a []float64) float64 { return math.Acos(arg(a, 0)) / deg },
	"math.atan":  func(a []float64) float64 { return math.Atan(arg(a, 0)) / deg },
	"math.atan2": func(a []float64) float64 { return math.Atan2(arg(a, 0), arg(a, 1)) / deg },
	"math.abs":   func(a []float64) float64 { return math.Abs(arg(a, 0)) },
	"math.ceil":  func(a []float64) float64 { return math.Ceil(arg(a, 0)) },
	"math.floor": func(a []float64) float64 { return math.Floor(arg(a, 0)) },
	"math.round": func(a []float64) float64 { return math.Round(arg(a, 0)) },
	"math.trunc": func(a []float64) float64 { return math.Trunc(arg(a, 0)) },
	"math.sqrt":  func(a []float64) float64 { return math.Sqrt(arg(a, 0)) },
	"math.exp":   func(a []float64) float64 { return math.Exp(arg(a, 0)) },
	"math.ln":    func(a []float64) float64 { return math.Log(arg(a, 0)) },
	"math.pow":   func(a []float64) float64 { return math.Pow(arg(a, 0), arg(a, 1)) },
	"math.mod": func(a []float64) float64 {
		if arg(a, 1) == 0 {
			return 0
		}
		return math.Mod(arg(a, 0), arg(a, 1))
	},
	"math.min":   func(a []float64) float64 { return math.Min(arg(a, 0), arg(a, 1)) },
	"math.max":   func(a []float64) float64 { return math.Max(arg(a, 0), arg(a, 1)) },
	"math.clamp": func(a []float64) float64 { return math.Max(arg(a, 1), math.Min(arg(a, 2), arg(a, 0))) },
	"math.lerp":  func(a []float64) float64 { return arg(a, 0) + (arg(a, 1)-arg(a, 0))*arg(a, 2) },
	"math.lerprotate": func(a []float64) float64 {
		from, to := arg(a, 0), arg(a, 1)
		d := math.Mod(to-from+540, 360) - 180
		return from + d*arg(a, 2)
	},
	"math.hermite_blend":    func(a []float64) float64 { t := arg(a, 0); return 3*t*t - 2*t*t*t },
	"math.random":           func(a []float64) float64 { return (arg(a, 0) + arg(a, 1)) / 2 },
	"math.random_integer":   func(a []float64) float64 { return math.Round((arg(a, 0) + arg(a, 1)) / 2) },
	"math.die_roll":         func(a []float64) float64 { return arg(a, 0) * (arg(a, 1) + arg(a, 2)) / 2 },
	"math.die_roll_integer": func(a []float64) float64 { return math.Round(arg(a, 0) * (arg(a, 1) + arg(a, 2)) / 2) },
	"math.min_angle": func(a []float64) float64 {
		v := math.Mod(arg(a, 0), 360)
		if v >= 180 {
			v -= 360
		} else if v < -180 {
			v += 360
		}
		return v
	},
}
