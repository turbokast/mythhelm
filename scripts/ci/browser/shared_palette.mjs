// Parse only the shared ## Colours table. Malformed or ambiguous input throws;
// other sections (including the TUI palette) cannot expand the site allow-list.
export function sharedPalette(brand) {
  const lines = brand.replaceAll('\r\n', '\n').split('\n');
  const starts = lines.flatMap((line, index) => /^## Colours[ \t]*$/.test(line) ? [index] : []);
  const fail = () => { throw new Error('cannot parse the nine declared shared brand colors'); };
  if (starts.length !== 1) fail();
  const start = starts[0] + 1;
  const next = lines.findIndex((line, index) => index >= start && /^#{1,2}\s/.test(line));
  const rows = lines.slice(start, next < 0 ? lines.length : next).filter(line => line.trim().startsWith('|'));
  if (rows.length !== 11 || !/^\|\s*Token\s*\|\s*Hex\s*\|\s*Use\s*\|$/.test(rows[0])
      || !/^\|\s*:?-+:?\s*\|\s*:?-+:?\s*\|\s*:?-+:?\s*\|$/.test(rows[1])) fail();
  const tokens = new Set(['Night', 'Frost', 'Ink', 'Star (dark)', 'Star (light)', 'Mist', 'Paper', 'Amber (dark)', 'Amber (light)']);
  const palette = [];
  for (const row of rows.slice(2)) {
    const match = row.match(/^\|\s*([^|]+?)\s*\|\s*`(#[0-9a-fA-F]{6})`\s*\|\s*[^|]+\s*\|$/);
    if (!match || !tokens.delete(match[1].trim())) fail();
    const color = match[2].toLowerCase();
    if (palette.includes(color)) fail();
    palette.push(color);
  }
  if (tokens.size) fail();
  return palette;
}
