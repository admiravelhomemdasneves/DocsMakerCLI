#!/usr/bin/env bash
# End-to-end demo AND regression check of docsmaker's guarantees, run against
# a throwaway git repository made from testdata/sample-project:
#
#   1. nothing is written inline until the git filter is set up
#   2. a planted credential is blocked / redacted and never reaches any output
#   3. AI-written comments land in the sources, but never in a commit
#   4. re-running is idempotent and incremental (cache)
#   5. remote Ollama hosts and "cloud" models are refused
#
# Needs a running local Ollama with a model (see `docsmaker model status`).
# Works in bash on Linux, macOS and Git Bash on Windows. Exits non-zero if any
# guarantee does not hold.
#
#   scripts/demo.sh [path/to/docsmaker]
#
#   DEMO_MODEL=...   model to use (default: qwen2.5-coder:1.5b if present, else the tool's default)
#   DEMO_PAUSE=1     wait for Enter between steps (for presenting)
#   DEMO_OFFLINE=1   add a step where you disconnect the network and run again
#   DEMO_KEEP=1      keep the temporary repository and print where it is
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${DEMO_SRC:-$ROOT/testdata/sample-project}"

EXE="${1:-}"
if [ -z "$EXE" ]; then
  for c in "$ROOT/docsmaker" "$ROOT/docsmaker.exe"; do [ -x "$c" ] && EXE="$c" && break; done
fi
[ -z "$EXE" ] && EXE="$(command -v docsmaker || true)"
[ -n "$EXE" ] && [ -x "$EXE" ] || { echo "docsmaker binary not found; pass its path as the first argument." >&2; exit 2; }
[ -d "$SRC" ] || { echo "sample project not found at $SRC" >&2; exit 2; }

failures=0
say()  { printf '\n\033[1m== %s\033[0m\n' "$*"; if [ "${DEMO_PAUSE:-}" = 1 ]; then read -r -p "   [enter] " _; fi; }
ok()   { printf '   \033[32mPASS\033[0m %s\n' "$*"; }
bad()  { printf '   \033[31mFAIL\033[0m %s\n' "$*"; failures=$((failures + 1)); }
check() { local desc=$1; shift; if "$@"; then ok "$desc"; else bad "$desc"; fi; }
show() { sed 's/^/     | /' | head -"${1:-12}"; }

contains() { grep -qE -- "$2" <<<"$1"; }

# --- preflight -------------------------------------------------------------
say "0. Preflight: local Ollama and model"
status="$("$EXE" model status 2>&1)"; echo "$status" | show 6
contains "$status" 'running: +true' || { echo "Ollama is not running; start it (or run: $EXE model pull) and retry." >&2; exit 2; }
MODEL="${DEMO_MODEL:-}"
if [ -z "$MODEL" ]; then
  if "$EXE" model list 2>/dev/null | grep -q '^qwen2.5-coder:1.5b'; then MODEL=qwen2.5-coder:1.5b; fi
fi
MODEL_ARGS=(); [ -n "$MODEL" ] && MODEL_ARGS=(--model "$MODEL")
"$EXE" model list 2>/dev/null | grep -q "^${MODEL:-qwen2.5-coder:7b}" || {
  echo "model ${MODEL:-qwen2.5-coder:7b} is not downloaded; run: $EXE model pull ${MODEL:-}" >&2; exit 2; }
echo "   using model: ${MODEL:-default}"

# --- throwaway repo ----------------------------------------------------------
W="$(mktemp -d)"; LOGS="$(mktemp -d)"
cleanup() { if [ "${DEMO_KEEP:-}" = 1 ]; then echo "kept: $W"; else rm -rf "$W" "$LOGS"; fi; }
trap cleanup EXIT

cp -R "$SRC"/. "$W"/
rm -rf "$W/.docsmaker" "$W/target"
git_() { git -C "$W" -c user.name=demo -c user.email=demo@example.invalid "$@"; }
git_ init -q && git_ config core.autocrlf false
git_ add -A && git_ commit -qm "base" && BASE="$(git_ rev-parse HEAD)"
GREETER=src/main/java/com/example/greeting/Greeter.java

