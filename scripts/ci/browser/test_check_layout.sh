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
    elif re.search(r'(?:^|[;|&(`])\s*(?:(?:if|elif|while|until|then|do|exec|env|command|!)\s+|(?:sudo|timeout|nice|nohup)\s+(?:[^;|&\n]*?\s+)?|[A-Za-z_][A-Za-z0-9_]*=[^\s;]+\s+)*(?:[^\s;]*/)?node(?:\s|$)', path.read_text(errors='replace'), re.M):
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
for invocation in 'if node file; then true; fi' 'CI=true node file' 'result=$(node file)' '/usr/bin/node file' 'sudo node file' 'sudo -u user node file' 'timeout 5s node file' 'nohup node file'; do
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
 'hidden-command':('#quickstart{display:none}', '<pre id=quickstart>demo</pre>'),
 'no-title-marker':('#quickstart{margin-top:2000px}', '<pre id=quickstart>demo</pre>'),
 'websocket':('', '<script>new WebSocket("wss://example.invalid/socket")</script>'),
 'resource':('', '<img src=missing.png alt=missing>'),
 'controls':('', '<button id=search-open>Search</button>'),
 'hero':('video{width:100px;height:100px}', '<figure class=hero-recording><video preload=none></video><figcaption>Recording without scripted description</figcaption></figure>'),
 'caption':('figure{width:80px}img{width:100px;height:100px}', '<figure class=recording><figcaption>Above image</figcaption><img src=poster.png alt="scripted demo"></figure>'),
 'quickstart-heading':('#quickstart pre{margin-top:2000px}', '<h1 id=landing-title>Landing</h1><section id=quickstart><h2>Quickstart</h2><pre>demo</pre></section>'),
 'quickstart':('#quickstart{margin-top:2000px}', '<h1 id="landing-title">Landing</h1><pre id="quickstart">demo</pre>'),
 'weight':('', '<h1 id="landing-title">Landing</h1><pre id="quickstart">demo</pre><!--' + 'x'*500001 + '-->'),
 'preload':('', '<video preload="auto"></video>'),
 'poster':('', '<figure class="recording"><img src="still.png" alt="demo"></figure>'),
})
healthy_controls = """<button id="search-open" onclick="document.querySelector('dialog').showModal();fetch('search.json').then(r=>r.json()).then(()=>document.getElementById('search-status').textContent='Ready')">Search</button><dialog id="site-search" class="search-dialog"><input id="search-query" oninput="document.getElementById('search-status').textContent='1 result';document.getElementById('search-results').innerHTML='<a href=#quickstart>MYTHHELM</a>'"><p id="search-status"></p><div id="search-results"></div></dialog>"""
for name,(css,body) in cases.items():
 d=root/name;d.mkdir()
 if name not in {'quickstart','quickstart-heading','weight','hidden-command','no-title-marker'}:
  body='<pre id="quickstart">demo</pre>'+body
 if name != 'controls': body+=healthy_controls
 css='a{color:#101621}dialog,input{background:#f6f4ee;color:#101621;border:1px solid #101621}input{width:120px;height:44px}.search-dialog a{display:block;min-width:44px;min-height:44px}'+css
 (d/'index.html').write_text(base.replace('EXTRA',css).replace('BODY',body))
 (d/'search.json').write_text('{}')

PY
check="$root/scripts/ci/browser/check_layout.mjs"
node "$check" "$work/good"
printf '%s\n' '--- PASS: conforming fixture'
for case in wide clip contrast undeclared target origin gif motion quickstart quickstart-heading weight preload poster opacity value placeholder hidden-command no-title-marker websocket resource controls hero caption; do
  if node "$check" "$work/$case" > "$work/$case.log" 2>&1; then
    echo "FAIL: $case accepted"; exit 1
  fi
  expected="$case"
  case "$case" in opacity|value|placeholder) expected=contrast ;; quickstart-heading|hidden-command|no-title-marker) expected=quickstart ;; websocket) expected=origin ;; esac
  grep -q "${expected}:" "$work/$case.log" || { cat "$work/$case.log"; exit 1; }
  printf '%s\n' "--- PASS: $case rejected"
done
if CHROME_BIN="$work/absent-chrome" node "$check" "$work/good" > "$work/missing.log" 2>&1; then
  echo 'FAIL: missing Chrome accepted'; exit 1
fi
grep -q 'browser-source: preinstalled Chrome unavailable' "$work/missing.log"
printf '%s\n' '--- PASS: missing Chrome rejected'

set +e
node "$check" "$work/good/index.html" > "$work/invalid.log" 2>&1
invalid_exit=$?
set -e
if [[ "$invalid_exit" != 2 ]]; then cat "$work/invalid.log"; exit 1; fi
printf '%s\n' '--- PASS: invalid site uses exit 2'

mkdir "$work/no-landing"
printf '<p>Missing landing</p>' > "$work/no-landing/page.html"
if node "$check" "$work/no-landing" > "$work/no-landing.log" 2>&1; then
  echo 'FAIL: absent landing accepted'; exit 1
fi
grep -q 'landing: required landing page is absent' "$work/no-landing.log"
printf '%s\n' '--- PASS: missing landing rejected'
