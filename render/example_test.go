package render_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/DipaolaFederico/kmd/render"
)

func ExampleRender() {
	out, err := render.Render("# Hello\n\nSome **bold** text and $x^2$.\n", render.Options{Width: 40})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	//
	//   HELLO
	//   ═════
	//
	//   Some bold text and x².
}

func ExampleRender_terminal() {
	// Colours, hyperlinks and the light theme, as for a terminal with a
	// light background. Images in the document are drawn with Unicode
	// blocks, read only from the docs directory.
	out, err := render.Render("See [the docs](https://example.com).\n", render.Options{
		Width:      100,
		Theme:      render.DefaultTheme(false),
		Color:      true,
		TrueColor:  true,
		Hyperlinks: true,
		Images:     render.ImagesBlocks,
		BaseDir:    "docs",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(strings.Contains(out, "\x1b]8;;https://example.com"))
	// Output: true
}

func ExampleHTML() {
	out, err := render.HTML("Hi <script>alert(1)</script> [x](javascript:alert(1))\n", render.HTMLOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(out)
	// Output:
	// <p>Hi <!-- raw HTML omitted -->alert(1)<!-- raw HTML omitted --> <a href="">x</a></p>
}

func ExampleLoadTheme() {
	// A bundled theme, or ~/.config/kmd/themes/NAME.theme when the
	// directory is passed.
	th, err := render.LoadTheme("dracula")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(th.Name(), th.Dark())
	// Output: dracula true
}

func ExampleParseTheme() {
	th, err := render.ParseTheme("mine", strings.NewReader(`
inherit = nord
link    = #ff8800
`))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(th.Name(), th.Dark())
	// Output: mine true
}

func ExampleThemes() {
	fmt.Println(render.Themes()[:2])
	// Output: [tokyo-night tokyo-night-day]
}
