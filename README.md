# docsmaker

Generates documentation for your code (class and method descriptions, package
overviews, a project README) and recovers the **business rules** buried in it,
using an AI model that runs **on your own machine**. Java today; the analyzer
is pluggable for other languages.

- **Nothing leaves your machine.** It talks only to a local [Ollama](https://ollama.com).
  A remote `OLLAMA_HOST` and Ollama "cloud" models are refused. Your code is
  never uploaded or used to train anything.
- **Sensitive content is caught first.** Passwords, keys, tokens and connection
  strings are detected and you decide what happens before they can reach the
  model or any output.
- **AI text never reaches the shared repository.** You can write the docs
  straight into your source files; a git filter strips those lines on commit.
- **One executable.** No runtime to install (Ollama aside, which `docsmaker`
  sets up for you, asking first).

## Install

Download the archive for your OS from the releases page, extract it, and run
the installer inside:

| OS | Command |
|----|---------|
| Windows | `.\install.ps1` |
| Linux / macOS | `./install.sh` |

It copies the binary into your user folder and (Windows) adds it to your user
`PATH`. No administrator rights. Then, once:

```
docsmaker model pull     # installs/starts Ollama and downloads the model; asks before each step
```

## Use

```
docsmaker analyze [path]                 # structure only, no AI
docsmaker doc [path]                     # project README + every level, printed to the console
docsmaker doc --level class --target com.acme.billing.Invoice
docsmaker doc --level method --target com.acme.billing.Invoice#total
docsmaker doc --mode sidecar             # write to .docsmaker/doc/ instead of printing
docsmaker doc --mode inline              # write /** */ comments into the sources (see below)
```

`path` defaults to the current directory. Levels: `method`, `class`, `package`,
`project`. Results are cached in `.docsmaker/`, so re-running is fast and only
changed code is sent to the model again. Use `--model` to pick another local
model and `--format json` for machine-readable output.

### Business rules

Technical docs say *what* code does; in legacy code the missing part is *why*
(that `if (total < 20)`, that limit of 500). `docsmaker rules` reads the methods
that hold decisions, thresholds and validations and asks the local model to
state the rule each one implements:

```
docsmaker rules                              # report printed to the console
docsmaker rules --target com.acme.billing    # one package, class or method
docsmaker rules --mode sidecar               # also write .docsmaker/rules/RULES.md + one file per class
docsmaker rules --min-confidence medium      # hide the least trustworthy rules
docsmaker rules --format json                # machine-readable
```

Each rule comes with the code lines it rests on, a confidence level and the
questions only someone who knows the business can answer. The model sees only
the code, so the *why* is always presented as an inferred hypothesis, and a
rule whose quoted evidence is not really in the code is capped at low
confidence. Sources are never modified, and the same secret screening as `doc`
applies. Methods with no decisions (getters, assignments) are not analysed;
`--min-score` and `--max-methods` control how many are.

### Sensitive information

Every prompt (before the model) and every generated text (before it is kept)
is screened. When something looks sensitive you are asked:

| Answer | Effect |
|--------|--------|
| `r` (default) | redact it (`[REDACTED:kind]`) and continue |
| `s` | skip that class/method |
| `w` | not a secret: include it and never ask again (see below) |
| `a` | include it once, as is |
| `q` | abort |

`--secrets strict` and `--yes` never ask: any finding stops the run.
A value that is not a secret can be allowlisted by answering `w`: only its hash is
stored, in `.docsmaker/allowlist.json` (local; do not commit), and it is never asked
about again, even under `--secrets strict` or `--yes`. Manage entries with
`docsmaker allowlist list`, `remove <id>` and `add-pattern <regex>`.

`--secrets off` disables screening (prints a warning; for synthetic code only).
Decisions are logged, with masked values only, in `.docsmaker/audit.log`.
Detection is pattern- and entropy-based: expect occasional false positives and
misses; the prompt exists so a person has the last word.

### Keeping AI-written lines out of commits (`--mode inline`)

Once per clone:

```
docsmaker init
```

This configures a git *clean* filter for this clone only (`.git/info/attributes`
and `.git/config`; nothing tracked changes, pass `--shared` to use
`.gitattributes` instead). From then on `docsmaker doc --mode inline` adds
comments wrapped in `docsmaker:ai-begin` / `docsmaker:ai-end` markers, and git
removes those blocks from whatever you stage, even inside a file that
otherwise contains your own changes. `--mode inline` refuses to run in a git
repository where `init` has not been done.

Good to know:

- Hand-written doc comments are never touched; earlier AI blocks are refreshed in place.
- Each edit is re-parsed before it is saved; a file that would stop compiling is left unchanged.
- The AI comments exist only in your working tree. A `git pull` that changes a
  file, or a stash, drops them; run the command again to restore them (cached).
- If `docsmaker` is moved or reinstalled elsewhere, run `docsmaker init` again.
  The filter is marked *required*, so a missing binary makes `git add` fail
  instead of silently committing AI lines.

## Commands

| Command | Purpose |
|---------|---------|
| `analyze` | structural analysis, no AI |
| `doc` | generate documentation |
| `rules` | extract the business rules the code implements |
| `init` | wire the git filter in this clone |
| `model status, list, pull` | manage the local Ollama and models |
| `allowlist list, remove, add-pattern` | manage values declared not to be secrets |

## Build from source

Needs Go 1.26+ and a C compiler (the tree-sitter grammar is C). It cannot be
cross-compiled, so build on each target OS.

```
scripts/build.sh          # Linux / macOS
scripts\build.ps1         # Windows (gcc must be on PATH, installed in a path without spaces)
```

Output goes to `dist/`: an archive, its checksum, and a smoke-tested binary.
CI builds and releases for Windows, Linux and macOS (`.github/workflows`).

## Limits

- Java only; nested classes, generics and import resolution are not modelled.
- Quality depends on the local model: bigger models write better docs and need more RAM.
- Generated text is a draft. Review it before relying on it.
- Extracted business rules are the model's reading of the code alone: the
  *why* is a guess, each method is read in isolation, and constants or tables
  defined elsewhere are not seen.

## Verify the guarantees yourself

`scripts/demo.sh` runs the whole flow on a throwaway git repository built from
`testdata/sample-project` and checks, with real commits, that: inline docs are
refused before `init`; a planted credential never reaches a prompt, an output,
the cache or a commit; AI comments never appear in `git show HEAD`; re-runs are
cached; remote hosts and cloud models are refused. It exits non-zero if any of
that stops being true.

```
scripts/demo.sh            # needs a running local Ollama with a model
DEMO_PAUSE=1 scripts/demo.sh   # pause between steps, for presenting
```