# --- 1. structure, no AI ------------------------------------------------------
say "1. analyze: structure only, no AI involved"
out="$("$EXE" analyze "$W" 2>&1)"; echo "$out" | show 8
check "found the sample classes" contains "$out" 'class Greeter|interface Greeter'

# --- 2. inline is refused before init ----------------------------------------
say "2. --mode inline is refused until the git filter is set up"
out="$("$EXE" doc "$W" --mode inline --level class --target com.example.greeting.Greeter "${MODEL_ARGS[@]}" 2>&1)"; rc=$?
echo "$out" | show 4
check "exit code is non-zero" test "$rc" -ne 0
check "tells the user to run init" contains "$out" 'docsmaker init'
check "sources untouched" git_ diff --quiet

# --- 3. init -------------------------------------------------------------------
say "3. docsmaker init (local to this clone, idempotent)"
"$EXE" init "$W" | show 6
check "attributes are in .git/info, not a tracked file" test ! -e "$W/.gitattributes"
out="$("$EXE" init "$W")"
check "second init changes nothing" contains "$out" 'already set'

# --- 4. secrets ----------------------------------------------------------------
say "4. A planted credential (DatabaseConfig) is caught before the model sees it"
out="$("$EXE" doc "$W" --level class --target com.example.config.DatabaseConfig --yes "${MODEL_ARGS[@]}" 2>&1)"; rc=$?
echo "$out" | show 6
check "--yes blocks the run" test "$rc" -ne 0
check "the finding is shown masked" contains "$out" 'jdb\*\*\*'
check "the secret value is not printed" bash -c '! grep -q hunter2 <<<"$1"' _ "$out"

# --- 5. inline generation --------------------------------------------------------
say "5. Generate docs inline for the whole project (answering 'redact' at the secret prompt)"
printf 'r\n%.0s' {1..30} | "$EXE" doc "$W" --mode inline --level project "${MODEL_ARGS[@]}" >"$LOGS/out.txt" 2>"$LOGS/err.txt"
rc=${PIPESTATUS[1]}
grep '=== inline' "$LOGS/out.txt" | show 2
check "run succeeded" test "$rc" -eq 0
blocks="$(grep -rc 'docsmaker:ai-begin' "$W/src" | awk -F: '{s+=$2} END {print s+0}')"
check "AI comments were written into the sources ($blocks blocks)" test "$blocks" -gt 0
ai_text="$(find "$W/src" -name '*.java' -exec awk '/docsmaker:ai-begin/{f=1} f{print} /docsmaker:ai-end/{f=0}' {} +)"
check "the secret is not in any AI comment" bash -c '! grep -q hunter2 <<<"$1"' _ "$ai_text"
check "the secret is not in the console output" bash -c '! grep -q hunter2 "$1" "$2"' _ "$LOGS/out.txt" "$LOGS/err.txt"
check "the secret is not in the cache or audit log" bash -c '! grep -rq hunter2 "$1/.docsmaker"' _ "$W"
check "the decision was audited (masked)" grep -q 'decision=redact' "$W/.docsmaker/audit.log"
echo "   sample of what was written to $GREETER:"; sed -n 1,12p "$W/$GREETER" | show 12

# --- 6. idempotent -----------------------------------------------------------------
say "6. Running again is a no-op served from the cache"
start=$(date +%s)
out="$(printf 'r\n%.0s' {1..30} | "$EXE" doc "$W" --mode inline --level project "${MODEL_ARGS[@]}" 2>/dev/null | grep '=== inline')"
echo "$out" | show 2; echo "   took $(( $(date +%s) - start ))s"
check "nothing inserted or replaced" contains "$out" ' 0 inserted, 0 replaced'

# --- 7. nothing reaches git -----------------------------------------------------------
say "7. What git sees: no AI lines, even though the files contain them"
check "git sees no modification" git_ diff --quiet HEAD
git_ add -A
check "nothing to commit after 'git add -A'" git_ diff --cached --quiet

