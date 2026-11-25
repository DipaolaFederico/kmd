package tex

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/styled"
	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

func text(l styled.Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

func block(ls []styled.Line) string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = text(l)
	}
	return strings.Join(out, "\n")
}

func TestInline(t *testing.T) {
	tests := []struct{ src, want string }{
		// letters, numbers, operators and spacing
		{`a+b=c`, "a + b = c"},
		{`-x`, "−x"},
		{`a - -b`, "a − −b"},
		{`3.14 \cdot 2`, "3.14 ⋅ 2"},
		{`f(x, y)`, "f(x, y)"},
		{`\alpha \beta \Gamma \Omega \varepsilon \aleph`, "αβΓΩεℵ"},
		{`x \le y \neq z`, "x ≤ y ≠ z"},
		{`A \subseteq B \cup C`, "A ⊆ B ∪ C"},
		{`\forall x \exists y: x \to y`, "∀x∃y : x → y"},
		{`p \implies q \iff r`, "p ⟹ q ⟺ r"},
		{`\neg p \land q`, "¬p ∧ q"},
		{`1, 2, \ldots, n`, "1, 2, …, n"},
		{`\{ x \mid x > 0 \}`, "{x ∣ x > 0}"},
		{`\langle u, v \rangle`, "⟨u, v⟩"},
		{`\lfloor x \rfloor + \lceil y \rceil`, "⌊x⌋ + ⌈y⌉"},
		{`\|v\|`, "‖v‖"},
		{`\# \$ \% \& \_`, "#$%&_"},
		{`n!`, "n!"},
		{`a \times b \div c \pm d`, "a × b ÷ c ± d"},
		{`x \in \mathbb{R}, \quad x > 0`, "x ∈ ℝ,  x > 0"},
		{`a\,b\;c\:d\!e\ f~g \qquad h`, "a b c de f g    h"},
		// scripts
		{`x^2`, "x²"},
		{`x^{n+1}`, "xⁿ⁺¹"},
		{`x_i`, "xᵢ"},
		{`x_{ij}^{2}`, "xᵢⱼ²"},
		{`a_{n-1}`, "aₙ₋₁"},
		{`10^{-3}`, "10⁻³"},
		{`e^{-x^2}`, "e⁻ˣ²"},
		{`e^{i\pi}`, "e^(iπ)"},
		{`x^\infty`, "x^∞"},
		{`x_{\text{max}}`, "xₘₐₓ"},
		{`A^T`, "Aᵀ"},
		{`f'(x) + g''(x)`, "f′(x) + g″(x)"},
		{`f'^2`, "f′²"},
		{`90^\circ`, "90°"},
		{`x^{(n)}`, "x⁽ⁿ⁾"},
		{`a_{i,j}`, "a_(i,j)"},
		{`^2`, "²"},
		// fractions
		{`\frac{a}{b}`, "a/b"},
		{`\frac{1}{2}`, "½"},
		{`\tfrac34`, "¾"},
		{`\frac{12}{35}`, "¹²⁄₃₅"},
		{`\frac{a+1}{b}`, "(a + 1)/b"},
		{`\frac{a}{b+c}`, "a/(b + c)"},
		{`\frac{n(n+1)}{2}`, "n(n + 1)/2"},
		{`\dfrac{2a}{3b}`, "2a/(3b)"},
		{`\frac\pi2`, "π/2"},
		{`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, "x = (−b ± √(b² − 4ac))/(2a)"},
		{`2\frac{1}{2}`, "2½"},
		{`\binom{n}{k}`, "(ⁿₖ)"},
		{`\binom{n+1}{q}`, "C(n+1, q)"},
		// roots
		{`\sqrt{x}`, "√x"},
		{`\sqrt2`, "√2"},
		{`\sqrt{x+1}`, "√(x + 1)"},
		{`\sqrt[3]{x}`, "∛x"},
		{`\sqrt[4]{x}`, "∜x"},
		{`\sqrt[2]{x}`, "√x"},
		{`\sqrt[n]{x}`, "ⁿ√x"},
		{`\sqrt[\alpha+q]{x}`, "(α+q)√x"},
		// big operators and functions
		{`\sum_{i=1}^{n} i`, "∑ᵢ₌₁ⁿ i"},
		{`\prod_{k} a_k`, "∏ₖ aₖ"},
		{`\int_0^1 f(x)\,dx`, "∫₀¹ f(x) dx"},
		{`\int_0^\infty e^{-x}\,dx`, "∫₀^∞ e⁻ˣ dx"},
		{`\oint \iint \iiint`, "∮ ∬ ∭"},
		{`\lim_{x \to 0} \frac{\sin x}{x} = 1`, "lim_(x→0) (sin x)/x = 1"},
		{`\sin^2\theta + \cos^2\theta = 1`, "sin² θ + cos² θ = 1"},
		{`\log_2 n`, "log₂ n"},
		{`\ln(x)`, "ln(x)"},
		{`2\sin x`, "2 sin x"},
		{`\limsup_{n} a_n`, "lim supₙ aₙ"},
		{`\max_{x} f`, "maxₓ f"},
		{`\operatorname{sgn}(x)`, "sgn(x)"},
		{`\operatorname*{argmax}_\theta L`, "argmax_θ L"},
		{`\det(A - \lambda I) = 0`, "det(A − λI) = 0"},
		{`a \equiv b \pmod{n}`, "a ≡ b (mod n)"},
		{`a \bmod b`, "a mod b"},
		// fonts
		{`\mathbb{RZNQC}`, "ℝℤℕℚℂ"},
		{`\mathbb{A1}`, "𝔸𝟙"},
		{`\mathcal{L} \mathcal{A}`, "ℒ𝒜"},
		{`\mathfrak{g}`, "𝔤"},
		{`\mathbf{v} + \boldsymbol{x}`, "𝐯 + 𝒙"},
		{`\mathrm{d}x`, "dx"},
		{`\mathsf{A}\mathtt{b}`, "𝖠𝚋"},
		{`{\bf x} y`, "𝐱y"},
		{`\text{if } x \text{ is odd}`, "if x is odd"},
		{`\textbf{ok}`, "𝐨𝐤"},
		{`\text{a\{b\}c\,d}`, "a{b}c d"},
		// accents
		{`\hat{x}`, "x̂"},
		{`\vec{v}`, "v⃗"},
		{`\bar x`, "x̄"},
		{`\dot{x}\ddot{y}`, "x\u0307y\u0308"},
		{`\overline{AB}`, "A̅B̅"},
		{`\underline{ab}`, "a̲b̲"},
		{`\widehat{xy}`, "xy\u0302"},
		{`\cancel{ab}`, "a̶b̶"},
		// delimiters
		{`\left( \frac{a}{b} \right)`, "(a/b)"},
		{`\left. \frac{df}{dx} \right|_{x=0}`, "df/(dx)|ₓ₌₀"},
		{`\left\langle x \right\rangle`, "⟨x⟩"},
		{`\bigl( x \bigr)`, "(x)"},
		{`\Big[ x \Big]`, "[x]"},
		{`\left( a \middle| b \right)`, "(a | b)"},
		// environments
		{`\begin{pmatrix} a & b \\ c & d \end{pmatrix}`, "(a b; c d)"},
		{`\begin{bmatrix} 1 & 0 \\ 0 & 1 \end{bmatrix}`, "[1 0; 0 1]"},
		{`\begin{matrix} 1 & 2 \end{matrix}`, "[1 2]"},
		{`\begin{vmatrix} a & b \\ c & d \end{vmatrix}`, "|a b; c d|"},
		{`\begin{Vmatrix} a \end{Vmatrix}`, "‖a‖"},
		{`\begin{Bmatrix} a \end{Bmatrix}`, "{a}"},
		{`|x| = \begin{cases} x & x \ge 0 \\ -x & x < 0 \end{cases}`, "|x| = {x, x ≥ 0; −x, x < 0"},
		{`\begin{aligned} a &= b \\ c &= d \end{aligned}`, "a = b; c = d"},
		{`a &= b \\ c &= d`, "a = b; c = d"},
		{`a \\ b`, "a; b"},
		{`\begin{array}{cc} 1 & 2 \end{array}`, "1 2"},
		{`\sum_{\substack{i<n \\ j<m}} a_{ij}`, "∑_(i<n, j<m) aᵢⱼ"},
		// stacking
		{`\overset{!}{=}`, "=^!"},
		{`\underset{x}{\arg\min} f`, "arg minₓ f"},
		{`a \xrightarrow{f} b`, "a →ᶠ b"},
		{`\overbrace{a+b}^{n}`, "(a + b)^n"},
		{`\underbrace{a+b}_{\text{sum}}`, "(a + b)_(sum)"},
		// misc
		{`\not= \not\in \not\subset \not\approx \not\vdash`, "≠∉⊄≉⊬"},
		{`\boxed{x}`, "x"},
		{`\phantom{ab}c`, "  c"},
		{`\color{red} x \textcolor{blue}{y}`, "xy"},
		{`\displaystyle x \label{eq} \nonumber`, "x"},
		{`E = mc^2 \tag{1}`, "E = mc²  (1)"},
		{`\eqref{a}`, "(a)"},
		{`x % comment`, "x"},
		// unknown commands and malformed input
		{`\foo`, `\foo`},
		{`\foo{x}{y} + 1`, `\foo{x}{y} + 1`},
		{`\frac{a`, "a/"},
		{`}{`, ""},
		{`^^`, ""},
		{`x^`, "x"},
		{`\left(`, "("},
		{`\right)`, ""},
		{`\end{x} a`, "a"},
		{`\sqrt`, "√"},
		{`\`, `\`},
		{"a\x1b[31mb", "a[31mb"},
		{"\\\x05x", `\x`},
		{"\\\x000", `\0`},
		{"", ""},
	}
	for _, tt := range tests {
		if got := text(Inline(tt.src)); got != tt.want {
			t.Errorf("Inline(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestInlineClasses(t *testing.T) {
	l := Inline(`\sin x + 2 = \alpha \foo \text{ok} \sum`)
	want := []styled.Span{
		{Text: "sin", Class: styled.Func}, {Text: " "}, {Text: "x", Class: styled.Var},
		{Text: " "}, {Text: "+", Class: styled.Op}, {Text: " "}, {Text: "2", Class: styled.Num},
		{Text: " "}, {Text: "=", Class: styled.Op}, {Text: " "}, {Text: "α", Class: styled.Var},
		{Text: `\foo`, Class: styled.Muted}, {Text: "ok "}, {Text: "∑", Class: styled.Symbol},
	}
	if len(l) != len(want) {
		t.Fatalf("got %d spans %q, want %d", len(l), l, len(want))
	}
	for i := range want {
		if l[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, l[i], want[i])
		}
	}
}

func TestDisplay(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"quadratic", `x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, `
           ________
     −b ± √b² − 4ac
x = ────────────────
           2a       `},
		{"sum", `\sum_{i=1}^{n} i = \frac{n(n+1)}{2}`, `
 n       n(n + 1)
 ∑  i = ──────────
i=1         2     `},
		{"integral", `\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt\pi}{2}`, `
               _
 ∞            √π
∫  e⁻ˣ² dx = ────
 0            2  `},
		{"pmatrix", `A = \begin{pmatrix} 1 & 2 \\ 3 & 4 \end{pmatrix}`, `
A = ⎛ 1  2 ⎞
    ⎝ 3  4 ⎠`},
		{"bmatrix3", `\begin{bmatrix} a & b & c \\ d & e & f \\ g & h & i \end{bmatrix}`, `
⎡ a  b  c ⎤
⎢ d  e  f ⎥
⎣ g  h  i ⎦`},
		{"cases", `f(x) = \begin{cases} x & \text{if } x \ge 0 \\ -x & \text{otherwise} \end{cases}`, `
f(x) = ⎧ x   if x ≥ 0
       ⎩ −x  otherwise`},
		{"cases3", `\begin{cases} 1 & a \\ 2 & b \\ 3 & c \end{cases}`, `
⎧ 1  a
⎨ 2  b
⎩ 3  c`},
		{"euler", `e^{i\pi} + 1 = 0`, `
 iπ
e   + 1 = 0`},
		{"nested", `\frac{1}{1 + \frac{1}{x}}`, `
    1
─────────
      1
 1 + ───
      x  `},
		{"limit", `\lim_{x \to 0} \frac{\sin x}{x} = 1`, `
     sin x
lim ─────── = 1
x→0    x       `},
		{"tall base", `\left( \frac{a}{b} \right)^2`, `
⎛ a ⎞2
⎜───⎟
⎝ b ⎠ `},
		{"roots", `\sqrt[3]{x} + \sqrt[n]{\frac{a}{b}}`, `
     n ┌───
 _     │ a
∛x +   │───
      ╲│ b `},
		{"binom", `\binom{n}{k}`, `
⎛n⎞
⎜ ⎟
⎝k⎠`},
		{"aligned", `\begin{aligned} a &= b + c \\ dd &= e \end{aligned}`, `
 a = b + c
dd = e    `},
		{"array", `\begin{array}{lr} a & bbb \\ cccc & d \end{array}`, `
a     bbb
cccc    d`},
		{"scripts", `x^{\frac{1}{2}} + y_{a,b}`, `
 ½
x  + y
      a,b  `},
		{"both scripts", `x_{a,b}^{c,d}`, `
 c,d
x
 a,b`},
		{"product limits", `\prod_{p \text{ prime}} \frac{1}{1-p^{-s}}`, `
            1
   ∏    ─────────
p prime  1 − p⁻ˢ `},
		{"accents", `\hat{x} + \overline{AB} + \vec{AB} + \widehat{xyz} + \underline{ab}`, `
    __   ─→    ^
x̂ + AB + AB + xyz + ab
                    ‾‾`},
		{"boxed", `\boxed{E = mc^2}`, `
┌─────────┐
│ E = mc² │
└─────────┘`},
		{"braces", `\overbrace{a+b+c}^{3} = \underbrace{x+y}_{\text{two}}`, `
    3
╭───┴───╮
a + b + c = x + y
            ╰─┬─╯
             two `},
		{"xarrow", `x \xrightarrow[g]{f} y \xleftarrow{} z`, `
   f
x ──→ y ←─ z
   g         `},
		{"middle", `\left( \frac{1}{2} \middle| x \right)`, `
⎛ 1  │  ⎞
⎜─── │ x⎟
⎝ 2  │  ⎠`},
		{"angle", `\left\langle \frac{a}{b} \right\rangle`, `
╱ a ╲
⟨───⟩
╲ b ╱`},
		{"floor", `\left\lfloor \frac{n}{2} \right\rfloor \left\lceil \frac{n}{2} \right\rceil`, `
⎢ n ⎥⎡ n ⎤
⎢───⎥⎢───⎥
⎣ 2 ⎦⎢ 2 ⎥`},
		{"braces4", `\left\{ \begin{matrix} a \\ b \\ c \\ d \end{matrix} \right\}`, `
⎧ a ⎫
⎨ b ⎬
⎪ c ⎪
⎩ d ⎭`},
		{"bars", `\left| \frac{a}{b} \right\| \left/ \frac{a}{b} \right\backslash`, `
│ a ‖╱ a ╲
│───‖╱───╲
│ b ‖╱ b ╲`},
		{"overset", `A \overset{\text{def}}{=} B`, `
  def
A  =  B`},
		{"underset", `\underset{x}{\arg\min} f(x)`, `
arg min f(x)
   x        `},
		{"substack", `\sum_{\substack{0<i<m \\ 0<j<n}} P(i,j)`, `
  ∑   P(i, j)
0<i<m
0<j<n`},
		{"tall sqrt index", `\sqrt[3]{\frac{a}{b}}`, `
3 ┌───
  │ a
  │───
 ╲│ b`},
		{"inline line", `a + b`, `a + b`},
	}
	for _, tt := range tests {
		d := Display(tt.src, 80)
		checkBlock(t, tt.src, d, 80)
		got := trimLines(block(d))
		want := trimLines(strings.TrimPrefix(tt.want, "\n"))
		if got != want {
			t.Errorf("%s: Display(%q) =\n%s\nwant\n%s", tt.name, tt.src, got, want)
		}
	}
}

// trimLines drops trailing blanks so goldens survive editors; equal
// widths are checked separately.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func TestDisplayTag(t *testing.T) {
	got := block(Display(`E = mc^2 \tag{1}`, 20))
	if want := "E = mc²          (1)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// No room for the tag: it is dropped.
	if got := block(Display(`E = mc^2 \tag{1}`, 9)); got != "E = mc²" {
		t.Errorf("narrow tag: got %q", got)
	}
}

