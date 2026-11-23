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

Only the scaffolding exists: the build tag, the configuration keys and the
CI job. kmd does not contact the server yet.

## Building

```sh
make build-ai    # bin/kmd-ai, built with -tags kmdai
make test-ai     # vet, staticcheck, race tests and coverage with the tag
```

or `go build -tags kmdai .`. A build without the tag contains none of the
code below and makes no network requests, as the README promises.

## Configuration

In the usual config file (`kmd --config` shows where it is and, in a tagged
build, the effective values):

| Key           | Default                                       | Meaning                       |
|---------------|-----------------------------------------------|-------------------------------|
| `ai`          | `off`                                         | `on` enables the assistant    |
| `ai-url`      | `unix://$XDG_RUNTIME_DIR/kmd-llama.sock`, or `http://127.0.0.1:8091` without `XDG_RUNTIME_DIR` | the local server |
| `ai-key-file` | `~/.config/kmd/ai-key` (honours `XDG_CONFIG_HOME`) | absolute path of the server's API key, if it needs one |

A build without the tag accepts these keys, so one config file serves both,
and prints a one-line notice when `ai = on`.

The reference model is
[LiquidAI/LFM3-VL-2B](https://huggingface.co/LiquidAI/LFM3-VL-2B) (GGUF
`Q4_0`) served by llama.cpp's `llama serve`; measurements on a CPU are in
issue #44.

## Threat model

The document is untrusted, so the model's answer is untrusted too: a hostile
document can steer the model (prompt injection), and #44 showed that it
sometimes succeeds. The design follows from that:

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
- **Files.** The only files read are the configured key file and the images
  kmd would already read to draw them.

A Unix socket in `$XDG_RUNTIME_DIR` is the default because only its owner
can connect to it; any local user can connect to a TCP port on loopback,
which is what the optional key file is for.

## Graduation

The feature stays behind the tag until a decision, on evidence, to make it
a regular feature. The criteria and the checklist are in issue #53; this
section will record the decision.
