package render

// langDef describes how to highlight one language. It is plain data read
// by the generic tokenizer in highlight.go: adding a language means adding
// an entry here and a golden test (testdata/highlight/<alias>.txt; see
// CONTRIBUTING.md, "Adding a language"). Word lists and delimiters are
// space-separated strings so the table is static data with no start-up
// cost.
type langDef struct {
	name    string // as listed in the README and the man page
	aliases string // fence info strings and file extensions, lowercase

	keywords string // → syntax-keyword
	types    string // built-in type names → syntax-type
	builtins string // built-in functions and constants → syntax-function

	line      string // line-comment prefixes
	lineStart string // line-comment prefixes valid only first on a line
	block     string // block comment: "open close"
	nested    bool   // block comments nest

	quotes    string // characters that delimit one-line strings
	noEscapes bool   // backslash does not escape inside strings
	long      string // multi-line strings: "open close open close …"

	identExtra string // characters allowed in identifiers after the first
	sigils     string // characters that start a variable: $HOME, @array
	prefix     string // characters that start a command or directive: \section, @interface

	caps     bool // Capitalized identifiers are types
	ci       bool // keywords ignore case
	keyColon bool // `key:` and `key =` are highlighted as keys (YAML, TOML)
	lispCall bool // the word after "(" is a function call
}

// Word lists shared by several languages.
const (
	cKw  = "auto break case char class const continue default do double else enum extern float for goto if int long register return short signed sizeof static struct switch typedef union unsigned void volatile while bool true false nullptr NULL namespace using template typename public private protected virtual override new delete this throw try catch final import package extends implements boolean byte super null var val fun let func in is as when object interface"
	jsKw = "async await break case catch class const continue debugger default delete do else export extends false finally for from function if import in instanceof let new null of return static super switch this throw true try typeof undefined var void while yield interface type enum implements readonly as declare keyof"

	cBlock = "/* */"
)

// fallbackLang highlights code in a language kmd does not know.
var fallbackLang = langDef{
	keywords: "if else elif for while return function func fn def class import from const let var true false null nil none None True False and or not in end do then",
	line:     "// #", block: cBlock, quotes: "\"'`", caps: true,
}

