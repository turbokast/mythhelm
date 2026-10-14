#!/usr/bin/env bash
# Exercise browser checks with temporary static sites. Exit 1 on any missed defect.
# Requires npm ci and installed Google Chrome; writes only a temporary directory.
set -euo pipefail
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# Structural enumeration is deliberately Python standard library.
cat > "$work/enumerate.py" <<'PY'
import pathlib, re, sys
root = pathlib.Path(sys.argv[1])
errors = []
for path in sorted(root.rglob('*')):
    if not path.is_file() or path.relative_to(root).as_posix().startswith('ci/browser/'):
        continue
    rel = path.relative_to(root).as_posix()
    print(rel)
    if path.name in {'package.json', 'package-lock.json'} or path.suffix in {'.js', '.mjs', '.cjs'}:
        errors.append(rel)
    elif re.search(r'(?:^|[;|&(`])\s*(?:(?:if|elif|while|until|then|do|exec|env|command|!)\s+|[A-Za-z_][A-Za-z0-9_]*=[^\s;]+\s+)*(?:[^\s;]*/)?node(?:\s|$)', path.read_text(errors='replace'), re.M):
        errors.append(rel)
if errors:
    sys.exit('outside browser carve-out: ' + ', '.join(errors))
PY
python3 "$work/enumerate.py" "$root/scripts"
printf '%s\n' '--- PASS: structural enumeration'
mkdir -p "$work/outside/ci"
printf '{}\n' > "$work/outside/ci/package.json"
if python3 "$work/enumerate.py" "$work/outside" > "$work/structural.log" 2>&1; then
  echo 'FAIL: outside manifest accepted'; exit 1
fi
printf '%s\n' '--- PASS: outside manifest rejected'
rm "$work/outside/ci/package.json"
# shellcheck disable=SC2016 # Deliberately test literal command substitution without executing it.
for invocation in 'if node file; then true; fi' 'CI=true node file' 'result=$(node file)' '/usr/bin/node file'; do
  printf '%s\n' "$invocation" > "$work/outside/ci/check.sh"
  if python3 "$work/enumerate.py" "$work/outside" > "$work/structural.log" 2>&1; then
    echo "FAIL: outside invocation accepted: $invocation"; exit 1
  fi
done
printf '%s\n' '--- PASS: outside command words rejected' 
python3 - "$work" <<'PY'
import pathlib,sys
root=pathlib.Path(sys.argv[1])
base='''<!doctype html><html><head><style>:root{--color-paper:#f6f4ee;--color-ink:#101621;--color-star-light:#00829d}body{margin:0;background:#f6f4ee;color:#101621}button{width:44px;height:44px;background:#f6f4ee;color:#101621;border:1px solid #101621}EXTRA</style></head><body><p>Readable text</p>BODY</body></html>'''
cases={'good':('','<button>Go</button>'),'wide':('pre{width:2000px}','<pre>wide</pre>'),'clip':('html,body{overflow-x:clip}pre{width:2000px}','<pre>wide</pre>'),'contrast':('p{color:#00829d}',''),'undeclared':('p{color:#123456}',''),'target':('button{width:20px;height:20px}','<button>Go</button>'),'origin':('','<script src="https://example.invalid/a.js"></script>'),'gif':('','<img src="demo.gif" alt="scripted demo">'),'motion':('@keyframes spin{to{transform:rotate(10deg)}}p{animation:spin 2s infinite}','')}
cases.update({
 'opacity':('p{opacity:.1}', ''),
 'value':('input{width:100px;height:44px;color:#00829d;background:#f6f4ee;border:1px solid #101621}', '<input value=bad>'),
 'placeholder':('input{width:100px;height:44px;color:#101621;background:#f6f4ee;border:1px solid #101621}input::placeholder{color:#00829d;opacity:1}', '<input placeholder=bad>'),
 'quickstart-heading':('#quickstart pre{margin-top:2000px}', '<h1 id=landing-title>Landing</h1><section id=quickstart><h2>Quickstart</h2><pre>demo</pre></section>'),
 'quickstart':('#quickstart{margin-top:2000px}', '<h1 id="landing-title">Landing</h1><pre id="quickstart">demo</pre>'),
 'weight':('', '<h1 id="landing-title">Landing</h1><pre id="quickstart">demo</pre><!--' + 'x'*500001 + '-->'),
 'preload':('', '<video preload="auto"></video>'),
 'poster':('', '<figure class="recording"><img src="still.png" alt="demo"></figure>'),
})
for name,(css,body) in cases.items():
 d=root/name;d.mkdir();(d/'page.html').write_text(base.replace('EXTRA',css).replace('BODY',body))
PY
check="$root/scripts/ci/browser/check_layout.mjs"
node "$check" "$work/good"
printf '%s\n' '--- PASS: conforming fixture'
for case in wide clip contrast undeclared target origin gif motion quickstart quickstart-heading weight preload poster opacity value placeholder; do
  if node "$check" "$work/$case" > "$work/$case.log" 2>&1; then
    echo "FAIL: $case accepted"; exit 1
  fi
  expected="$case"
  case "$case" in opacity|value|placeholder) expected=contrast ;; quickstart-heading) expected=quickstart ;; esac
  grep -q "${expected}:" "$work/$case.log" || { cat "$work/$case.log"; exit 1; }
  printf '%s\n' "--- PASS: $case rejected"
done
if CHROME_BIN="$work/absent-chrome" node "$check" "$work/good" > "$work/missing.log" 2>&1; then
  echo 'FAIL: missing Chrome accepted'; exit 1
fi
grep -q 'browser-source:' "$work/missing.log"
printf '%s\n' '--- PASS: missing Chrome rejected'
