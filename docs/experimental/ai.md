# Experimental: local AI assistant

> **Warning.** This feature is experimental. It is not in the released
> binaries, it is not covered by the README or the man page, and it may
> change or be removed at any time. Use it only if you build kmd yourself.

The goal is a local AI assistant inside kmd: summaries of a document or of
the section on screen, questions about it, and descriptions of images. The
model runs on your own machine, in a server you install and start; kmd only
talks to it. The plan is tracked in the GitHub issues labelled
[`experimental-ai`](https://github.com/DipaolaFederico/kmd/labels/experimental-ai).

## Status

The build tag, the configuration keys, the CI job and the client in
`internal/ai` exist. Nothing in kmd calls the client yet: the first users
are `--ai-check` and `--ai-setup` (#48).

## Building

```sh
make build-ai    # bin/kmd-ai, built with -tags kmdai; --version ends in +kmdai
make test-ai     # vet, staticcheck, dead code, race tests, coverage,
                 # govulncheck and gosec with the tag
```

or `go build -tags kmdai .`. A build without the tag contains none of this
code and makes no network requests, as the README promises.

## Configuration

In the usual config file (`kmd --config` shows where it is and, in a tagged
build, the effective values):

| Key           | Default                                       | Meaning                       |
|---------------|-----------------------------------------------|-------------------------------|
| `ai`          | `off`                                         | `on` enables the assistant    |
| `ai-url`      | `unix://$XDG_RUNTIME_DIR/kmd-llama.sock`, or `http://127.0.0.1:8091` without `XDG_RUNTIME_DIR` | the local server |
| `ai-key-file` | `~/.config/kmd/ai-key` (honours `XDG_CONFIG_HOME`) | absolute path of the server's API key, if it needs one; a relative path is a config error |

A build without the tag accepts these keys, so one config file serves both,
and prints a one-line notice when `ai = on`.

The reference model is
[LiquidAI/LFM3-VL-2B](https://huggingface.co/LiquidAI/LFM3-VL-2B) (GGUF
`Q4_0`) served by llama.cpp's `llama serve`; measurements on a CPU are in
issue #44.

## Threat model

The document is untrusted, so the model's answer is untrusted too: a hostile
document can steer the model (prompt injection), and #44 showed that it
sometimes succeeds. These are the rules the coming pull requests implement
(#46 to #51):

- **kmd never installs, downloads or starts a model or a server.** The user
  runs the server; kmd is only a client.
- **Loopback only.** kmd connects only to a Unix socket or to `127.0.0.1`,
  `::1` or `localhost` (resolved without DNS), with its own minimal HTTP
  client: no TLS, no proxies, no redirects. The destination comes from the
  configuration, never from a document.
- **The model gets no capabilities.** Requests never contain `tools`; the
  document is sent as delimited data after a fixed system prompt.
- **Every answer is hostile text.** It goes through kmd's Markdown pipeline,
  which removes terminal escapes and unsafe URLs exactly as for documents.
- **Bounded.** Input size, output tokens, response size, line length and
  time are all capped; `Esc` cancels.

  | Limit | Value |
  |-------|-------|
  | connect | 5 s |
  | from sending the request to the end of the headers | 5 s |
  | between two reads of the body | 60 s (llama.cpp pings every 30 s) |
  | whole exchange | 180 s |
  | response body | 8 MiB, an error past it (never a silently cut answer) |
  | a header, chunk-size or event line, or one event's data | 64 KiB |
  | response headers, trailers | 100 lines |

  An `ai-url` with another host, TLS, credentials, a path or a query is a
  configuration error; a redirect is an error.
- **Files.** The only files read are the configured key file and the images
  kmd would already read to draw them.

A Unix socket in `$XDG_RUNTIME_DIR` is the default because only its owner
can create it or connect to it. A loopback TCP port is weaker in both
directions:

- any local user can connect to the server, which is what the optional key
  file is for;
- any local user can listen on the port first, when the server is not
  running, and receive the document and the key. kmd cannot tell from the
  connection who is listening.

kmd sends the key over TCP too. Without `XDG_RUNTIME_DIR` (macOS, for one)
TCP is the default, and there the key is what keeps other local users off
the server; withholding it would make the key useless where it matters
most. A squatter learns a key to the user's own local server, while the
document, the more valuable of the two, would reach it with or without
the key. Checking the listener's owner is possible only on some systems
(`/proc/net/tcp` on Linux) and racy, so kmd does not; instead
`--ai-check` (#48) will say when the connection is TCP and recommend the
socket.

The key file is read only from an absolute path. kmd refuses it unless it
is a regular file of at most 4 KiB that only its owner can read (`chmod
600`; not checked on Windows), holding a single line of printable ASCII
without spaces (it goes into an HTTP header). A missing key file means no
key.

## Graduation

The feature stays behind the tag until a decision, on evidence, to make it
a regular feature. The criteria and the checklist are in issue #53; this
section will record the decision.
