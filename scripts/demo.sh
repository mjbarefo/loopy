#!/bin/sh
# demo.sh — watch a loop converge to green, no API keys required.
#
# Builds loopy, creates a throwaway repo containing a fizzbuzz with three
# bugs, registers a scripted shell "agent" that fixes whichever bug the
# verifier feedback names, and runs one loop. The agent never sees the test
# file — only the prompt loopy composes. Three iterations, three fixes, green.
#
# Usage: scripts/demo.sh   (from the loopy repo root)
set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
echo "==> building loopy"
(cd "$repo_root" && CGO_ENABLED=0 go build -o /tmp/loopy-demo-bin ./cmd/loopy)
LOOPY=/tmp/loopy-demo-bin

demo=$(mktemp -d /tmp/loopy-demo.XXXXXX)
echo "==> demo repo: $demo"
cd "$demo"
git init -q -b main
git config user.email demo@loopy.local
git config user.name "loopy demo"

# --- the "project": a fizzbuzz with three bugs -------------------------------
cat > fizzbuzz.sh <<'EOF'
#!/bin/sh
n="$1"
if [ $((n % 15)) -eq 0 ]; then
  echo "number"
elif [ $((n % 3)) -eq 0 ]; then
  echo "fiz"
elif [ $((n % 5)) -eq 0 ]; then
  echo "buz"
else
  echo "$n"
fi
EOF

# --- the verifier: the loop's definition of done -----------------------------
cat > test.sh <<'EOF'
#!/bin/sh
check() {
  want="$2"; got="$(sh ./fizzbuzz.sh "$1")"
  if [ "$got" != "$want" ]; then
    echo "fizzbuzz($1): want $want, got $got"
    exit 1
  fi
}
check 2 2
check 3 fizz
check 5 buzz
check 15 fizzbuzz
echo "all cases pass"
EOF

# --- the "agent": a script that reads loopy's prompt and fixes one bug -------
# It parses only the feedback section of the prompt — exactly what a real
# agent acts on — and patches the single failure named there.
cat > agent.sh <<'EOF'
#!/bin/sh
prompt="$1"
fb=$(sed -n '/## Feedback/,/## Changes/p' "$prompt")
case "$fb" in
  *"want fizzbuzz"*) sed -i.bak 's/echo "number"/echo "fizzbuzz"/' fizzbuzz.sh ;;
  *"want fizz"*)     sed -i.bak 's/echo "fiz"/echo "fizz"/'        fizzbuzz.sh ;;
  *"want buzz"*)     sed -i.bak 's/echo "buz"/echo "buzz"/'        fizzbuzz.sh ;;
  *) echo "agent: nothing recognizable in the feedback" ;;
esac
rm -f fizzbuzz.sh.bak
EOF

git add -A
git commit -q -m "fizzbuzz with three bugs"

echo "==> loopy init + agent registration"
"$LOOPY" init </dev/null >/dev/null
"$LOOPY" agent add fixer --cmd "sh $demo/agent.sh {prompt_file}" --default
git add -A
git commit -q -m "loopy setup"

echo
echo "==> the loop: one goal, one verifier, a budget — then hands off"
echo "    \$ loopy run \"make fizzbuzz pass its tests\" --verify \"sh ./test.sh\" --max-iters 6"
echo
"$LOOPY" run "make fizzbuzz pass its tests" --verify "sh ./test.sh" --max-iters 6

echo
echo "==> the loop is parked; loopy never merges. The evidence trail:"
echo
"$LOOPY" status make-fizzbuzz-pass-its-tests
echo
echo "==> the monitor's view of the same loop (loopy watch, one frame):"
echo
COLUMNS=100 "$LOOPY" watch make-fizzbuzz-pass-its-tests --once
echo
echo "==> what the agent was told in iteration 2 (excerpt):"
sed -n '/## Feedback/,/```$/p' .loopy/loops/make-fizzbuzz-pass-its-tests/iterations/0002/prompt.md | head -8
echo
echo "==> the reviewed output is a diff, not a merge:"
echo
cat .loopy/loops/make-fizzbuzz-pass-its-tests/iterations/0003/diff.patch
echo
echo "==> explore the full trail yourself:"
echo "    cd $demo"
echo "    $LOOPY log make-fizzbuzz-pass-its-tests"
echo "    find .loopy/loops -type f   # every prompt, log, diff, and verdict"
