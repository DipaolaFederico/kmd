// Package render renders Markdown for terminals and as HTML, with the same
// renderer, themes and safety limits as the kmd command.
//
//	out, err := render.Render(src, render.Options{Width: 80, Color: true})
//
// The input is GitHub Flavored Markdown with GitHub alerts, footnotes, math
// ($…$, $$…$$), mermaid and Graphviz diagrams, emoji shortcodes and wiki
// links; [Options.CommonMark] restricts it to strict CommonMark.
//
// # Untrusted documents
//
// Documents are treated as hostile input, whatever the options:
//
//   - Control characters and escape sequences in the document never reach
//     the output: only the renderer's own colour (SGR), hyperlink (OSC 8)
//     and image sequences do, and only when enabled.
//   - Work and memory are bounded: inputs over 16 MiB are refused with
//     [ErrTooLarge], and nesting, table size, links, diagrams, formulas and
//     images are capped per document, so a short document cannot make a
//     call run for long or allocate without limit.
//   - Nothing is fetched from the network and no program is run. The only
//     files read are local images, and only when [Options.Images] is set:
//     they must lie inside [Options.BaseDir] (absolute paths, file: URLs,
//     ".." and symbolic links leading outside it are refused), are size
//     limited and validated as PNG, JPEG or GIF before decoding. Hard links
//     and mount points inside BaseDir are not detected: BaseDir should be a
//     directory the document's author cannot write outside of. Theme files
//     are read only by [LoadTheme], from the paths the caller passes.
//   - [HTML] drops control characters, omits raw HTML and drops
//     javascript:, vbscript:, file: and non-image data: URLs unless
//     [HTMLOptions.Unsafe] is set (raw HTML is then passed as written).
//
// # Concurrency
//
// [Render], [HTML] and the theme functions are safe for concurrent use, and
// a [*Theme] may be shared by any number of goroutines: themes are immutable.
//
// # Compatibility
//
// The package follows semantic versioning with the kmd module: from v1.0.0
// on, exported identifiers are only added, never changed or removed, within
// a major version. The rendered output itself is not part of the API: it
// improves between releases (layout, colours, new syntax), so tests should
// not compare it byte for byte against a fixed string.
package render
