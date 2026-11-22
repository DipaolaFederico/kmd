package tex

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// All tables are switch statements: they compile to binary searches and,
// unlike package-level maps, cost nothing at program start.

// symbol resolves a symbol command (letters, relations, operators, misc).
func symbol(name string) (string, atomType, styled.Class, bool) {
	if s := greek(name); s != "" {
		return s, tOrd, styled.Var, true
	}
	if s := relation(name); s != "" {
		return s, tRel, styled.Op, true
	}
	if s := binary(name); s != "" {
		return s, tBin, styled.Op, true
	}
	if s := ordinary(name); s != "" {
		return s, tOrd, styled.Symbol, true
	}
	if s, t := fence(name); s != "" {
		return s, t, styled.Symbol, true
	}
	switch name {
	case "colon":
		return ":", tPunct, styled.Plain, true
	case "ldotp":
		return ".", tPunct, styled.Plain, true
	case "cdotp":
		return "⋅", tPunct, styled.Plain, true
	case "hbar", "hslash":
		return "ℏ", tOrd, styled.Var, true
	case "ell":
		return "ℓ", tOrd, styled.Var, true
	case "imath":
		return "ı", tOrd, styled.Var, true
	case "jmath":
		return "ȷ", tOrd, styled.Var, true
	case "neg", "lnot":
		return "¬", tOrd, styled.Op, true
	case "#", "$", "%", "&", "_":
		return name, tOrd, styled.Plain, true
	}
	return "", tOrd, styled.Plain, false
}

func greek(name string) string {
	switch name {
	case "alpha":
		return "α"
	case "beta":
		return "β"
	case "gamma":
		return "γ"
	case "delta":
		return "δ"
	case "epsilon":
		return "ϵ"
	case "varepsilon":
		return "ε"
	case "zeta":
		return "ζ"
	case "eta":
		return "η"
	case "theta":
		return "θ"
	case "vartheta":
		return "ϑ"
	case "iota":
		return "ι"
	case "kappa":
		return "κ"
	case "varkappa":
		return "ϰ"
	case "lambda":
		return "λ"
	case "mu":
		return "μ"
	case "nu":
		return "ν"
	case "xi":
		return "ξ"
	case "omicron":
		return "ο"
	case "pi":
		return "π"
	case "varpi":
		return "ϖ"
	case "rho":
		return "ρ"
	case "varrho":
		return "ϱ"
	case "sigma":
		return "σ"
	case "varsigma":
		return "ς"
	case "tau":
		return "τ"
	case "upsilon":
		return "υ"
	case "phi":
		return "ϕ"
	case "varphi":
		return "φ"
	case "chi":
		return "χ"
	case "psi":
		return "ψ"
	case "omega":
		return "ω"
	case "digamma":
		return "ϝ"
	case "Gamma", "varGamma":
		return "Γ"
	case "Delta", "varDelta":
		return "Δ"
	case "Theta", "varTheta":
		return "Θ"
	case "Lambda", "varLambda":
		return "Λ"
	case "Xi", "varXi":
		return "Ξ"
	case "Pi", "varPi":
		return "Π"
	case "Sigma", "varSigma":
		return "Σ"
	case "Upsilon", "varUpsilon":
		return "Υ"
	case "Phi", "varPhi":
		return "Φ"
	case "Psi", "varPsi":
		return "Ψ"
	case "Omega", "varOmega":
		return "Ω"
	case "Alpha":
		return "Α"
	case "Beta":
		return "Β"
	case "Epsilon":
		return "Ε"
	case "Zeta":
		return "Ζ"
	case "Eta":
		return "Η"
	case "Iota":
		return "Ι"
	case "Kappa":
		return "Κ"
	case "Mu":
		return "Μ"
	case "Nu":
		return "Ν"
	case "Omicron":
		return "Ο"
	case "Rho":
		return "Ρ"
	case "Tau":
		return "Τ"
	case "Chi":
		return "Χ"
	case "aleph":
		return "ℵ"
	case "beth":
		return "ℶ"
	case "gimel":
		return "ℷ"
	case "daleth":
		return "ℸ"
	}
	return ""
}

