// Parser-only regression checks: no browser, filesystem writes or dependencies.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { sharedPalette } from './shared_palette.mjs';
const brand = readFileSync(new URL('../../../docs/assets/brand/README.md', import.meta.url), 'utf8');
const expected = ['#090f1a', '#ebeff5', '#101621', '#4ddae9', '#00829d', '#989fab', '#f6f4ee', '#f0c27b', '#8a5a00'];
const shared = brand.split('## Colours\n')[1].split('\n## ')[0];
assert.deepEqual(sharedPalette('## Colours\n' + shared), expected);
assert.deepEqual(sharedPalette(brand.replaceAll('\n', '\r\n')), expected, 'CRLF headings remain supported');
assert.deepEqual(sharedPalette(brand), expected, 'separate TUI palette must not enter the site allow-list');
for (const [name, malformed] of [
  ['missing table', brand.replace(shared, '\n')],
  ['duplicate table', brand.replace(shared, shared + shared)],
  ['bad separator', brand.replace('|---|---|---|', '|--x|---|---|')],
  ['missing section', brand.replace('## Colours', '## Other')],
  ['duplicate section', brand + '\n## Colours\n' + shared],
  ['invalid hex', brand.replace('`#090f1a`', '`#gggggg`')],
  ['missing row', brand.replace(/^\| Night \|.*\n/m, '')],
  ['duplicate token', brand.replace('| Frost |', '| Night |')],
  ['duplicate color', brand.replace('`#ebeff5`', '`#090f1a`')],
  ['wrong header', brand.replace('| Token | Hex | Use |', '| Token | Value | Use |')],
  ['extra table', brand.replace('## TUI palette', '| Extra | `#123456` | invalid |\n\n## TUI palette')],
]) assert.throws(() => sharedPalette(malformed), /shared|brand|palette/, name);
console.log('--- PASS: shared palette excludes TUI colors and rejects ambiguous/malformed input');
