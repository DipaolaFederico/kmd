package render

import "strings"

// lang is a tiny lexical description of a programming language: enough
// for pleasant highlighting without shipping real grammars.
type lang struct {
	kw         map[string]bool
	lineCmt    []string
	blockCmt   [2]string
	quotes     string
	types      bool // Capitalized identifiers are types
	ci         bool // case-insensitive keywords
	keyColon   bool // `key:` is highlighted (YAML/TOML-like)
	dollarVars bool // $VAR highlighted (shells)
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

const (
	cKw    = "auto break case char class const continue default do double else enum extern float for goto if int long register return short signed sizeof static struct switch typedef union unsigned void volatile while bool true false nullptr NULL namespace using template typename public private protected virtual override new delete this throw try catch final import package extends implements boolean byte super null var val fun let func in is as when object interface"
	jsKw   = "async await break case catch class const continue debugger default delete do else export extends false finally for from function if import in instanceof let new null of return static super switch this throw true try typeof undefined var void while yield interface type enum implements readonly as declare keyof"
	baseKw = "if else elif for while return function func fn def class import from const let var true false null nil none None True False and or not in end do then"
)

var cStyle = [2]string{"/*", "*/"}

// langFor returns the lexer for a fenced-code info string, or nil.
func langFor(name string) *lang {
	switch name {
	case "":
		return nil
	case "go", "golang":
		return &lang{kw: words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var nil true false iota"), lineCmt: []string{"//"}, blockCmt: cStyle, quotes: "\"'`", types: true}
	case "rust", "rs":
		return &lang{kw: words("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while"), lineCmt: []string{"//"}, blockCmt: cStyle, quotes: "\"", types: true}
	case "python", "py":
		return &lang{kw: words("and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield self match case"), lineCmt: []string{"#"}, quotes: "\"'", types: true}
	case "js", "javascript", "ts", "typescript", "jsx", "tsx", "mjs":
		return &lang{kw: words(jsKw), lineCmt: []string{"//"}, blockCmt: cStyle, quotes: "\"'`", types: true}
	case "c", "h", "cpp", "c++", "cc", "hpp", "java", "cs", "csharp", "kotlin", "kt", "swift", "scala", "dart", "php":
		return &lang{kw: words(cKw), lineCmt: []string{"//"}, blockCmt: cStyle, quotes: "\"'", types: true}
	case "sh", "bash", "zsh", "shell", "console", "fish", "ksh":
		return &lang{kw: words("if then else elif fi case esac for while until do done in function return local export readonly exit set unset source alias sudo echo cd"), lineCmt: []string{"#"}, quotes: "\"'", dollarVars: true}
	case "sql", "mysql", "postgresql", "psql", "sqlite":
		return &lang{kw: words("select from where and or not insert into values update set delete create table index view drop alter add join left right inner outer full on as group by order having limit offset union all distinct null is in like between case when then else end primary key foreign references default exists returning with"), lineCmt: []string{"--"}, blockCmt: cStyle, quotes: "'\"", ci: true}
	case "ruby", "rb":
		return &lang{kw: words("alias and begin break case class def defined? do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield require puts attr_accessor"), lineCmt: []string{"#"}, quotes: "\"'", types: true}
	case "lua":
		return &lang{kw: words("and break do else elseif end false for function goto if in local nil not or repeat return then true until while"), lineCmt: []string{"--"}, quotes: "\"'"}
	case "yaml", "yml":
		return &lang{kw: words("true false null yes no on off ~"), lineCmt: []string{"#"}, quotes: "\"'", keyColon: true}
	case "toml", "ini", "conf", "cfg", "properties", "env", "dotenv":
		return &lang{kw: words("true false"), lineCmt: []string{"#", ";"}, quotes: "\"'", keyColon: true}
	case "json", "jsonc", "json5":
		return &lang{kw: words("true false null"), lineCmt: []string{"//"}, quotes: "\""}
	case "css", "scss", "less", "sass":
		return &lang{kw: words("important inherit initial unset none auto"), blockCmt: cStyle, lineCmt: []string{"//"}, quotes: "\"'"}
	case "html", "xml", "svg", "vue", "svelte":
		return &lang{kw: map[string]bool{}, blockCmt: [2]string{"<!--", "-->"}, quotes: "\"'"}
	case "dockerfile", "docker", "containerfile":
		return &lang{kw: words("FROM RUN CMD COPY ADD ENV ARG WORKDIR EXPOSE ENTRYPOINT USER LABEL VOLUME HEALTHCHECK SHELL ONBUILD STOPSIGNAL AS"), lineCmt: []string{"#"}, quotes: "\"'", dollarVars: true}
	case "make", "makefile", "mk":
		return &lang{kw: words("ifeq ifneq ifdef ifndef else endif include define endef export override .PHONY"), lineCmt: []string{"#"}, quotes: "\"'", dollarVars: true}
	}
	return &lang{kw: words(baseKw), lineCmt: []string{"//", "#"}, blockCmt: cStyle, quotes: "\"'`", types: true}
}

func isIdentStart(c byte) bool { return isAlphaByte(c) || c == '_' }
func isIdent(c byte) bool      { return isIdentStart(c) || (c >= '0' && c <= '9') }
func isAlphaByte(c byte) bool  { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }

// highlight tokenizes code, reusing lexers already built for this document.
func (r *renderer) highlight(src, name string) []rline {
	if r.langs == nil {
		r.langs = map[string]*lang{}
	}
	lg, ok := r.langs[name]
	if !ok {
		lg = langFor(name)
		r.langs[name] = lg
	}
	return highlightWith(src, name, lg, r.th)
}

// highlight tokenizes code into styled lines.
func highlight(src, name string, th *Theme) []rline {
	return highlightWith(src, name, langFor(name), th)
}

func highlightWith(src, name string, lg *lang, th *Theme) []rline {
	lines := strings.Split(src, "\n")
	out := make([]rline, len(lines))
	if name == "diff" || name == "patch" {
		for i, l := range lines {
			st := Style{}
			switch {
			case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
				st = Style{Fg: th.Muted, Attr: Bold}
			case strings.HasPrefix(l, "+"):
				st.Fg = th.Added
			case strings.HasPrefix(l, "-"):
				st.Fg = th.Removed
			case strings.HasPrefix(l, "@@"):
				st.Fg = th.Type
			}
			out[i] = rline{{text: l, st: st}}
		}
		return out
	}
	if lg == nil {
		for i, l := range lines {
			out[i] = rline{{text: l}}
		}
		return out
	}
	com := Style{Fg: th.Comment, Attr: Italic}
	inBlock := false
	for i, l := range lines {
		h := lineLexer{l: l}
		for h.j < len(l) {
			if inBlock {
				e := strings.Index(l[h.j:], lg.blockCmt[1])
				if e < 0 {
					h.token(len(l), com)
					break
				}
				h.token(h.j+e+len(lg.blockCmt[1]), com)
				inBlock = false
				continue
			}
			rest := l[h.j:]
			if hasAnyPrefix(rest, lg.lineCmt) {
				h.token(len(l), com)
				break
			}
			if lg.blockCmt[0] != "" && strings.HasPrefix(rest, lg.blockCmt[0]) {
				if e := strings.Index(rest[len(lg.blockCmt[0]):], lg.blockCmt[1]); e >= 0 {
					h.token(h.j+len(lg.blockCmt[0])+e+len(lg.blockCmt[1]), com)
					continue
				}
				h.token(len(l), com)
				inBlock = true
				break
			}
			c := l[h.j]
			switch {
			case strings.IndexByte(lg.quotes, c) >= 0:
				k := h.j + 1
				for k < len(l) && l[k] != c {
					if l[k] == '\\' {
						k++
					}
					k++
				}
				if k < len(l) {
					k++
				}
				h.token(min(k, len(l)), Style{Fg: th.String})
			case c >= '0' && c <= '9' && (h.j == 0 || !isIdent(l[h.j-1])):
				k := h.j
				for k < len(l) && (isIdent(l[k]) || l[k] == '.') {
					k++
				}
				h.token(k, Style{Fg: th.Num})
			case c == '$' && lg.dollarVars && h.j+1 < len(l) && (isIdentStart(l[h.j+1]) || l[h.j+1] == '{'):
				k := h.j + 1
				for k < len(l) && (isIdent(l[k]) || l[k] == '{' || l[k] == '}') {
					k++
				}
				h.token(k, Style{Fg: th.Type})
			case isIdentStart(c):
				k := h.j
				for k < len(l) && (isIdent(l[k]) || l[k] == '-' && lg.keyColon) {
					k++
				}
				w := l[h.j:k]
				lookup := w
				if lg.ci {
					lookup = strings.ToLower(w)
				}
				switch {
				case lg.keyColon && k < len(l) && (l[k] == ':' || strings.HasPrefix(strings.TrimLeft(l[k:], " "), "=")):
					h.token(k, Style{Fg: th.Func})
				case lg.kw[lookup]:
					h.token(k, Style{Fg: th.Keyword})
				case k < len(l) && l[k] == '(':
					h.token(k, Style{Fg: th.Func})
				case lg.types && w[0] >= 'A' && w[0] <= 'Z':
					h.token(k, Style{Fg: th.Type})
				default:
					h.j = k
				}
			default:
				h.j++
			}
		}
		h.token(len(l), Style{})
		out[i] = h.out
	}
	return out
}

func hasAnyPrefix(s string, ps []string) bool {
	for _, p := range ps {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// lineLexer accumulates styled tokens; text between tokens stays unstyled.
type lineLexer struct {
	l     string
	j     int // scan position
	plain int // start of pending unstyled text
	out   rline
}

// token emits pending plain text, then l[j:end] with style st.
func (h *lineLexer) token(end int, st Style) {
	if h.j > h.plain {
		h.out = append(h.out, run{text: h.l[h.plain:h.j]})
	}
	if end > h.j {
		h.out = append(h.out, run{text: h.l[h.j:end], st: st})
	}
	h.j, h.plain = end, end
}