func relation(name string) string {
	switch name {
	case "leq", "le":
		return "≤"
	case "geq", "ge":
		return "≥"
	case "neq", "ne":
		return "≠"
	case "approx":
		return "≈"
	case "approxeq":
		return "≊"
	case "equiv":
		return "≡"
	case "sim":
		return "∼"
	case "nsim":
		return "≁"
	case "simeq":
		return "≃"
	case "backsim":
		return "∽"
	case "cong":
		return "≅"
	case "ncong":
		return "≇"
	case "propto", "varpropto":
		return "∝"
	case "ll":
		return "≪"
	case "gg":
		return "≫"
	case "lll":
		return "⋘"
	case "ggg":
		return "⋙"
	case "leqq":
		return "≦"
	case "geqq":
		return "≧"
	case "leqslant":
		return "⩽"
	case "geqslant":
		return "⩾"
	case "lesssim":
		return "≲"
	case "gtrsim":
		return "≳"
	case "nless":
		return "≮"
	case "ngtr":
		return "≯"
	case "nleq":
		return "≰"
	case "ngeq":
		return "≱"
	case "lneq":
		return "⪇"
	case "gneq":
		return "⪈"
	case "prec":
		return "≺"
	case "succ":
		return "≻"
	case "preceq":
		return "⪯"
	case "succeq":
		return "⪰"
	case "subset":
		return "⊂"
	case "supset":
		return "⊃"
	case "subseteq":
		return "⊆"
	case "supseteq":
		return "⊇"
	case "subsetneq":
		return "⊊"
	case "supsetneq":
		return "⊋"
	case "nsubseteq":
		return "⊈"
	case "nsupseteq":
		return "⊉"
	case "sqsubset":
		return "⊏"
	case "sqsupset":
		return "⊐"
	case "sqsubseteq":
		return "⊑"
	case "sqsupseteq":
		return "⊒"
	case "in", "isin":
		return "∈"
	case "notin":
		return "∉"
	case "ni", "owns":
		return "∋"
	case "notni":
		return "∌"
	case "to", "rightarrow":
		return "→"
	case "leftarrow", "gets":
		return "←"
	case "leftrightarrow":
		return "↔"
	case "Rightarrow":
		return "⇒"
	case "Leftarrow":
		return "⇐"
	case "Leftrightarrow":
		return "⇔"
	case "iff", "Longleftrightarrow":
		return "⟺"
	case "implies", "Longrightarrow":
		return "⟹"
	case "impliedby", "Longleftarrow":
		return "⟸"
	case "longrightarrow":
		return "⟶"
	case "longleftarrow":
		return "⟵"
	case "longleftrightarrow":
		return "⟷"
	case "mapsto":
		return "↦"
	case "longmapsto":
		return "⟼"
	case "uparrow":
		return "↑"
	case "downarrow":
		return "↓"
	case "updownarrow":
		return "↕"
	case "Uparrow":
		return "⇑"
	case "Downarrow":
		return "⇓"
	case "Updownarrow":
		return "⇕"
	case "nearrow":
		return "↗"
	case "searrow":
		return "↘"
	case "swarrow":
		return "↙"
	case "nwarrow":
		return "↖"
	case "hookrightarrow":
		return "↪"
	case "hookleftarrow":
		return "↩"
	case "twoheadrightarrow":
		return "↠"
	case "rightharpoonup":
		return "⇀"
	case "leftharpoonup":
		return "↼"
	case "rightleftharpoons":
		return "⇌"
	case "leftrightarrows":
		return "⇆"
	case "rightrightarrows":
		return "⇉"
	case "leadsto", "rightsquigarrow":
		return "⇝"
	case "nrightarrow":
		return "↛"
	case "nleftarrow":
		return "↚"
	case "perp":
		return "⊥"
	case "parallel":
		return "∥"
	case "nparallel":
		return "∦"
	case "mid":
		return "∣"
	case "nmid":
		return "∤"
	case "models", "vDash":
		return "⊨"
	case "vdash":
		return "⊢"
	case "dashv":
		return "⊣"
	case "therefore":
		return "∴"
	case "because":
		return "∵"
	case "doteq":
		return "≐"
	case "asymp":
		return "≍"
	case "bowtie", "Join":
		return "⋈"
	case "coloneqq", "coloneq":
		return "≔"
	case "eqqcolon", "eqcolon":
		return "≕"
	case "triangleq":
		return "≜"
	case "smile":
		return "⌣"
	case "frown":
		return "⌢"
	}
	return ""
}