func TestDisplayFallback(t *testing.T) {
	src := `x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`
	got := Display(src, 12)
	want := "x = (−b ± \n√(b² −    \n4ac))/(2a)"
	if block(got) != want {
		t.Errorf("fallback =\n%q\nwant\n%q", block(got), want)
	}
	for _, l := range got {
		if styled.LineWidth(l) != 10 {
			t.Errorf("line %q has width %d", text(l), styled.LineWidth(l))
		}
	}
	// A single word longer than the width is cut.
	if got := block(Display(`\text{abcdef}`, 4)); got != "abcd\nef  " {
		t.Errorf("hard wrap = %q", got)
	}
	// Width below one is treated as one.
	if got := Display(`ab`, 0); len(got) != 2 {
		t.Errorf("zero width = %q", block(got))
	}
}

func TestDisplayEmpty(t *testing.T) {
	for _, src := range []string{"", "  ", `\label{x}`, `{}`} {
		if got := Display(src, 40); got != nil {
			t.Errorf("Display(%q) = %q, want nil", src, block(got))
		}
	}
}

func TestLimits(t *testing.T) {
	inputs := []string{
		strings.Repeat("{", 200) + "x" + strings.Repeat("}", 200),
		strings.Repeat(`\frac{`, 100) + "x",
		strings.Repeat(`\left(`, 100),
		strings.Repeat(`\sqrt`, 100) + "x",
		strings.Repeat(`\hat`, 100) + "x",
		strings.Repeat(`\begin{matrix}`, 100),
		strings.Repeat("x", maxSource+1),
	}
	for _, src := range inputs {
		l := Inline(src)
		if len(l) != 1 || l[0].Class != styled.Muted || l[0].Text != src {
			t.Errorf("Inline(%.20q…) is not the verbatim source", src)
		}
		d := Display(src, 50)
		if len(d) == 0 || d[0][0].Class != styled.Muted {
			t.Errorf("Display(%.20q…) is not the verbatim source", src)
		}
		checkBlock(t, src, d, 50)
	}
	// Moderate nesting still renders.
	src := strings.Repeat(`\frac{1}{`, 10) + "x" + strings.Repeat("}", 10)
	if l := Inline(src); l[0].Class == styled.Muted {
		t.Errorf("10 nested fractions fell back to the source")
	}
	// Control characters never survive, even in the verbatim fallback.
	src = strings.Repeat("{", 100) + "\x1b]0;x\x07"
	for _, s := range Inline(src) {
		if strings.ContainsAny(s.Text, "\x1b\x07") {
			t.Errorf("control character in %q", s.Text)
		}
	}
	// Formulas taller than maxRows are shown linearly, which also bounds
	// the layout work hostile input can cause.
	src = `\left(\begin{matrix}` + strings.Repeat(`a\\`, 150) + `\end{matrix}\right)` + strings.Repeat(`\middle|x`, 100)
	if d := Display(src, 80); len(d) > 30 {
		t.Errorf("tall formula laid out in %d rows", len(d))
	}
	// Repeated scripts do not nest.
	src = "x" + strings.Repeat("^2", 1000) + strings.Repeat("'", 50)
	if got := Inline(src); got[0].Class == styled.Muted {
		t.Errorf("repeated scripts fell back to the source")
	}
}

