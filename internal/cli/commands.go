package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
)

func listThemes(env Env, s settings) int {
	dirs := render.ThemeDirs(env.Getenv)
	user := render.UserThemes(dirs)
	sort.Strings(user)
	if !s.opts.Color {
		for _, n := range append(render.BuiltinThemes(), user...) {
			fmt.Fprintln(env.Stdout, n)
		}
		return 0
	}
	var b strings.Builder
	section := func(title string, names []string) {
		fmt.Fprintf(&b, "\n  \x1b[1m%s\x1b[0m\n", title)
		for _, n := range names {
			t, err := render.LoadTheme(n, dirs)
			if err != nil {
				fmt.Fprintf(&b, "    %-18s \x1b[2m(%v)\x1b[0m\n", n, err)
				continue
			}
			mark := "  "
			if n == s.themeNm {
				mark = "▸ "
			}
			kind := "light"
			if t.Dark {
				kind = "dark"
			}
			fmt.Fprintf(&b, "  %s%-18s ", mark, n)
			b.WriteString(swatch(t.Bg, t.Fg, " Aa "))
			for _, c := range t.Series {
				b.WriteString(swatch(c, 0, "  "))
			}
			fmt.Fprintf(&b, "  \x1b[2m%s\x1b[0m\n", kind)
		}
	}
	section("Bundled themes", render.BuiltinThemes())
	if len(user) > 0 {
		section("Your themes ("+strings.Join(dirs[:1], "")+")", user)
	}
	b.WriteString("\n  Use: kmd -t NAME · customize: kmd --theme-template NAME > ~/.config/kmd/themes/mine.theme\n\n")
	_, _ = io.WriteString(env.Stdout, b.String())
	return 0
}

func rgb(c render.Color) string {
	v := uint32(c)
	return strconv.Itoa(int(v>>16&255)) + ";" + strconv.Itoa(int(v>>8&255)) + ";" + strconv.Itoa(int(v&255))
}

func swatch(bg, fg render.Color, text string) string {
	s := "\x1b[48;2;" + rgb(bg) + "m"
	if fg != 0 {
		s += "\x1b[38;2;" + rgb(fg) + "m"
	}
	return s + text + "\x1b[0m"
}

func showConfig(env Env, cfg config, s settings) int {
	onOff := map[bool]string{true: "on", false: "off"}
	eff := map[string]string{
		"theme":      s.themeNm,
		"width":      strconv.Itoa(s.opts.Width),
		"color":      onOff[s.opts.Color],
		"truecolor":  onOff[s.opts.TrueColor],
		"hyperlinks": onOff[s.opts.Hyperlinks],
		"images":     s.opts.Images.String(),
		"math":       onOff[s.opts.Math],
		"diagrams":   onOff[s.opts.Diagrams],
		"pager":      firstNonEmpty(s.pager, "(disabled)"),
		"mouse":      onOff[s.mouse],
		"view":       map[bool]string{true: "always", false: "auto"}[s.alwaysView],
	}
	path := firstNonEmpty(cfg.path, "(no HOME)")
	fmt.Fprintf(env.Stdout, "Config file:  %s\nTheme dirs:   %s\n\nEffective settings:\n%s\nTemplate (copy to the config file and uncomment):\n\n%s",
		path, strings.Join(render.ThemeDirs(env.Getenv), ", "), cfg.describe(eff), configTemplate)
	return 0
}

const bashCompletion = `# bash completion for kmd
_kmd() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    case "$prev" in
        -t|--theme|--theme-template)
            COMPREPLY=($(compgen -W "$(kmd --list-themes --color=never 2>/dev/null)" -- "$cur")); return ;;
        -c|--color|--hyperlinks)
            COMPREPLY=($(compgen -W "auto always never" -- "$cur")); return ;;
        -i|--images)
            COMPREPLY=($(compgen -W "auto blocks kitty iterm2 sixel none" -- "$cur")); return ;;
        --completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")); return ;;
        -w|--width|-s|--section) return ;;
    esac
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "-w --width -t --theme -c --color -i --images --hyperlinks --no-math --no-diagrams --commonmark -P --no-pager --view --slides --toc -s --section --html --unsafe-html -f --follow --list-themes --theme-template --config --completion -v --version -h --help" -- "$cur"))
        return
    fi
    COMPREPLY=($(compgen -f -X '!*.@(md|markdown|mdown|mkd|txt|MD)' -- "$cur") $(compgen -d -- "$cur"))
}
complete -o filenames -F _kmd kmd
`