func binary(name string) string {
	switch name {
	case "pm":
		return "±"
	case "mp":
		return "∓"
	case "times":
		return "×"
	case "div":
		return "÷"
	case "cdot", "centerdot":
		return "⋅"
	case "ast":
		return "∗"
	case "star":
		return "⋆"
	case "circ":
		return "∘"
	case "bullet":
		return "∙"
	case "oplus":
		return "⊕"
	case "ominus":
		return "⊖"
	case "otimes":
		return "⊗"
	case "oslash":
		return "⊘"
	case "odot":
		return "⊙"
	case "cup":
		return "∪"
	case "cap":
		return "∩"
	case "sqcup":
		return "⊔"
	case "sqcap":
		return "⊓"
	case "uplus":
		return "⊎"
	case "setminus", "smallsetminus":
		return "∖"
	case "wedge", "land":
		return "∧"
	case "vee", "lor":
		return "∨"
	case "diamond":
		return "⋄"
	case "bigtriangleup":
		return "△"
	case "bigtriangledown":
		return "▽"
	case "triangleleft":
		return "◃"
	case "triangleright":
		return "▹"
	case "dagger":
		return "†"
	case "ddagger":
		return "‡"
	case "amalg":
		return "⨿"
	case "wr":
		return "≀"
	case "ltimes":
		return "⋉"
	case "rtimes":
		return "⋊"
	case "boxplus":
		return "⊞"
	case "boxminus":
		return "⊟"
	case "boxtimes":
		return "⊠"
	case "dotplus":
		return "∔"
	case "intercal":
		return "⊺"
	case "barwedge":
		return "⊼"
	case "veebar":
		return "⊻"
	case "circledast":
		return "⊛"
	case "circledcirc":
		return "⊚"
	}
	return ""
}

func ordinary(name string) string {
	switch name {
	case "infty":
		return "∞"
	case "partial":
		return "∂"
	case "nabla":
		return "∇"
	case "forall":
		return "∀"
	case "exists":
		return "∃"
	case "nexists":
		return "∄"
	case "emptyset", "varnothing":
		return "∅"
	case "angle":
		return "∠"
	case "measuredangle":
		return "∡"
	case "sphericalangle":
		return "∢"
	case "prime":
		return "′"
	case "degree":
		return "°"
	case "Re":
		return "ℜ"
	case "Im":
		return "ℑ"
	case "wp":
		return "℘"
	case "top":
		return "⊤"
	case "bot":
		return "⊥"
	case "ldots", "dots", "dotsc", "dotso", "mathellipsis":
		return "…"
	case "cdots", "dotsb", "dotsm", "dotsi":
		return "⋯"
	case "vdots":
		return "⋮"
	case "ddots":
		return "⋱"
	case "iddots":
		return "⋰"
	case "triangle", "vartriangle":
		return "△"
	case "square", "Box":
		return "□"
	case "blacksquare":
		return "■"
	case "lozenge":
		return "◊"
	case "blacklozenge":
		return "⧫"
	case "diamondsuit":
		return "♢"
	case "heartsuit":
		return "♡"
	case "clubsuit":
		return "♣"
	case "spadesuit":
		return "♠"
	case "flat":
		return "♭"
	case "natural":
		return "♮"
	case "sharp":
		return "♯"
	case "checkmark":
		return "✓"
	case "dag":
		return "†"
	case "ddag":
		return "‡"
	case "S":
		return "§"
	case "P":
		return "¶"
	case "copyright":
		return "©"
	case "pounds":
		return "£"
	case "yen":
		return "¥"
	case "euro":
		return "€"
	case "surd":
		return "√"
	case "backslash", "textbackslash":
		return "\\"
	case "eth":
		return "ð"
	case "mho":
		return "℧"
	case "complement":
		return "∁"
	case "Finv":
		return "Ⅎ"
	case "Game":
		return "⅁"
	case "bigstar":
		return "★"
	case "circledS":
		return "Ⓢ"
	case "|", "Vert":
		return "‖"
	case "vert":
		return "|"
	case "ulcorner":
		return "⌜"
	case "urcorner":
		return "⌝"
	case "llcorner":
		return "⌞"
	case "lrcorner":
		return "⌟"
	}
	return ""
}