// langDefs is every language kmd highlights. The first entry whose aliases
// contain the fence's info string wins.
var langDefs = [...]langDef{
	{name: "Go", aliases: "go golang",
		keywords: "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var nil true false iota",
		line:     "//", block: cBlock, quotes: "\"'`", caps: true},
	{name: "Rust", aliases: "rust rs",
		keywords: "as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while",
		line:     "//", block: cBlock, quotes: "\"", caps: true},
	{name: "Python", aliases: "python py",
		keywords: "and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield self match case",
		line:     "#", quotes: "\"'", caps: true},
	{name: "JavaScript", aliases: "js javascript jsx mjs",
		keywords: jsKw, line: "//", block: cBlock, quotes: "\"'`", caps: true},
	{name: "TypeScript", aliases: "ts typescript tsx",
		keywords: jsKw, line: "//", block: cBlock, quotes: "\"'`", caps: true},
	{name: "C", aliases: "c h",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "C++", aliases: "cpp c++ cc hpp",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Java", aliases: "java",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "C#", aliases: "cs csharp",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Kotlin", aliases: "kotlin kt",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Swift", aliases: "swift",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Scala", aliases: "scala",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Dart", aliases: "dart",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "PHP", aliases: "php",
		keywords: cKw, line: "//", block: cBlock, quotes: "\"'", caps: true},
	{name: "Shell", aliases: "sh bash zsh shell console fish ksh",
		keywords: "if then else elif fi case esac for while until do done in function return local export readonly exit set unset source alias sudo echo cd",
		line:     "#", quotes: "\"'", sigils: "$"},
	{name: "SQL", aliases: "sql mysql postgresql psql sqlite",
		keywords: "select from where and or not insert into values update set delete create table index view drop alter add join left right inner outer full on as group by order having limit offset union all distinct null is in like between case when then else end primary key foreign references default exists returning with",
		line:     "--", block: cBlock, quotes: "'\"", ci: true},
	{name: "Ruby", aliases: "ruby rb",
		keywords: "alias and begin break case class def defined? do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield require puts attr_accessor",
		line:     "#", quotes: "\"'", caps: true},
	{name: "Lua", aliases: "lua",
		keywords: "and break do else elseif end false for function goto if in local nil not or repeat return then true until while",
		line:     "--", quotes: "\"'"},
	{name: "YAML", aliases: "yaml yml",
		keywords: "true false null yes no on off ~", line: "#", quotes: "\"'", keyColon: true},
	{name: "TOML", aliases: "toml",
		keywords: "true false", line: "# ;", quotes: "\"'", keyColon: true},
	{name: "INI", aliases: "ini conf cfg properties env dotenv",
		keywords: "true false", line: "# ;", quotes: "\"'", keyColon: true},
	{name: "JSON", aliases: "json jsonc json5",
		keywords: "true false null", line: "//", quotes: "\""},
	{name: "CSS", aliases: "css scss less sass",
		keywords: "important inherit initial unset none auto", line: "//", block: cBlock, quotes: "\"'"},
	{name: "HTML", aliases: "html vue svelte",
		block: "<!-- -->", quotes: "\"'"},
	{name: "XML", aliases: "xml svg",
		block: "<!-- -->", quotes: "\"'"},
	{name: "Dockerfile", aliases: "dockerfile docker containerfile",
		keywords: "FROM RUN CMD COPY ADD ENV ARG WORKDIR EXPOSE ENTRYPOINT USER LABEL VOLUME HEALTHCHECK SHELL ONBUILD STOPSIGNAL AS",
		line:     "#", quotes: "\"'", sigils: "$"},
	{name: "Makefile", aliases: "make makefile mk",
		keywords: "ifeq ifneq ifdef ifndef else endif include define endef export override .PHONY",
		line:     "#", quotes: "\"'", sigils: "$"},

	// Added with the table-driven highlighter.
	{name: "Haskell", aliases: "haskell hs lhs",
		keywords: "case class data default deriving do else foreign if import in infix infixl infixr instance let module newtype of then type where forall qualified as hiding mdo rec proc",
		types:    "Int Integer Float Double Bool Char String Maybe Either IO Word Ordering",
		builtins: "map filter foldr foldl print putStrLn show read return pure fmap mapM_ length head tail not otherwise error undefined fst snd zip concat Just Nothing Left Right True False",
		line:     "--", block: "{- -}", nested: true, quotes: `"`, identExtra: "'", caps: true},
	{name: "Elm", aliases: "elm",
		keywords: "if then else case of let in type alias module exposing import as port effect where",
		types:    "Int Float Bool Char String List Maybe Result Cmd Sub Html",
		builtins: "Just Nothing Ok Err True False identity always never",
		line:     "--", block: "{- -}", nested: true, quotes: `"`, long: `""" """`, caps: true},
	{name: "Elixir", aliases: "elixir ex exs",
		keywords: "def defp defmodule defmacro defmacrop defstruct defprotocol defimpl defdelegate defguard do end fn if else unless case cond with for try catch rescue after raise receive quote unquote import alias require use when and or not in true false nil",
		builtins: "IO Enum Map List String Kernel Process Agent GenServer Task Keyword",
		line:     "#", quotes: `"'`, long: `""" """ ''' '''`, identExtra: "?!", sigils: "@", caps: true},
	{name: "Erlang", aliases: "erlang erl hrl",
		keywords: "after and andalso band begin bnot bor bsl bsr bxor case catch cond div end fun if let not of or orelse receive rem try when xor",
		builtins: "io format spawn self send register is_atom is_list is_integer length hd tl element setelement lists maps true false ok error undefined",
		line:     "%", quotes: `"`, caps: true},
	{name: "OCaml", aliases: "ocaml ml mli",
		keywords: "and as assert begin class constraint do done downto else end exception external false for fun function functor if in include inherit initializer lazy let match method module mutable new nonrec object of open or private rec sig struct then to true try type val virtual when while with",
		types:    "int float bool char string unit list array option ref exn bytes",
		builtins: "print_string print_endline print_int printf failwith raise ignore fst snd not Some None",
		block:    "(* *)", nested: true, quotes: `"`, caps: true},
	{name: "F#", aliases: "fsharp f# fs fsx fsi",
		keywords: "abstract and as assert base begin class default delegate do done downcast downto elif else end exception extern false finally fixed for fun function global if in inherit inline interface internal lazy let match member module mutable namespace new not null of open or override private public rec return select static struct then to true try type upcast use val void when while with yield async task",
		types:    "int float bool char string unit list array option seq decimal byte int64 obj",
		builtins: "printfn printf sprintf failwith raise ignore fst snd id Some None List Seq Array Map",
		line:     "//", block: "(* *)", nested: true, quotes: `"`, long: `""" """`, caps: true},
	{name: "Clojure", aliases: "clojure clj cljs cljc edn",
		keywords: "def defn defn- defmacro defmulti defmethod defprotocol defrecord deftype fn if if-not when when-not when-let if-let cond condp case do let letfn loop recur try catch finally throw ns require import quote var new and or not nil true false",
		builtins: "println print str map filter reduce first rest cons conj assoc dissoc get get-in update swap! reset! atom deref count apply partial comp vec list hash-map keys vals range into seq",
		line:     ";", quotes: `"`, identExtra: "-?!*<>=/+.'", lispCall: true},
	{name: "Common Lisp", aliases: "lisp common-lisp cl elisp emacs-lisp el",
		keywords: "defun defmacro defvar defparameter defconstant defclass defmethod defgeneric defstruct lambda let let* flet labels if when unless cond case progn prog1 loop do dolist dotimes return return-from block setq setf and or not quote function declare nil t",
		builtins: "car cdr cons list append mapcar funcall apply format print princ length nth eq eql equal null",
		line:     ";", block: "#| |#", nested: true, quotes: `"`, identExtra: "-?!*<>=/+", lispCall: true},
	{name: "Scheme", aliases: "scheme scm ss racket rkt",
		keywords: "define define-syntax define-record-type lambda let let* letrec letrec* if cond case when unless else begin do and or not quote quasiquote set! delay syntax-rules import library export",
		builtins: "car cdr cons list append map for-each apply display newline length null? pair? eq? equal? vector string-append",
		line:     ";", block: "#| |#", nested: true, quotes: `"`, identExtra: "-?!*<>=/+", lispCall: true},
	{name: "R", aliases: "r rscript",
		keywords: "if else repeat while function for in next break return TRUE FALSE NULL Inf NaN NA NA_integer_ NA_real_ NA_character_ library require",
		builtins: "c list data.frame vector matrix print cat paste paste0 length sum mean apply lapply sapply seq rep",
		line:     "#", quotes: "\"'`", identExtra: "."},
	{name: "Julia", aliases: "julia jl",
		keywords: "abstract baremodule begin break catch const continue do else elseif end export false finally for function global if import in isa let local macro module mutable primitive quote return struct true try type using where while",
		types:    "Int Int8 Int16 Int32 Int64 UInt8 UInt64 Float32 Float64 Bool Char String Vector Matrix Array Dict Any Nothing",
		builtins: "println print length push! pop! map filter reduce collect zeros ones size typeof nothing missing",
		line:     "#", block: "#= =#", nested: true, quotes: `"`, long: `""" """`, identExtra: "!", caps: true},
	{name: "Perl", aliases: "perl pl pm",
		keywords: "my our local sub if elsif else unless while until for foreach do last next redo return use no require package BEGIN END and or not eq ne lt gt le ge cmp qw undef",
		builtins: "print printf say open close die warn chomp split join push pop shift unshift keys values exists delete defined scalar ref bless",
		line:     "#", quotes: `"'`, sigils: "$@%"},
	{name: "PowerShell", aliases: "powershell ps1 psm1 psd1 pwsh posh",
		keywords: "begin break catch class continue data do dynamicparam else elseif end enum exit filter finally for foreach function if in param process return switch throw trap try until using while",
		builtins: "Write-Host Write-Output Write-Error Get-Item Get-ChildItem Set-Location Get-Content Set-Content Where-Object ForEach-Object Select-Object New-Object Invoke-WebRequest",
		line:     "#", block: "<# #>", quotes: `"'`, identExtra: "-", sigils: "$", ci: true},
	{name: "Groovy", aliases: "groovy gradle gvy",
		keywords: "abstract as assert break case catch class const continue def default do else enum extends final finally for goto if implements import in instanceof interface new package private protected public return static super switch this throw throws trait try var void while true false null",
		types:    "int long short byte float double boolean char String Object List Map",
		builtins: "println print each collect find findAll inject with apply plugins dependencies repositories implementation",
		line:     "//", block: cBlock, quotes: `"'`, long: `""" """ ''' '''`, sigils: "$", caps: true},
	{name: "Zig", aliases: "zig",
		keywords: "addrspace align allowzero and anyframe anytype asm async await break callconv catch comptime const continue defer else enum errdefer error export extern fn for if inline linksection noalias noinline nosuspend opaque or orelse packed pub resume return struct suspend switch test threadlocal try union unreachable usingnamespace var volatile while true false null undefined",
		types:    "i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f16 f32 f64 f128 bool void noreturn type anyerror anyopaque comptime_int comptime_float",
		builtins: "@import @as @intCast @floatCast @ptrCast @bitCast @sizeOf @TypeOf @This @panic @compileError @field @memcpy @memset",
		line:     "//", quotes: `"'`, prefix: "@", caps: true},
	{name: "Nim", aliases: "nim nims nimble",
		keywords: "addr and as asm bind block break case cast concept const continue converter defer discard distinct div do elif else end enum except export finally for from func if import in include interface is isnot iterator let macro method mixin mod nil not notin object of or out proc ptr raise ref return shl shr static template try tuple type using var when while xor yield true false",
		types:    "int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 float float32 float64 bool char string cstring seq array set openArray void auto",
		builtins: "echo len add high low inc dec assert newSeq",
		line:     "#", block: "#[ ]#", nested: true, quotes: `"'`, long: `""" """`, caps: true},
	{name: "Nix", aliases: "nix",
		keywords: "let in with rec inherit if then else assert import or true false null",
		builtins: "builtins derivation fetchurl fetchTarball mkDerivation stdenv lib pkgs callPackage toString map",
		line:     "#", block: cBlock, quotes: `"`, long: "'' ''", identExtra: "-'", keyColon: true},
	{name: "HCL", aliases: "hcl tf tfvars terraform",
		keywords: "resource data variable output module provider locals terraform backend for in if true false null dynamic content each count",
		builtins: "var local path self file lookup merge concat length element join split format jsonencode",
		line:     "# //", block: cBlock, quotes: `"`, keyColon: true},
	{name: "Protocol Buffers", aliases: "protobuf proto proto3",
		keywords: "syntax package import option message enum service rpc returns stream oneof map repeated optional required reserved extend extensions to max public weak true false",
		types:    "double float int32 int64 uint32 uint64 sint32 sint64 fixed32 fixed64 sfixed32 sfixed64 bool string bytes",
		line:     "//", block: cBlock, quotes: `"'`, caps: true},
	{name: "GraphQL", aliases: "graphql gql",
		keywords: "query mutation subscription fragment on type input enum interface union scalar schema extend implements directive repeatable true false null",
		types:    "Int Float String Boolean ID",
		line:     "#", quotes: `"`, long: `""" """`, sigils: "$", prefix: "@", caps: true},
	{name: "Objective-C", aliases: "objc objective-c objectivec m mm",
		keywords: cKw + " id self nil YES NO SEL IMP BOOL instancetype nonatomic atomic strong weak copy assign readonly readwrite @interface @implementation @end @property @synthesize @protocol @class @selector @autoreleasepool @try @catch @finally @import",
		line:     "//", block: cBlock, quotes: `"'`, prefix: "@", caps: true},
	{name: "Assembly", aliases: "asm assembly nasm masm gas s x86asm armasm",
		keywords: "mov movzx movsx lea push pop call ret jmp je jne jz jnz jg jge jl jle ja jb cmp test add sub mul imul div idiv inc dec and or xor not shl shr sal sar nop int syscall ldr str ldp stp bl bx b cbz cbnz",
		builtins: "rax rbx rcx rdx rsi rdi rsp rbp r8 r9 r10 r11 r12 r13 r14 r15 eax ebx ecx edx esi edi esp ebp ax bx cx dx al bl cl dl sp lr pc x0 x1 x2 x3 x29 x30 w0 w1",
		line:     "; # //", block: cBlock, quotes: `"'`, prefix: ".%", ci: true},
	{name: "Vim script", aliases: "vim vimscript viml",
		keywords:  "function endfunction func endfunc if elseif else endif for endfor while endwhile try catch finally endtry return let unlet const call execute set setlocal map nnoremap inoremap vnoremap noremap autocmd augroup command abort silent",
		builtins:  "exists has expand strlen len empty get keys values filter map add remove printf echo echom",
		lineStart: `"`, quotes: `'"`, sigils: "&"},
	{name: "Batch", aliases: "batch bat cmd dosbatch",
		keywords:  "if else for in do goto call exit set setlocal endlocal echo off on not exist defined errorlevel shift pause start cd del copy move nul",
		lineStart: "rem ::", quotes: `"`, noEscapes: true, sigils: "%", ci: true},
	{name: "LaTeX", aliases: "latex tex",
		line: "%", prefix: `\`},
	{name: "CMake", aliases: "cmake",
		keywords: "if elseif else endif foreach endforeach while endwhile function endfunction macro endmacro return break continue set unset option project add_executable add_library target_link_libraries target_include_directories target_compile_options find_package include message install cmake_minimum_required AND OR NOT ON OFF TRUE FALSE",
		line:     "#", quotes: `"`, sigils: "$", ci: true},
	{name: "Nginx", aliases: "nginx nginxconf",
		keywords: "server location listen server_name root index return rewrite proxy_pass proxy_set_header upstream include try_files error_page access_log error_log ssl_certificate ssl_certificate_key http events worker_processes worker_connections add_header if set gzip on off",
		line:     "#", quotes: `"'`, sigils: "$"},
	{name: "D", aliases: "d dlang",
		keywords: "abstract alias align asm assert auto body break case cast catch class const continue debug default delegate delete deprecated do else enum export extern false final finally for foreach foreach_reverse function goto if immutable import in inout interface invariant is lazy mixin module new nothrow null out override package pragma private protected public pure ref return scope shared static struct super switch synchronized template this throw true try typeid typeof union unittest version while with",
		types:    "bool byte ubyte short ushort int uint long ulong float double real char wchar dchar string void size_t",
		line:     "//", block: "/+ +/", nested: true, quotes: "\"'`", caps: true},
	{name: "Pascal", aliases: "pascal pas delphi objectpascal lpr dpr",
		keywords: "and array as begin case class const constructor destructor div do downto else end except exit file finally for function goto if implementation in inherited interface is label mod nil not object of or out packed procedure program property raise record repeat set shl shr then to try type unit until uses var while with xor true false",
		types:    "integer real boolean char string byte word longint cardinal double single",
		builtins: "writeln write readln read length inc dec ord chr",
		line:     "//", block: "{ }", quotes: "'", noEscapes: true, ci: true},
	{name: "Fortran", aliases: "fortran f90 f95 f03 f08 for f",
		keywords: "program end subroutine function module use implicit none integer real double precision complex logical character parameter dimension allocatable intent in out inout if then else elseif endif do enddo while call return contains type select case default print write read stop allocate deallocate result",
		builtins: "abs sqrt sin cos exp log size sum maxval minval present trim len",
		line:     "!", quotes: `"'`, noEscapes: true, ci: true},
	{name: "Awk", aliases: "awk gawk mawk nawk",
		keywords: "BEGIN END if else while for do break continue next nextfile exit return delete in function getline print printf",
		builtins: "length substr index split sub gsub match sprintf tolower toupper system close NR NF FS OFS RS ORS FILENAME",
		line:     "#", quotes: `"`, sigils: "$"},
	{name: "Tcl", aliases: "tcl tk",
		keywords: "proc set if elseif else for foreach while switch return break continue expr puts global upvar uplevel namespace package source catch error eval incr list lappend lindex llength string array dict",
		line:     "#", quotes: `"`, sigils: "$"},
	{name: "Solidity", aliases: "solidity sol",
		keywords: "pragma solidity import contract interface library abstract function modifier event emit struct enum mapping public private internal external view pure payable constant immutable returns return if else for while do break continue new delete memory storage calldata using is override virtual constructor fallback receive require revert assert true false",
		types:    "address bool string bytes byte int uint int8 int256 uint8 uint256 bytes32",
		builtins: "msg block tx this keccak256 abi",
		line:     "//", block: cBlock, quotes: `"'`, caps: true},
	{name: "Crystal", aliases: "crystal cr",
		keywords: "abstract alias annotation as begin break case class def do else elsif end ensure enum extend false for fun if in include instance_sizeof is_a? lib macro module next nil of out pointerof private protected require rescue return select self sizeof struct super then true type typeof uninitialized union unless until when while with yield",
		builtins: "puts print p pp raise loop spawn",
		line:     "#", quotes: `"'`, identExtra: "?!", sigils: "@", caps: true},
	{name: "Verilog", aliases: "verilog v sv systemverilog vh svh",
		keywords: "module endmodule input output inout wire reg logic always always_ff always_comb assign begin end if else case endcase default for while parameter localparam initial posedge negedge function endfunction task endtask generate endgenerate genvar integer",
		line:     "//", block: cBlock, quotes: `"`, prefix: "$`"},
}
