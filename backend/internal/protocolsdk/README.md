# Official SDK compatibility in Go

The account login state machine, outbound requests, proxies and persistence run
in Go. The current public official SDK is interpreted by `goja` in memory.
There is no Node/Python executable, external worker, or Chromium dependency.
`environment.js` defines host API compatibility; it is embedded into the Go
binary and is not an independent business runtime.

The Go boundary exposes only random bytes, URL parsing, timers and a caller-owned
HTTP callback. The workbench caller restricts that callback to one fixed official
endpoint. SDK execution has a deadline, cancellation interrupts JavaScript, and
errors do not expose scripts, tokens or response bodies. Cookies and credentials
are never written to frontend storage. Each new SDK invocation has fresh memory.

`goja` (MIT) supplies the maintained pure Go ECMAScript implementation. uTLS
(BSD-3-Clause) supplies the TLS ClientHello profile in the workbench HTTP layer.
The configured Chrome 133 profile is supported by uTLS 1.8.2 and differs from the
reference tool's default Chrome 146. New SDK APIs and challenge types may need
compatibility updates; failures stop authorization instead of falling back to
interactive browsers or returning a fabricated security token.