// fence resolves opening and closing delimiter commands.
func fence(name string) (string, atomType) {
	switch name {
	case "{", "lbrace":
		return "{", tOpen
	case "}", "rbrace":
		return "}", tClose
	case "langle":
		return "⟨", tOpen
	case "rangle":
		return "⟩", tClose
	case "lfloor":
		return "⌊", tOpen
	case "rfloor":
		return "⌋", tClose
	case "lceil":
		return "⌈", tOpen
	case "rceil":
		return "⌉", tClose
	case "lbrack":
		return "[", tOpen
	case "rbrack":
		return "]", tClose
	case "lvert":
		return "|", tOpen
	case "rvert":
		return "|", tClose
	case "lVert":
		return "‖", tOpen
	case "rVert":
		return "‖", tClose
	case "lgroup":
		return "⟮", tOpen
	case "rgroup":
		return "⟯", tClose
	}
	return "", tOrd
}

// delimGlyph resolves a delimiter command after \left, \right or \big.
func delimGlyph(name string) string {
	if s, _ := fence(name); s != "" {
		return s
	}
	switch name {
	case "|", "Vert":
		return "‖"
	case "vert":
		return "|"
	case "backslash":
		return "\\"
	case "uparrow", "downarrow", "updownarrow", "Uparrow", "Downarrow", "Updownarrow":
		return relation(name)
	}
	return ""
}

// unicodeType classifies math characters typed directly as Unicode.
func unicodeType(r rune) (atomType, bool) {
	switch r {
	case '≤', '≥', '≠', '≈', '≡', '∼', '≃', '≅', '∝', '≪', '≫', '⊂', '⊃', '⊆', '⊇',
		'∈', '∉', '∋', '→', '←', '↔', '⇒', '⇐', '⇔', '⟹', '⟺', '↦', '⊥', '∥', '∣', '⊨', '⊢':
		return tRel, true
	case '±', '∓', '×', '÷', '·', '⋅', '∘', '∙', '⊕', '⊗', '∪', '∩', '∧', '∨', '∖', '−':
		return tBin, true
	case '∑', '∏', '∐', '∫', '∬', '∭', '∮', '⋃', '⋂':
		return tBig, true
	case '∞', '∂', '∇', '∀', '∃', '∅', '¬':
		return tOrd, true
	}
	return tOrd, false
}

// negate returns the negated form of a relation for \not.
func negate(s string) string {
	switch s {
	case "=":
		return "≠"
	case "<":
		return "≮"
	case ">":
		return "≯"
	case "≤":
		return "≰"
	case "≥":
		return "≱"
	case "∈":
		return "∉"
	case "∋":
		return "∌"
	case "≡":
		return "≢"
	case "∼":
		return "≁"
	case "≈":
		return "≉"
	case "≅":
		return "≇"
	case "⊂":
		return "⊄"
	case "⊃":
		return "⊅"
	case "⊆":
		return "⊈"
	case "⊇":
		return "⊉"
	case "∣":
		return "∤"
	case "∥":
		return "∦"
	case "→":
		return "↛"
	case "←":
		return "↚"
	}
	return s + "̸"
}