// TestLinearTime: under maxSource, the time grows linearly with the
// formula, inline and displayed (n repetitions of each pattern).
func TestLinearTime(t *testing.T) {
	for name, unit := range map[string]string{
		"symbols":   `\alpha + \beta \times 2 `,
		"scripts":   "x^2_i ",
		"fractions": `\frac{a}{b} `,
		"roots":     `\sqrt{x} `,
		"fences":    `\left( x \middle| y \right) `,
		"matrix":    `\begin{matrix}a & b \\ c & d\end{matrix} `,
		"text":      `\text{some words} `,
		"unknown":   `\nosuchcommand{x} `,
	} {
		testlimit.Linear(t, name, maxSource/len(unit), func(n int) func() {
			src := strings.Repeat(unit, n)
			return func() {
				_ = Inline(src)
				_ = Display(src, 80)
			}
		})
	}
}

// checkBlock asserts the Display contract: equal widths within width.
func checkBlock(t *testing.T, src string, d []styled.Line, width int) {
	t.Helper()
	for i, l := range d {
		w := styled.LineWidth(l)
		if w > width {
			t.Errorf("Display(%q, %d): line %d is %d wide", src, width, i, w)
		}
		if w != styled.LineWidth(d[0]) {
			t.Errorf("Display(%q, %d): line %d width %d != %d", src, width, i, w, styled.LineWidth(d[0]))
		}
		for _, s := range l {
			for _, r := range s.Text {
				if styled.Hidden(r) {
					t.Errorf("Display(%q): control character %U", src, r)
				}
			}
		}
	}
}