const zshCompletion = `#compdef kmd
# zsh completion for kmd
_kmd_themes() { local -a t; t=(${(f)"$(kmd --list-themes --color=never 2>/dev/null)"}); _describe theme t }
_arguments -s \
  '(-w --width)'{-w,--width}'[render width]:columns:' \
  '(-t --theme)'{-t,--theme}'[color theme]:theme:_kmd_themes' \
  '(-c --color)'{-c,--color}'[use colors]:when:(auto always never)' \
  '(-i --images)'{-i,--images}'[image protocol]:mode:(auto blocks kitty iterm2 sixel none)' \
  '--hyperlinks[clickable links]:when:(auto always never)' \
  '--no-math[show LaTeX source]' \
  '--no-diagrams[show diagram source]' \
  '--commonmark[strict CommonMark]' \
  '(-P --no-pager)'{-P,--no-pager}'[never page output]' \
  '--view[open the viewer even for short documents]' \
  '--toc[print table of contents]' \
  '--slides[present as slides]' \
  '(-s --section)'{-s,--section}'[only the section under a heading]:heading:' \
  '--html[export HTML]' \
  '--unsafe-html[keep raw HTML in --html]' \
  '(-f --follow)'{-f,--follow}'[re-render on change]' \
  '--list-themes[list themes]' \
  '--theme-template[print editable theme]:theme:_kmd_themes' \
  '--config[show configuration]' \
  '--completion[print completion script]:shell:(bash zsh fish)' \
  '(-v --version)'{-v,--version}'[print version]' \
  '(-h --help)'{-h,--help}'[show help]' \
  '*:markdown file:_files -g "*.(md|markdown|mdown|mkd|txt)(-.)"'
`

const fishCompletion = `# fish completion for kmd
complete -c kmd -k -f -a "(__fish_complete_suffix .md .markdown .mdown .mkd .txt)"
complete -c kmd -s w -l width -x -d 'Render width'
complete -c kmd -s t -l theme -x -a "(kmd --list-themes --color=never 2>/dev/null)" -d 'Color theme'
complete -c kmd -s c -l color -x -a 'auto always never' -d 'Use colors'
complete -c kmd -s i -l images -x -a 'auto blocks kitty iterm2 sixel none' -d 'Image protocol'
complete -c kmd -l hyperlinks -x -a 'auto always never' -d 'Clickable links'
complete -c kmd -l no-math -d 'Show LaTeX source'
complete -c kmd -l no-diagrams -d 'Show diagram source'
complete -c kmd -l commonmark -d 'Strict CommonMark'
complete -c kmd -s P -l no-pager -d 'Never page output'
complete -c kmd -l view -d 'Open the viewer even for short documents'
complete -c kmd -l toc -d 'Print table of contents'
complete -c kmd -l slides -d 'Present as slides'
complete -c kmd -s s -l section -x -d 'Only the section under a heading'
complete -c kmd -l html -d 'Export HTML'
complete -c kmd -l unsafe-html -d 'Keep raw HTML in --html'
complete -c kmd -s f -l follow -d 'Re-render on change'
complete -c kmd -l list-themes -d 'List themes'
complete -c kmd -l theme-template -x -a "(kmd --list-themes --color=never 2>/dev/null)" -d 'Print editable theme'
complete -c kmd -l config -d 'Show configuration'
complete -c kmd -l completion -x -a 'bash zsh fish' -d 'Print completion script'
complete -c kmd -s v -l version -d 'Print version'
complete -c kmd -s h -l help -d 'Show help'
`

func completion(env Env, shell string) int {
	switch shell {
	case "bash":
		_, _ = io.WriteString(env.Stdout, bashCompletion)
	case "zsh":
		_, _ = io.WriteString(env.Stdout, zshCompletion)
	case "fish":
		_, _ = io.WriteString(env.Stdout, fishCompletion)
	default:
		fmt.Fprintf(env.Stderr, "kmd: unknown shell %q (want bash, zsh or fish)\n", shell)
		return 2
	}
	return 0
}