// bigOp resolves big operators; limits reports whether display mode puts
// the scripts above and below (integrals keep them on the right).
func bigOp(name string) (string, bool, bool) {
	switch name {
	case "sum":
		return "∑", true, true
	case "prod":
		return "∏", true, true
	case "coprod":
		return "∐", true, true
	case "bigcup":
		return "⋃", true, true
	case "bigcap":
		return "⋂", true, true
	case "bigoplus":
		return "⨁", true, true
	case "bigotimes":
		return "⨂", true, true
	case "bigodot":
		return "⨀", true, true
	case "biguplus":
		return "⨄", true, true
	case "bigsqcup":
		return "⨆", true, true
	case "bigvee":
		return "⋁", true, true
	case "bigwedge":
		return "⋀", true, true
	case "int", "intop", "smallint":
		return "∫", false, true
	case "iint":
		return "∬", false, true
	case "iiint":
		return "∭", false, true
	case "iiiint":
		return "⨌", false, true
	case "oint":
		return "∮", false, true
	case "oiint":
		return "∯", false, true
	case "oiiint":
		return "∰", false, true
	}
	return "", false, false
}

// function resolves named operators such as \sin or \lim.
func function(name string) (string, bool, bool) {
	switch name {
	case "sin", "cos", "tan", "cot", "sec", "csc", "arcsin", "arccos", "arctan",
		"arccot", "arcsec", "arccsc", "sinh", "cosh", "tanh", "coth", "sech", "csch",
		"log", "ln", "lg", "exp", "dim", "ker", "deg", "arg", "hom", "tg", "ctg",
		"sh", "ch", "th", "arctg", "arcctg", "cth":
		return name, false, true
	case "lim", "max", "min", "sup", "inf", "det", "gcd", "Pr", "lcm":
		return name, true, true
	case "limsup":
		return "lim sup", true, true
	case "liminf":
		return "lim inf", true, true
	case "injlim":
		return "inj lim", true, true
	case "projlim":
		return "proj lim", true, true
	case "argmax":
		return "arg max", true, true
	case "argmin":
		return "arg min", true, true
	}
	return "", false, false
}

// accentMark returns the combining character of an accent command.
func accentMark(name string) rune {
	switch name {
	case "hat", "widehat":
		return 0x302
	case "check", "widecheck":
		return 0x30C
	case "tilde", "widetilde":
		return 0x303
	case "acute":
		return 0x301
	case "grave":
		return 0x300
	case "dot":
		return 0x307
	case "ddot":
		return 0x308
	case "dddot":
		return 0x20DB
	case "breve":
		return 0x306
	case "bar":
		return 0x304
	case "overline", "overbar":
		return 0x305
	case "underline", "underbar":
		return 0x332
	case "vec", "overrightarrow":
		return 0x20D7
	case "overleftarrow":
		return 0x20D6
	case "overleftrightarrow":
		return 0x20E1
	case "mathring":
		return 0x30A
	case "utilde", "undertilde":
		return 0x330
	case "cancel", "bcancel", "xcancel", "sout":
		return 0x336
	}
	return 0
}

// Accent styles: how a mark spans multi-character bodies.
const (
	accOver   = iota // a single spacing glyph centered above
	accLine          // a line over every character
	accArrow         // a stretched arrow above
	accUnder         // a line under every character
	accStrike        // struck through every character
)

func accentStyle(m rune) int {
	switch m {
	case 0x304, 0x305:
		return accLine
	case 0x20D6, 0x20D7, 0x20E1:
		return accArrow
	case 0x332, 0x330:
		return accUnder
	case 0x336:
		return accStrike
	}
	return accOver
}