func TestMalformed(t *testing.T) {
	inputs := []string{
		`\frac{a`, `}{`, `^^`, `__x`, `x^`, `\left(`, `\right)`, `\left( a \right`,
		`\begin{pmatrix} a & b`, `\end{pmatrix}`, `\begin{array}`, `\begin{array}[t]{c|c} a & b \end{array}`,
		`\sqrt[`, `\sqrt[3`, `\frac`, `\binom{}{}`, `\text{`, `\text{\`, `\operatorname`,
		`a & & b \\ \\`, `\\[2pt] a`, `{\bf`, `\mathbb`, `\not`, `\not{}`, `\middle|`,
		`\big`, `\bigl\{`, `\overset{a}`, `\xrightarrow[`, `\substack{a`, `\substack x`,
		`\kern-3mu x`, `\mskip 2mu plus 1fil x`, `\hspace*{1em}x`, `\vspace{1pt}`,
		`\tag*{A}`, `\smash[b]{x}`, `\fbox{a}`, `\colorbox{red}{a}`, `\fcolorbox{a}{b}{c}`,
		`\href{u}{t}`, `\mathop{X}_a^b`, `\mathbin{+}\mathrel{=}\mathopen{(}\mathclose{)}\mathpunct{,}\mathinner{x}\mathord{+}`,
		`\sum\limits_a`, `\int\limits_0^1`, `\lim\nolimits_{x}`, `\limits`, `x\limits^2`,
		`\left\uparrow x \right\Downarrow`, `\left\backslash x \right/`, `\left x \right y`,
		`\left< x \right>`, `\Bigg( x \Biggr) \bigm| \Big\langle`, `\pmod`, `\pod{n}`,
		"\xff\xfe", "x\u202ey", `\' \"`, `$x$`, `#`, `\hline`, `x \cr y`, `\newline`,
		`\begin{alignat}{2} a &= b & c &= d \end{alignat}`, `\begin{rcases} a \\ b \end{rcases}`,
		`\begin{unknown} a & b \end{unknown}`, `\begin{gather} a \\ b \end{gather}`,
		`\begin{smallmatrix} a \end{smallmatrix}`, `\begin{split} a &= b \end{split}`,
		`\left\{ \begin{array}{l} a \\ b \end{array} \right.`, `\underbrace{}_{}`, `\overbrace{ab}`,
		`\phantom{\frac{a}{b}}x`, `\hphantom{x}`, `\vphantom{x}y`, `\mathring{a}\breve{a}\check{a}\acute{a}\grave{a}\tilde{a}\dddot{a}`,
		`\widetilde{abc}\overleftarrow{ab}\overleftrightarrow{ab}\utilde{ab}\widecheck{ab}\acute{ab}\grave{ab}\dot{ab}\ddot{ab}\breve{ab}\mathring{ab}\dddot{ab}\bar{ab}`,
		`\xRightarrow{a}\xLeftarrow{a}\xleftrightarrow{a}\xLeftrightarrow{a}\xmapsto{a}\xhookrightarrow{a}\xtwoheadrightarrow{a}\xlongequal{a}`,
		`\rm a \it b \sf c \tt d \cal E \frak F \mit g`, `\mathit{ab}\mathscr{A}\Bbb{N}\bm{x}\mathnormal{x}`,
		`x_{\frac{a}{b}}^{\begin{matrix}1\\2\end{matrix}}`, `\left(\begin{matrix}a\\b\end{matrix}\right)^{x+y}_{i}`,
		`\sqrt{\begin{matrix}a\\b\end{matrix}}^2`, `\frac{\sum_a^b}{c}`, `\int_{\frac{a}{b}}`,
		`\boxed{\frac{a}{b}}`, `\cancel{\frac{a}{b}}`, `\hat{\frac{a}{b}}`, `\lvert x \rvert \lVert y \rVert`,
		`\ref{a}\url{b}\emph{c}\textit{d}\textsf{e}\texttt{f}\mbox{g}`, `\mod \bmod`, `\colon \ldotp \cdotp`,
		`\text{\alpha \unknown x\\y~z\!}`, `\text{` + "\x1b\t" + `}`, `\text{\{}`, `\clap{x}\rlap{y}`,
		`1.5.3`, `.5`, `x.`, `{}^{14}C`, `\mathbb{\alpha}`, `\left.\right.`, `\begin{cases}\end{cases}`,
	}
	for _, src := range inputs {
		_ = Inline(src)
		for _, w := range []int{1, 5, 20, 80} {
			checkBlock(t, src, Display(src, w), max(w, 2))
		}
	}
}

func TestDisplayWidths(t *testing.T) {
	inputs := []string{
		`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, `\sum_{i=1}^{n} i = \frac{n(n+1)}{2}`,
		`\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt\pi}{2}`, `\begin{pmatrix} 1 & 2 \\ 3 & 4 \end{pmatrix}`,
		`\frac{\frac{a}{b}}{\frac{c}{d}}`, `\sqrt{\frac{\sqrt{x}}{y}}`, `漢字 = \frac{漢}{字}`,
		`\begin{bmatrix} \frac{1}{2} & 0 \\ 0 & \sqrt{2} \end{bmatrix}`,
	}
	for _, src := range inputs {
		for w := 1; w <= 60; w++ {
			checkBlock(t, src, Display(src, w), max(w, 2))
		}
	}
}

// TestSymbolTables calls every case of the lookup switches in symbols.go
// and checks each yields a result, so no entry is dead or mistyped.
func TestSymbolTables(t *testing.T) {
	f, err := goparser.ParseFile(token.NewFileSet(), "symbols.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	strFuncs := map[string]func(string) bool{
		"greek":       func(s string) bool { return greek(s) != "" },
		"relation":    func(s string) bool { return relation(s) != "" },
		"binary":      func(s string) bool { return binary(s) != "" },
		"ordinary":    func(s string) bool { return ordinary(s) != "" },
		"fence":       func(s string) bool { g, _ := fence(s); return g != "" },
		"delimGlyph":  func(s string) bool { return delimGlyph(s) != "" },
		"bigOp":       func(s string) bool { _, _, ok := bigOp(s); return ok },
		"function":    func(s string) bool { _, _, ok := function(s); return ok },
		"accentMark":  func(s string) bool { return accentMark(s) != 0 },
		"fontCommand": func(s string) bool { _, ok := fontCommand(s); return ok },
		"switchFont":  func(s string) bool { return switchFont(s) != fIt || s == "it" },
		"xArrow":      func(s string) bool { return xArrow(s) != "" },
		"negate":      func(s string) bool { return !strings.HasSuffix(negate(s), "̸") },
		"stretchArrow": func(s string) bool {
			return utf8.RuneCountInString(stretchArrow(s, 5)) == 5
		},
		"symbol": func(s string) bool { _, _, _, ok := symbol(s); return ok },
		"vulgar": func(s string) bool { n, d, _ := strings.Cut(s, "/"); return vulgar(n, d) != "" },
	}
	runeFuncs := map[string]func(rune) bool{
		"supRune":     func(r rune) bool { return supRune(r) != 0 },
		"subRune":     func(r rune) bool { return subRune(r) != 0 },
		"unicodeType": func(r rune) bool { _, ok := unicodeType(r); return ok },
		"accentStyle": func(r rune) bool { return accentStyle(r) != accOver },
		"accentGlyph": func(r rune) bool { return accentGlyph(r, 3) != "˚" },
	}
	count := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				lit, ok := e.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING && lit.Kind != token.CHAR {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				count++
				name := fn.Name.Name
				if g, ok := strFuncs[name]; ok && lit.Kind == token.STRING {
					if !g(v) {
						t.Errorf("%s(%q) has no result", name, v)
					}
				} else if g, ok := runeFuncs[name]; ok && lit.Kind == token.CHAR {
					if r, _ := utf8.DecodeRuneInString(v); !g(r) {
						t.Errorf("%s(%q) has no result", name, v)
					}
				}
			}
			return true
		})
	}
	if count < 500 {
		t.Errorf("only %d table entries found", count)
	}
	// Each named symbol also renders through the public API, unmuted.
	for _, name := range []string{"alpha", "leq", "pm", "infty", "langle", "hbar", "ell", "imath", "jmath", "lnot", "colon", "#"} {
		if l := Inline(`\` + name); len(l) != 1 || l[0].Class == styled.Muted {
			t.Errorf(`Inline(\%s) = %+v`, name, l)
		}
	}
	if accentGlyph(0x30A, 2) != "˚" || accentStyle(0x302) != accOver {
		t.Error("fallback accent glyph")
	}
}

func TestFonts(t *testing.T) {
	for _, f := range []font{fNone, fBB, fCal, fFrak, fBF, fBI, fIt, fRm, fSf, fTt} {
		for _, r := range "AZaz09αé" {
			m := fontRune(f, r)
			if styled.RuneWidth(m) != 1 {
				t.Errorf("font %d maps %q to %q of width %d", f, r, m, styled.RuneWidth(m))
			}
		}
	}
	if got := mapFont(fBB, "R2"); got != "ℝ𝟚" {
		t.Errorf("mapFont = %q", got)
	}
}

func TestWrapCombining(t *testing.T) {
	l := styled.Line{{Text: "x̂ŷ z"}}
	got := wrap(l, 2)
	if block(got) != "x\u0302y\u0302\nz " {
		t.Errorf("wrap = %q", block(got))
	}
}

// TestWrapForeignMarks: a mark from a later span joins the previous
// character too, up to maxCellBytes.
func TestWrapForeignMarks(t *testing.T) {
	l := styled.Line{{Text: "x"}, {Text: "\u0302y", Class: styled.Muted}, {Text: strings.Repeat("\u0301", 100)}}
	got := wrap(l, 2)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0].Text != "x\u0302" || !strings.HasPrefix(got[0][1].Text, "y\u0301") ||
		len(got[0][1].Text) > maxCellBytes+1 {
		t.Errorf("wrap = %q", got)
	}
}

// TestWrapZeroWidthLinear: the verbatim fallback of a formula over
// maxSource made of combining marks or zero-width spaces is wrapped in
// linear time (each mark used to be appended to its cell by copying it).
func TestWrapZeroWidthLinear(t *testing.T) {
	for name, mark := range map[string]string{"combining": "\u0301", "zero-width space": "\u200b"} {
		src := "a" + strings.Repeat(mark, 160_000)
		start := time.Now()
		if d := Display(src, 80); len(d) != 1 {
			t.Errorf("%s: %d lines", name, len(d))
		}
		if d := time.Since(start); d > testlimit.Scale()*time.Second {
			t.Errorf("%s: %v", name, d)
		}
		testlimit.Linear(t, name, 1<<20, func(n int) func() {
			src := "a" + strings.Repeat(mark, n)
			return func() { _ = Display(src, 80) }
		})
	}
}

func FuzzInline(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		l := Inline(src)
		for _, s := range l {
			if s.Text == "" {
				t.Fatalf("empty span in Inline(%q)", src)
			}
			for _, r := range s.Text {
				if styled.Hidden(r) {
					t.Fatalf("control character %U in Inline(%q)", r, src)
				}
			}
		}
	})
}

func FuzzDisplay(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, 40)
	}
	f.Fuzz(func(t *testing.T, src string, width int) {
		width = 2 + abs(width)%200
		checkBlock(t, src, Display(src, width), width)
	})
}

func abs(x int) int {
	if x < 0 {
		if x = -x; x < 0 {
			return 0
		}
	}
	return x
}

var fuzzSeeds = []string{
	`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`,
	`\sum_{i=1}^{n} i = \frac{n(n+1)}{2}`,
	`\int_0^\infty e^{-x^2}\,dx`,
	`\begin{pmatrix} a & b \\ c & d \end{pmatrix}`,
	`\begin{cases} x & x>0 \\ -x & \text{else} \end{cases}`,
	`\left( \frac{a}{b} \middle| c \right)^2_i`,
	`\sqrt[3]{\frac{1}{2}} \overbrace{a}^{b} \xrightarrow[a]{b}`,
	`\mathbb{R} \hat{x} \boxed{\binom{n}{k}} \foo{x} \text{t}`,
	`\begin{aligned} a &= b \\ &= c \end{aligned} \tag{1}`,
	`}{^^\frac{`,
}

func BenchmarkInline(b *testing.B) {
	for b.Loop() {
		Inline(`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`)
	}
}

func BenchmarkDisplay(b *testing.B) {
	for b.Loop() {
		Display(`x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}`, 80)
	}
}

func BenchmarkDisplayMatrix(b *testing.B) {
	for b.Loop() {
		Display(`A = \begin{pmatrix} \frac{1}{2} & \sqrt{2} & 0 \\ 0 & 1 & \alpha_1 \\ x^2 & y_i & z \end{pmatrix}`, 80)
	}
}