# A human edits a file that also carries AI comments, then commits.
sed -i 's|    String greet(String name);|    String greet(String name);\n\n    default String shout(String name) { return greet(name).toUpperCase(); }|' "$W/$GREETER"
git_ add -A && git_ commit -qm "human: add shout"
added="$(git_ show --numstat --format= HEAD | awk '{a+=$1; d+=$2} END {print a" "d}')"
check "the commit holds only the human's 2 lines (added deleted = '$added')" test "$added" = "2 0"
check "the committed file has the human change" bash -c 'git -C "$1" show "HEAD:$2" | grep -q shout' _ "$W" "$GREETER"
check "no committed .java file contains a docsmaker marker" bash -c '! git -C "$1" grep -q docsmaker HEAD -- "*.java"' _ "$W"
check "the working tree still has the AI comments" grep -q 'docsmaker:ai-begin' "$W/$GREETER"

# --- 8. incremental ------------------------------------------------------------------------
say "8. After the human edit, only what changed is regenerated"
out="$(printf 'r\n%.0s' {1..30} | "$EXE" doc "$W" --mode inline --level project "${MODEL_ARGS[@]}" 2>/dev/null | grep '=== inline')"
echo "$out" | show 2
check "the new method is documented, its class summary refreshed, the rest untouched" contains "$out" ' 1 inserted, [01] replaced, 1[34] unchanged'
git_ add -A
check "still nothing AI-written staged" git_ diff --cached --quiet

# --- 8b. allowlist -----------------------------------------------------------------------
say "8b. A false positive can be allowlisted once and is then never asked again"
CLS=(--level class --target com.example.config.DatabaseConfig)
printf 'w\n' | "$EXE" doc "$W" "${CLS[@]}" "${MODEL_ARGS[@]}" >/dev/null 2>&1
check "answering [w] succeeded" test "$?" -eq 0
check "only a hash was stored, not the value" bash -c '! grep -q hunter2 "$1/.docsmaker/allowlist.json" && grep -q sha256 "$1/.docsmaker/allowlist.json"' _ "$W"
"$EXE" doc "$W" "${CLS[@]}" --yes "${MODEL_ARGS[@]}" >/dev/null 2>&1
check "--yes now passes without a prompt" test "$?" -eq 0
id="$("$EXE" allowlist list --project "$W" | awk 'NR==2{print $1}')"
"$EXE" allowlist remove "$id" --project "$W" >/dev/null
"$EXE" doc "$W" "${CLS[@]}" --yes "${MODEL_ARGS[@]}" >/dev/null 2>&1
check "after removing the entry it is blocked again" test "$?" -ne 0

# --- 9. safety rails -------------------------------------------------------------------------
say "9. The 'nothing leaves the machine' rails"
out="$(OLLAMA_HOST=10.0.0.5:11434 "$EXE" model status 2>&1)"; rc=$?
echo "$out" | show 2
check "a remote OLLAMA_HOST is refused" bash -c 'test "$1" -ne 0 && grep -q "not this machine" <<<"$2"' _ "$rc" "$out"
out="$("$EXE" model pull gpt-oss:120b-cloud 2>&1)"; rc=$?
echo "$out" | show 2
check "a cloud model is refused" bash -c 'test "$1" -ne 0 && grep -q "cloud model" <<<"$2"' _ "$rc" "$out"
if [ -d "$ROOT/internal" ]; then
  users="$(grep -rl --include='*.go' '"net/http"' "$ROOT/internal" "$ROOT/cmd" | grep -v '_test.go' | sed "s|$ROOT/||" | sort)"
  echo "$users" | sed 's/^/     | net\/http used by: /'
  check "only internal/llm talks HTTP" bash -c '! grep -qv "^internal/llm/" <<<"$1"' _ "$users"
fi

# --- 10. optional offline run --------------------------------------------------------------------
if [ "${DEMO_OFFLINE:-}" = 1 ]; then
  say "10. Offline: disconnect Wi-Fi/Ethernet now, then press Enter"
  read -r -p "   [enter once the network is off] " _
  rm -rf "$W/.docsmaker/cache"
  printf 'r\n%.0s' {1..30} | "$EXE" doc "$W" --level class --target com.example.greeting.Greeter "${MODEL_ARGS[@]}" >/dev/null 2>&1
  check "generation works with no network (fresh cache, so the model really ran)" test "$?" -eq 0
fi

echo
if [ "$failures" -eq 0 ]; then printf '\033[32mAll guarantees held.\033[0m\n'; else printf '\033[31m%d check(s) failed.\033[0m\n' "$failures"; exit 1; fi