// accentGlyph is the row drawn above (or below) a multi-column body.
func accentGlyph(m rune, w int) string {
	switch m {
	case 0x304, 0x305:
		return strings.Repeat("_", w)
	case 0x332:
		return strings.Repeat("‾", w)
	case 0x330:
		return "~"
	case 0x20D7:
		return strings.Repeat("─", max(w-1, 0)) + "→"
	case 0x20D6:
		return "←" + strings.Repeat("─", max(w-1, 0))
	case 0x20E1:
		return "←" + strings.Repeat("─", max(w-2, 0)) + "→"
	case 0x302:
		return "^"
	case 0x30C:
		return "ˇ"
	case 0x303:
		return "~"
	case 0x301:
		return "´"
	case 0x300:
		return "`"
	case 0x307:
		return "˙"
	case 0x308:
		return "¨"
	case 0x20DB:
		return "⋯"
	case 0x306:
		return "˘"
	}
	return "˚"
}

// xArrow returns the arrow of an extensible-arrow command.
func xArrow(name string) string {
	switch name {
	case "xleftarrow":
		return "←"
	case "xRightarrow":
		return "⇒"
	case "xLeftarrow":
		return "⇐"
	case "xleftrightarrow":
		return "↔"
	case "xLeftrightarrow":
		return "⇔"
	case "xmapsto":
		return "↦"
	case "xhookrightarrow":
		return "↪"
	case "xtwoheadrightarrow":
		return "↠"
	case "xlongequal":
		return "="
	}
	return "→"
}

// stretchArrow draws arrow a over w columns.
func stretchArrow(a string, w int) string {
	switch a {
	case "←":
		return "←" + strings.Repeat("─", w-1)
	case "⇒":
		return strings.Repeat("═", w-1) + "⇒"
	case "⇐":
		return "⇐" + strings.Repeat("═", w-1)
	case "↔":
		return "←" + strings.Repeat("─", w-2) + "→"
	case "⇔":
		return "⇐" + strings.Repeat("═", w-2) + "⇒"
	case "↦":
		return "├" + strings.Repeat("─", w-2) + "→"
	case "=":
		return strings.Repeat("═", w)
	case "↠":
		return strings.Repeat("─", w-1) + "↠"
	}
	return strings.Repeat("─", w-1) + "→"
}

// font is a math alphabet.
type font uint8

const (
	fNone font = iota
	fBB        // double-struck: \mathbb
	fCal       // script: \mathcal, \mathscr
	fFrak      // fraktur: \mathfrak
	fBF        // bold: \mathbf
	fBI        // bold italic: \boldsymbol, \bm
	fIt        // italic: \mathit
	fRm        // upright: \mathrm
	fSf        // sans-serif: \mathsf
	fTt        // monospace: \mathtt
)

func fontCommand(name string) (font, bool) {
	switch name {
	case "mathbb", "Bbb", "mathbbm", "bbold":
		return fBB, true
	case "mathcal", "mathscr":
		return fCal, true
	case "mathfrak":
		return fFrak, true
	case "mathbf", "bold", "textbf":
		return fBF, true
	case "boldsymbol", "bm", "pmb":
		return fBI, true
	case "mathit", "mathnormal":
		return fIt, true
	case "mathrm", "mathup":
		return fRm, true
	case "mathsf":
		return fSf, true
	case "mathtt":
		return fTt, true
	}
	return fNone, false
}

// switchFont maps old-style font switches ({\bf x}).
func switchFont(name string) font {
	switch name {
	case "rm":
		return fRm
	case "bf":
		return fBF
	case "sf":
		return fSf
	case "tt":
		return fTt
	case "cal":
		return fCal
	case "frak":
		return fFrak
	}
	return fIt
}

// fontRune maps an ASCII letter or digit to the Mathematical Alphanumeric
// Symbols block; other runes are returned unchanged.
func fontRune(f font, r rune) rune {
	upper, lower, digit := r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9'
	if !upper && !lower && !digit {
		return r
	}
	var cap, low, dig rune
	switch f {
	case fBB:
		switch r {
		case 'C':
			return 'ℂ'
		case 'H':
			return 'ℍ'
		case 'N':
			return 'ℕ'
		case 'P':
			return 'ℙ'
		case 'Q':
			return 'ℚ'
		case 'R':
			return 'ℝ'
		case 'Z':
			return 'ℤ'
		}
		cap, low, dig = 0x1D538, 0x1D552, 0x1D7D8
	case fCal:
		switch r {
		case 'B':
			return 'ℬ'
		case 'E':
			return 'ℰ'
		case 'F':
			return 'ℱ'
		case 'H':
			return 'ℋ'
		case 'I':
			return 'ℐ'
		case 'L':
			return 'ℒ'
		case 'M':
			return 'ℳ'
		case 'R':
			return 'ℛ'
		case 'e':
			return 'ℯ'
		case 'g':
			return 'ℊ'
		case 'o':
			return 'ℴ'
		}
		cap, low = 0x1D49C, 0x1D4B6
	case fFrak:
		switch r {
		case 'C':
			return 'ℭ'
		case 'H':
			return 'ℌ'
		case 'I':
			return 'ℑ'
		case 'R':
			return 'ℜ'
		case 'Z':
			return 'ℨ'
		}
		cap, low = 0x1D504, 0x1D51E
	case fBF:
		cap, low, dig = 0x1D400, 0x1D41A, 0x1D7CE
	case fBI:
		cap, low, dig = 0x1D468, 0x1D482, 0x1D7CE
	case fSf:
		cap, low, dig = 0x1D5A0, 0x1D5BA, 0x1D7E2
	case fTt:
		cap, low, dig = 0x1D670, 0x1D68A, 0x1D7F6
	}
	switch {
	case upper && cap != 0:
		return cap + r - 'A'
	case lower && low != 0:
		return low + r - 'a'
	case digit && dig != 0:
		return dig + r - '0'
	}
	return r
}

// mapFont applies fontRune to every rune of s.
func mapFont(f font, s string) string {
	if f == fNone || f == fIt || f == fRm {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(fontRune(f, r))
	}
	return b.String()
}

// supRune returns the Unicode superscript form of r, or 0.
func supRune(r rune) rune {
	switch r {
	case '0':
		return '⁰'
	case '1':
		return '¹'
	case '2':
		return '²'
	case '3':
		return '³'
	case '4', '5', '6', '7', '8', '9':
		return '⁴' + r - '4'
	case '+':
		return '⁺'
	case '−', '-':
		return '⁻'
	case '=':
		return '⁼'
	case '(':
		return '⁽'
	case ')':
		return '⁾'
	case 'a':
		return 'ᵃ'
	case 'b':
		return 'ᵇ'
	case 'c':
		return 'ᶜ'
	case 'd':
		return 'ᵈ'
	case 'e':
		return 'ᵉ'
	case 'f':
		return 'ᶠ'
	case 'g':
		return 'ᵍ'
	case 'h':
		return 'ʰ'
	case 'i':
		return 'ⁱ'
	case 'j':
		return 'ʲ'
	case 'k':
		return 'ᵏ'
	case 'l':
		return 'ˡ'
	case 'm':
		return 'ᵐ'
	case 'n':
		return 'ⁿ'
	case 'o':
		return 'ᵒ'
	case 'p':
		return 'ᵖ'
	case 'r':
		return 'ʳ'
	case 's':
		return 'ˢ'
	case 't':
		return 'ᵗ'
	case 'u':
		return 'ᵘ'
	case 'v':
		return 'ᵛ'
	case 'w':
		return 'ʷ'
	case 'x':
		return 'ˣ'
	case 'y':
		return 'ʸ'
	case 'z':
		return 'ᶻ'
	case 'A':
		return 'ᴬ'
	case 'B':
		return 'ᴮ'
	case 'D':
		return 'ᴰ'
	case 'E':
		return 'ᴱ'
	case 'G':
		return 'ᴳ'
	case 'H':
		return 'ᴴ'
	case 'I':
		return 'ᴵ'
	case 'J':
		return 'ᴶ'
	case 'K':
		return 'ᴷ'
	case 'L':
		return 'ᴸ'
	case 'M':
		return 'ᴹ'
	case 'N':
		return 'ᴺ'
	case 'O':
		return 'ᴼ'
	case 'P':
		return 'ᴾ'
	case 'R':
		return 'ᴿ'
	case 'T':
		return 'ᵀ'
	case 'U':
		return 'ᵁ'
	case 'V':
		return 'ⱽ'
	case 'W':
		return 'ᵂ'
	case 'α':
		return 'ᵅ'
	case 'β':
		return 'ᵝ'
	case 'γ':
		return 'ᵞ'
	case 'δ':
		return 'ᵟ'
	case 'ε', 'ϵ':
		return 'ᵋ'
	case 'θ':
		return 'ᶿ'
	case 'ι':
		return 'ᶥ'
	case 'φ', 'ϕ':
		return 'ᵠ'
	case 'χ':
		return 'ᵡ'
	case '∘', '°':
		return '°'
	case '′', '″', '‴', '⁗', '∗', '*', '†', '‡', // already raised
		'⁰', '¹', '²', '³', '⁴', '⁵', '⁶', '⁷', '⁸', '⁹', '⁺', '⁻', '⁼', '⁽', '⁾', 'ⁿ', 'ⁱ':
		return r
	}
	return 0
}

// subRune returns the Unicode subscript form of r, or 0.
func subRune(r rune) rune {
	switch r {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return '₀' + r - '0'
	case '+':
		return '₊'
	case '−', '-':
		return '₋'
	case '=':
		return '₌'
	case '(':
		return '₍'
	case ')':
		return '₎'
	case 'a':
		return 'ₐ'
	case 'e':
		return 'ₑ'
	case 'h':
		return 'ₕ'
	case 'i':
		return 'ᵢ'
	case 'j':
		return 'ⱼ'
	case 'k':
		return 'ₖ'
	case 'l':
		return 'ₗ'
	case 'm':
		return 'ₘ'
	case 'n':
		return 'ₙ'
	case 'o':
		return 'ₒ'
	case 'p':
		return 'ₚ'
	case 'r':
		return 'ᵣ'
	case 's':
		return 'ₛ'
	case 't':
		return 'ₜ'
	case 'u':
		return 'ᵤ'
	case 'v':
		return 'ᵥ'
	case 'x':
		return 'ₓ'
	case 'β':
		return 'ᵦ'
	case 'γ':
		return 'ᵧ'
	case 'ρ':
		return 'ᵨ'
	case 'φ', 'ϕ':
		return 'ᵩ'
	case 'χ':
		return 'ᵪ'
	case '₀', '₁', '₂', '₃', '₄', '₅', '₆', '₇', '₈', '₉', '₊', '₋', '₌', '₍', '₎', 'ᵢ', 'ⱼ', 'ₙ', 'ₖ':
		return r // already lowered
	}
	return 0
}

// vulgar returns the precomposed fraction for n/d, or "".
func vulgar(n, d string) string {
	switch n + "/" + d {
	case "1/2":
		return "½"
	case "1/3":
		return "⅓"
	case "2/3":
		return "⅔"
	case "1/4":
		return "¼"
	case "3/4":
		return "¾"
	case "1/5":
		return "⅕"
	case "2/5":
		return "⅖"
	case "3/5":
		return "⅗"
	case "4/5":
		return "⅘"
	case "1/6":
		return "⅙"
	case "5/6":
		return "⅚"
	case "1/7":
		return "⅐"
	case "1/8":
		return "⅛"
	case "3/8":
		return "⅜"
	case "5/8":
		return "⅝"
	case "7/8":
		return "⅞"
	case "1/9":
		return "⅑"
	case "1/10":
		return "⅒"
	}
	return ""
}
