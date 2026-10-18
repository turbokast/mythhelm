#!/usr/bin/env python3
"""Validate built documentation and checked-in sources without running examples.

Usage: --site DIR --docs DIR. Read-only, no network. Requires ffprobe for actual
video duration. Exit 0 conforming, 1 findings, 2 unavailable/unparseable input.
YAML input deliberately supports flat lists of scalar mappings (or JSON), the
checked-in data contract; unsupported syntax fails closed rather than guessing.
"""
import argparse
from dataclasses import dataclass, field
import html
from html.parser import HTMLParser
import json
import math
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote, urlsplit


class InputError(ValueError):
    """Evidence cannot be read or parsed."""


def read(path):
    return path.read_text(encoding='utf-8')


def scalar(value):
    value = value.strip()
    if value.startswith('"'):
        return json.loads(value)
    if value.startswith("'") and value.endswith("'"):
        return value[1:-1].replace("''", "'")
    if value == 'null':
        return None
    if value in ('true', 'false'):
        return value == 'true'
    if re.fullmatch(r'-?\d+', value):
        return int(value)
    if not value or value[0] in '[{&*!>|' or ' #' in value:
        raise InputError('unsupported scalar syntax')
    return value


def load_rows(path):
    """Read the flat list-of-mappings docs data schema, preserving commands."""
    return parse_rows(read(path), path)


def parse_rows(text, path):
    """Parse the shared restricted flat mapping format from a bounded source."""
    if text.lstrip().startswith('['):
        rows = json.loads(text)
    else:
        rows = []
        for line in text.splitlines():
            if not line.strip() or line.lstrip().startswith('#'):
                continue
            match = re.fullmatch(r'(- |  )([a-z_]+):\s*(.*)', line)
            if not match:
                raise InputError(f'{path}: unsupported data row')
            prefix, key, value = match.groups()
            if prefix == '- ':
                rows.append({})
            if not rows or key in rows[-1]:
                raise InputError(f'{path}: duplicate key or missing list item')
            rows[-1][key] = scalar(value)
    if not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows):
        raise InputError(f'{path}: expected list of mappings')
    return rows


def normalise_output(text):
    """docs-output-v1: CRLF to LF and trailing whitespace per line only."""
    return '\n'.join(line.rstrip() for line in text.replace('\r\n', '\n').split('\n'))


def words(text):
    return ' '.join(html.unescape(text).split())


def prose(text):
    # Status quotes only use inline code/emphasis; keep punctuation and words.
    return words(re.sub(r'[`*]', '', text))


@dataclass
class Node:
    tag: str
    attrs: dict = field(default_factory=dict)
    parent: object = None
    line: int = 1
    children: list = field(default_factory=list)

    def all(self, tag=None, cls=None):
        result = []
        for child in self.children:
            if isinstance(child, Node):
                if (tag is None or child.tag == tag) and (cls is None or cls in child.classes):
                    result.append(child)
                result.extend(child.all(tag, cls))
        return result

    @property
    def classes(self):
        return self.attrs.get('class', '').split()

    @property
    def text(self):
        return ''.join(c.text if isinstance(c, Node) else c for c in self.children)

    def ancestor(self, attr):
        node = self.parent
        while node:
            if attr in node.attrs:
                return node.attrs[attr]
            node = node.parent
        return None


class Document(HTMLParser):
    def __init__(self, text):
        super().__init__(convert_charrefs=True)
        self.root = Node('document')
        self.current = self.root
        self.feed(text)
        self.close()

    def handle_starttag(self, tag, attrs):
        node = Node(tag, dict(attrs), self.current, self.getpos()[0])
        self.current.children.append(node)
        if tag not in {'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'meta', 'param', 'source', 'track', 'wbr'}:
            self.current = node

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        self.handle_endtag(tag)

    def handle_endtag(self, tag):
        node = self.current
        while node.parent:
            if node.tag == tag:
                self.current = node.parent
                return
            node = node.parent

    def handle_data(self, data):
        self.current.children.append(data)


@dataclass
class Fence:
    anchor: str
    text: str
    annotation: str | None
    line: int


def slugify(text):
    text = re.sub(r'<[^>]+>', '', text).lower()
    text = re.sub(r'[^\w\- ]', '', text)
    return text.replace(' ', '-')


def example_blocks(docs):
    """Resolve repo-relative h2/fence identities without counting raw/indented code.

    Returns fences plus author wrapper errors. No command is executed. Task 10
    consumes the same identities and docs-output-v1 for its executor.
    """
    blocks, errors = {}, []
    for path in sorted(docs.rglob('*.md')):
        text = read(path)
        heading, ordinal, duplicates = '', 0, {}
        fence, content, annotation, start, indent = None, [], None, 0, 0
        stack, wrapper_counts = [], {}
        raw_end = None
        for number, line in enumerate(text.splitlines(), 1):
            if fence:
                if re.fullmatch(r' {0,3}' + re.escape(fence[0]) + '{' + str(len(fence)) + r',}\s*', line):
                    anchor = f'docs/{path.relative_to(docs)}#{heading}[{ordinal}]'
                    if anchor in blocks:
                        errors.append((path, number, 'ambiguous fenced block anchor'))
                    blocks[anchor] = Fence(anchor, '\n'.join(content), annotation, start)
                    if annotation and annotation != anchor:
                        errors.append((path, start, 'annotation does not match contained source fence identity'))
                    fence = None
                else:
                    content.append(re.sub(r'^ {0,%d}' % indent, '', line))
                continue
            # Ignore raw regions only outside fences; commands and expected
            # output may themselves contain literal HTML or comments.
            if raw_end:
                end = re.search(raw_end, line, re.I)
                if not end:
                    continue
                line, raw_end = line[end.end():], None
            while True:
                start_raw = re.search(r'<!--|^ {0,3}<pre\b[^>]*>', line, re.I)
                if not start_raw:
                    break
                end_pattern = '-->' if start_raw[0] == '<!--' else r'</pre\s*>'
                end = re.search(end_pattern, line[start_raw.end():], re.I)
                if end:
                    line = line[:start_raw.start()] + line[start_raw.end() + end.end():]
                else:
                    line, raw_end = line[:start_raw.start()], end_pattern
                    break
            match = re.match(r'^ {0,3}##\s+(.+?)\s*#*\s*$', line)
            if match:
                base = slugify(match[1])
                duplicate = duplicates.get(base, 0)
                heading = base + (f'-{duplicate}' if duplicate else '')
                duplicates[base] = duplicate + 1
                ordinal = 0
            for tag in re.finditer(r'<div\b[^>]*>|</div\s*>', line, re.I):
                if tag[0].lower().startswith('</'):
                    if stack:
                        stack.pop()
                else:
                    parsed = Document(tag[0]).root.all('div')[0]
                    value = parsed.attrs.get('data-example-anchor') if 'example-block' in parsed.classes else None
                    stack.append((value, number))
                    if value is not None:
                        wrapper_counts[number] = 0
            match = re.match(r'^( {0,3})(`{3,}|~{3,})([^`]*)$', line)
            if match:
                indent, fence, content, start = len(match[1]), match[2], [], number
                ordinal += 1
                owner = next(((value, row) for value, row in reversed(stack) if value is not None), None)
                annotation = owner[0] if owner else None
                if owner:
                    wrapper_counts[owner[1]] += 1
        if fence:
            errors.append((path, start, 'unclosed Markdown fence'))
        for number, count in wrapper_counts.items():
            if count != 1:
                errors.append((path, number, 'example wrapper must contain exactly one fence'))
    return blocks, errors


class Checker:
    def __init__(self, site, docs):
        self.site, self.docs = Path(os.path.normpath(site)), docs
        self.findings = []
        paths = sorted(self.site.rglob('*.html'))
        if not paths:
            raise InputError('site contains no HTML pages')
        self.pages = {p: Document(read(p)).root for p in paths}

    def fail(self, path, category, detail, line=1):
        self.findings.append(f'{path}:{line}: {category}: {detail}')

    def layout(self):
        for path in self.pages:
            if '<!-- mythhelm-layout:v1 -->' not in read(path):
                self.fail(path, 'layout', 'missing layout marker')

    def styles(self):
        brand = read(self.docs / 'assets/brand/README.md')
        tokens = {}
        for name, value in re.findall(r'^\| ([^|]+?) \| `?(#[0-9a-fA-F]{6})`? \|', brand, re.M):
            key = '--color-' + name.lower().replace(' ', '-').replace('(', '').replace(')', '')
            tokens[key] = value.lower()
        if not tokens:
            raise InputError('brand colour table is empty')
        declared = {}
        linked = {self.site / 'assets/site.css'}
        for page, root in self.pages.items():
            for node in root.all('link'):
                if 'stylesheet' in node.attrs.get('rel', '').split() and self.same_origin(node.attrs.get('href', '')):
                    linked.add(self.local_path(node.attrs.get('href', ''), page))
        inline = [(p, n.text) for p, root in self.pages.items() for n in root.all('style')]
        pending = [(p, read(p)) for p in linked] + inline
        while pending:
            sheet, sheet_text = pending.pop()
            for match in re.finditer(r'@import\s+(?:url\(\s*)?[\"\']?([^\"\'\s;)]+)', re.sub(r'/\*.*?\*/', '', sheet_text, flags=re.S), re.I):
                if self.same_origin(match[1]):
                    target = self.local_path(match[1], sheet)
                    if target not in linked:
                        linked.add(target)
                        pending.append((target, read(target)))
        styles = [(p, read(p)) for p in sorted(linked)]
        styles += inline
        styles += [(p, '* {' + n.attrs['style'] + '}') for p, root in self.pages.items() for n in root.all() if 'style' in n.attrs]
        for path, text in styles:
            text = re.sub(r'/\*.*?\*/', '', text, flags=re.S)
            for key, value in re.findall(r'([\w-]+)\s*:\s*([^;{}]+)', text):
                value = value.strip()
                if key.startswith('--color-'):
                    if key not in tokens or tokens[key] != value.lower():
                        self.fail(path, 'tokens', f'brand table differs from {key}')
                    if key in declared:
                        self.fail(path, 'tokens', f'duplicate token {key}')
                    declared[key] = value.lower()
                    continue
                if re.search(r'var\(\s*--color-amber-', value):
                    self.fail(path, 'amber', 'amber reference outside token declaration')
                stripped = value
                while re.search(r'var\([^()]*\)', stripped):
                    updated = re.sub(r'var\(\s*--[\w-]+\s*(?:,\s*([^()]*))?\)', lambda m: m[1] or '', stripped)
                    if updated == stripped:
                        self.fail(path, 'tokens', 'unparseable CSS variable')
                        break
                    stripped = updated
                stripped = re.sub(r'url\([^)]*\)|["\'][^"\']*["\']', '', stripped)
                if re.search(r'#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|color-mix)\(', stripped):
                    self.fail(path, 'tokens', f'colour literal or derived colour in {key}')
                # Named colours are invalid in properties accepting colours. Remove
                # lengths, border styles and allowed non-colour keywords first.
                if key in {'color', 'background', 'background-color', 'border', 'border-color', 'outline', 'outline-color', 'fill', 'stroke', 'box-shadow', 'text-shadow', 'text-decoration', 'text-emphasis', 'column-rule', 'text-decoration-color', 'caret-color'} or 'gradient(' in stripped or key in {'background-image', 'border-image'} or key.endswith('-color') or key in {'border-top', 'border-right', 'border-bottom', 'border-left', 'border-block', 'border-inline', 'border-block-start', 'border-block-end', 'border-inline-start', 'border-inline-end'} or (key.startswith('--') and not key.startswith('--font-')):
                    clean = re.sub(r'[-+]?\d*\.?\d+(?:[a-z%]+)?', '', stripped)
                    clean = re.sub(r'(?:repeating-)?(?:linear|radial|conic)-gradient\(', '', clean)
                    names = re.findall(r'[a-zA-Z]+(?:-[a-zA-Z]+)*', clean)
                    allowed = {'transparent', 'currentcolor', 'none', 'inherit', 'initial', 'unset', 'solid', 'dashed', 'dotted', 'double', 'inset', 'outset', 'auto', 'to', 'left', 'right', 'top', 'bottom', 'circle', 'ellipse', 'at', 'center', 'underline', 'overline', 'line-through', 'wavy', 'blink', 'spelling-error', 'grammar-error', 'from-font', 'filled', 'open', 'dot', 'double-circle', 'triangle', 'sesame', 'hidden', 'groove', 'ridge'}
                    if any(n.lower() not in allowed for n in names):
                        self.fail(path, 'tokens', f'non-token colour in {key}')
            # The backdrop contains no text. Other opacity can tint text.
            for selector, body in re.findall(r'([^{}]+)\{([^{}]*)\}', text):
                exclusively_backdrop = all(re.search(r'::backdrop\s*$', part) for part in selector.split(','))
                if not exclusively_backdrop and re.search(r'\bopacity\s*:\s*(?!1(?:\.0+)?\s*[;}])', body + '}'):
                    self.fail(path, 'tokens', 'text opacity derives a colour')
        if declared != tokens:
            self.fail(self.site / 'assets/site.css', 'tokens', 'declared tokens do not equal brand table')

    def resources(self):
        config = read(self.docs / '_config.yml')
        url = re.search(r'^url:\s*(.+)$', config, re.M)
        base = re.search(r'^baseurl:\s*(.*)$', config, re.M)
        self.origin = str(scalar(url[1])) if url else 'https://example.com'
        self.base = str(scalar(base[1])) if base and base[1].strip() else ''
        allowed = set()
        for row in load_rows(self.docs / '_data/site-scripts.yml'):
            if set(row) != {'src', 'purpose'} or row['purpose'] not in {'search', 'sidebar', 'player'} or not isinstance(row['src'], str):
                raise InputError('invalid script allow-list entry')
            allowed.add(row['src'])
        actual = {str(p.relative_to(self.site)) for p in self.site.rglob('*') if p.suffix.lower() in {'.js', '.mjs', '.cjs'}}
        for name in actual - allowed:
            self.fail(self.site / name, 'scripts', 'script absent from allow-list')
        for name in allowed - actual:
            self.fail(self.site / name, 'scripts', 'listed script missing')
        for path, root in self.pages.items():
            for node in root.all():
                attrs = node.attrs
                for attr in ('src', 'poster', 'data'):
                    if attr in attrs:
                        self.resource(path, attrs[attr], node.line)
                if 'srcset' in attrs:
                    for entry in attrs['srcset'].split(','):
                        if entry.strip():
                            self.resource(path, entry.strip().split()[0], node.line)
                if node.tag == 'link' and set(attrs.get('rel', '').split()) & {'stylesheet', 'preload', 'icon', 'modulepreload', 'prefetch', 'preconnect', 'dns-prefetch'}:
                    self.resource(path, attrs.get('href', ''), node.line)
                if node.tag == 'base':
                    self.fail(path, 'origins', 'base element can redirect relative resources', node.line)
                if node.tag == 'script':
                    src = attrs.get('src')
                    if not src:
                        self.fail(path, 'scripts', 'inline script absent from file allow-list', node.line)
                    elif self.local_path(src, path).relative_to(self.site).as_posix() not in allowed:
                        self.fail(path, 'scripts', 'script reference absent from allow-list', node.line)
                if node.tag == 'style':
                    self.css_resources(path, node.text)
                if 'style' in attrs:
                    self.css_resources(path, attrs['style'])
        for path in self.site.rglob('*.css'):
            self.css_resources(path, read(path))

    def local_path(self, url, page=None):
        name = unquote(urlsplit(url).path)
        if name.startswith('/'):
            if self.base and name.startswith(self.base + '/'):
                name = name[len(self.base):]
            result = self.site / name.lstrip('/')
        else:
            result = (page.parent if page else self.site) / name
        result = Path(os.path.normpath(result))
        if not result.resolve().is_relative_to(self.site.resolve()):
            raise InputError('resource escapes site directory')
        if result.is_dir() or name.endswith('/'):
            result /= 'index.html'
        return result

    def same_origin(self, ref):
        parsed, origin = urlsplit(ref), urlsplit(self.origin)
        return not ((parsed.scheme and parsed.scheme not in {'http', 'https'}) or (parsed.netloc and (parsed.netloc != origin.netloc or (parsed.scheme and parsed.scheme != origin.scheme))))

    def resource(self, path, ref, line=1):
        if not self.same_origin(ref):
            self.fail(path, 'origins', f'off-origin resource: {ref}', line)

    def css_resources(self, path, text):
        text = re.sub(r'/\*.*?\*/', '', text, flags=re.S)
        for match in re.finditer(r'url\(\s*["\']?([^\s)"\']+)|@import\s+["\']([^"\']+)', text, re.I):
            self.resource(path, match[1] or match[2], text.count('\n', 0, match.start()) + 1)

    def landing(self):
        path = self.site / 'index.html'
        root = self.pages.get(path)
        if root is None:
            raise InputError('missing landing page')
        headings = [n for n in root.all('h1') if n.attrs.get('id') == 'landing-title']
        links = []
        if len(headings) == 1:
            heading = headings[0]
            siblings = heading.parent.children
            after = siblings[siblings.index(heading) + 1:]
            for sibling in after:
                if isinstance(sibling, Node):
                    if sibling.tag != 'p':
                        break
                    links.extend(n for n in sibling.all('a') if n.attrs.get('href') == '#status')
        if len(links) != 1:
            self.fail(path, 'panel-link', 'expected exactly one panel link immediately under headline')
        source = read(self.docs / 'limitations.md')
        sections = {}
        heading = None
        for line in source.splitlines():
            if line.startswith('## '):
                heading = line[3:].strip()
                sections[heading] = ''
            elif heading:
                sections[heading] += line + '\n'
        rows = load_rows(self.docs / '_data/status.yml')
        by_id = {}
        panels = [n for n in root.all() if n.attrs.get('id') == 'status']
        if len(panels) != 1 or not rows:
            self.fail(path, 'panel', 'missing status panel or status data')
            return
        panel = panels[0]
        for row in rows:
            if set(row) != {'id', 'section', 'quote', 'column', 'tracker'} or any(not isinstance(row[k], str) for k in ('id', 'section', 'quote', 'column')):
                raise InputError('invalid status data schema')
            ident, section, quote, column, tracker = (row[k] for k in ('id', 'section', 'quote', 'column', 'tracker'))
            if ident in by_id:
                self.fail(path, 'panel', f'duplicate status id {ident}')
            by_id[ident] = row
            if not quote.strip() or words(quote) not in words(sections.get(section, '')):
                self.fail(self.docs / 'limitations.md', 'panel', f'{ident}: quote missing from named section')
            if column not in {'works', 'in-progress', 'not-yet'} or (column == 'works' and section != 'Supported today' and not section.endswith(' limits')) or (column != 'works' and section != 'Not supported yet'):
                self.fail(path, 'panel', f'{ident}: invalid source section for column')
            if column == 'in-progress':
                if not isinstance(tracker, str) or not tracker.strip() or tracker not in quote:
                    self.fail(path, 'panel', f'{ident}: tracker must occur within quote')
            elif tracker is not None:
                self.fail(path, 'panel', f'{ident}: tracker only allowed in progress')
            matches = [n for n in panel.all() if n.attrs.get('data-status-id') == ident]
            if len(matches) != 1:
                self.fail(path, 'panel', f'{ident}: expected one rendered status item')
            for node in matches:
                quotes = node.all(cls='status-quote')
                if node.attrs.get('data-source-section') != section or node.ancestor('data-status-column') != column or not quotes or words(quotes[0].text) != prose(quote):
                    self.fail(path, 'panel', f'{ident}: rendered source/column/quote differs', node.line)
                if section.endswith(' limits') and not node.ancestor('data-status-id'):
                    self.fail(path, 'panel', f'{ident}: limit must be nested caveat', node.line)
        for node in panel.all():
            if 'data-status-id' in node.attrs and node.attrs['data-status-id'] not in by_id:
                self.fail(path, 'panel', 'rendered item absent from status data', node.line)
        # The landing front matter already declares the capability/caveat binding.
        # Reuse the flat data parser after removing the list's two-space indent.
        front = re.match(r'\A---\n(.*?)\n---(?:\n|$)', read(self.docs / 'index.md'), re.S)
        blocks = re.findall(r'^cells:\s*\n((?:[ \t].*(?:\n|$)|\n)*)', front[1] if front else '', re.M)
        if len(blocks) != 1:
            raise InputError('expected one landing cells list')
        declared = parse_rows('\n'.join(line[2:] if line.startswith('  ') else line for line in blocks[0].splitlines()), self.docs / 'index.md')
        if len(declared) != 4 or any(not {'title', 'need', 'status', 'evidence'} <= set(item) or set(item) - {'title', 'need', 'status', 'evidence', 'limit'} or any(not isinstance(value, str) or not value.strip() for value in item.values()) for item in declared):
            raise InputError('invalid landing cell schema')
        if len({item['title'] for item in declared}) != len(declared):
            raise InputError('duplicate landing cell mapping')
        cells = root.all(cls='capability')
        if len(cells) != 4:
            self.fail(path, 'panel', 'expected four capability cells')
        for cell, declaration in zip(cells, declared):
            row = by_id.get(cell.attrs.get('data-status-id'))
            labels = cell.all(cls='capability-label')
            evidence = cell.all(cls='capability-evidence')
            if declaration['status'] != cell.attrs.get('data-status-id'):
                self.fail(path, 'panel', 'cell capability differs from landing source', cell.line)
            if not row or cell.attrs.get('data-status') != row['column'] or len(labels) != 1 or words(labels[0].text) != row['column'].replace('-', ' ') or len(evidence) != 1 or words(evidence[0].text) != prose(row['quote']):
                self.fail(path, 'panel', 'cell label/evidence differs from status source', cell.line)
            caveats = cell.all(cls='capability-limit')
            expected = by_id.get(declaration.get('limit'))
            if row and row['column'] == 'works' and expected is None:
                self.fail(path, 'panel', 'working cell must declare a status-data limit', cell.line)
            if expected is not None:
                if len(caveats) != 1 or caveats[0].attrs.get('data-status-id') != expected['id'] or words(caveats[0].text) != prose(expected['quote']):
                    self.fail(path, 'panel', 'cell limit differs from status source', cell.line)
            elif caveats or 'limit' in declaration:
                self.fail(path, 'panel', 'cell limit absent from status source', cell.line)
            # Current cell evidence is a limitations heading slug, as rendered by
            # index.md. Verify both the declared destination and its built anchor.
            anchor = declaration['evidence']
            target = self.site / 'limitations.html'
            target_root = self.pages.get(target)
            source_sections = {slugify(section) for section in sections}
            valid_links = []
            for link in cell.all('a'):
                href = link.attrs.get('href', '')
                parsed = urlsplit(href)
                if self.same_origin(href) and not parsed.query and self.local_path(href, path) == target and unquote(parsed.fragment) == anchor:
                    valid_links.append(link)
            if len(valid_links) != 1 or anchor not in source_sections or target_root is None or not any(node.attrs.get('id') == anchor for node in target_root.all()):
                self.fail(path, 'panel', 'cell evidence must link its declared built limitations section', cell.line)

    def footer(self):
        required = {'limitations.html', 'contributing.html', 'mirror/LICENSE.html', 'mirror/SECURITY.html', 'mirror/CHANGELOG.html', 'mirror/GOVERNANCE.html'}
        for path, root in self.pages.items():
            footers = root.all('footer', 'site-footer')
            targets = []
            for footer in footers:
                for link in footer.all('a'):
                    if not self.same_origin(link.attrs.get('href', '')):
                        self.fail(path, 'footer', 'footer target must be on site', link.line)
                    target = self.local_path(link.attrs.get('href', ''), path)
                    targets.append(target.relative_to(self.site).as_posix())
                    if not target.is_file():
                        self.fail(path, 'footer', 'footer target missing: ' + str(target), link.line)
            if len(footers) != 1 or any(targets.count(name) != 1 for name in required):
                self.fail(path, 'footer', 'footer must link all six G10 pages exactly once')

    def chapters(self):
        path = self.site / 'demos/manifest.json'
        manifest = json.loads(read(path))
        if not isinstance(manifest, dict) or not isinstance(manifest.get('recordings'), list) or not all(isinstance(row, dict) for row in manifest['recordings']):
            raise InputError('invalid demo manifest schema')
        heroes = [row for row in manifest['recordings'] if row.get('name') == 'hero']
        if len(heroes) != 1:
            raise InputError('manifest must contain exactly one hero')
        chapters = heroes[0].get('chapters', [])
        if not isinstance(chapters, list) or not all(isinstance(chapter, dict) for chapter in chapters):
            raise InputError('invalid chapter schema')
        if [c.get('name') for c in chapters] != ['title', 'run', 'kill', 'recover', 'review']:
            self.fail(path, 'chapters', 'missing or reordered chapter names')
            return
        offsets = [c.get('at_seconds') for c in chapters]
        if any(type(t) not in (float, int) or not math.isfinite(t) for t in offsets):
            raise InputError('chapter offsets must be finite numbers')
        durations = []
        for extension in ('webm', 'mp4'):
            media = self.site / ('demos/hero.' + extension)
            result = subprocess.run(['ffprobe', '-v', 'error', '-show_entries', 'format=duration', '-of', 'json', str(media)], capture_output=True, text=True, timeout=30, check=True)
            duration = float(json.loads(result.stdout)['format']['duration'])
            if not math.isfinite(duration) or duration <= 0:
                raise InputError('video duration unavailable')
            durations.append(duration)
        if offsets[0] < 0 or any(b <= a for a, b in zip(offsets, offsets[1:])) or offsets[-1] >= min(durations):
            self.fail(path, 'chapters', 'offsets must increase and fit actual video duration')

    def states(self):
        valid = {'reported', 'estimated', 'unknown'}
        seen = set()
        for path, root in self.pages.items():
            for node in root.all(cls='state'):
                value = words(node.text)
                seen.add(value)
                if value not in valid or 'state-' + value not in node.classes or node.all():
                    self.fail(path, 'states', 'state span must wrap exactly its state word', node.line)
        if not valid <= seen:
            self.fail(self.site, 'states', 'all three state labels must be present')
        source_seen = set()
        for path in self.docs.rglob('*.md'):
            for match in re.finditer(r'{%\s*include\s+state\.html\b(.*?)%}', read(path), re.S):
                arg = re.fullmatch(r'\s+state="(reported|estimated|unknown)"\s*', match[1])
                if not arg:
                    self.fail(path, 'states', 'invalid author state argument', read(path).count('\n', 0, match.start()) + 1)
                else:
                    source_seen.add(arg[1])
        if not valid <= source_seen:
            self.fail(self.docs, 'states', 'source must contain all three state arguments')

    def search(self):
        path = self.site / 'search.json'
        text = read(path)
        if len(text.encode('utf-8')) > 300000:
            self.fail(path, 'search', 'index exceeds 300000 bytes')
        if any(token in text for token in ('{{', '{%', 'mythhelm-layout:v1', '<html', '<body', '<!DOCTYPE')):
            self.fail(path, 'search', 'unresolved Liquid or layout markup in JSON')
        data = json.loads(text)
        if not isinstance(data, dict) or not data:
            raise InputError('search index must be a nonempty object')
        groups = {}
        nav = load_rows(self.docs / '_data/navigation.yml')
        for row in nav:
            if row.get('group') not in {'Start', 'Use', 'Understand', 'Reference'} or not isinstance(row.get('path'), str):
                raise InputError('invalid navigation group/path')
            name = row['path'].split('#')[0]
            candidates = [self.site / (name[:-3] + '.html')]
            if name.endswith('README.md'):
                candidates.insert(0, self.site / name[:-9] / 'index.html')
            target = next((p for p in candidates if p.exists()), candidates[0])
            groups.setdefault(target, row['group'])
        indexed = {}
        for url, record in data.items():
            if not isinstance(url, str) or not isinstance(record, dict) or set(record) != {'title', 'group', 'headings', 'body', 'truncated'} or not all(isinstance(record[k], str) for k in ('title', 'group', 'body')) or type(record['truncated']) is not bool or not isinstance(record['headings'], list):
                raise InputError('invalid search record schema')
            if not self.same_origin(url):
                self.fail(path, 'search', 'index URL must be on site')
            target = self.local_path(url)
            if target in indexed or target not in self.pages:
                self.fail(path, 'search', f'index URL duplicate or missing page: {url}')
                continue
            indexed[target] = record
            if record['group'] not in set(groups.values()) | {'none'} or record['group'] != groups.get(target, 'none'):
                self.fail(path, 'search', f'wrong navigation group: {url}')
            if not record['title'].strip():
                self.fail(path, 'search', f'empty title: {url}')
            root = self.pages[target]
            mains = root.all('main')
            content = mains[0] if mains else root
            actual_headings = {n.attrs['id']: words(n.text) for n in content.all() if n.tag in {'h1', 'h2', 'h3', 'h4', 'h5', 'h6'} and n.attrs.get('id')}
            found = {}
            for heading in record['headings']:
                if not isinstance(heading, dict) or set(heading) != {'text', 'anchor'} or not all(isinstance(v, str) for v in heading.values()):
                    raise InputError('invalid search heading schema')
                anchor = urlsplit(heading['anchor'])
                ident = unquote(anchor.fragment)
                if not self.same_origin(heading['anchor']) or self.local_path(heading['anchor']) != target or ident not in actual_headings or words(heading['text']) != actual_headings[ident] or ident in found:
                    self.fail(path, 'search', f'heading differs from rendered content: {heading["anchor"]}')
                found[ident] = heading['text']
            if set(found) != set(actual_headings):
                self.fail(path, 'search', f'heading coverage differs: {url}')
        if set(indexed) != set(self.pages) or not set(groups) <= set(indexed):
            self.fail(path, 'search', 'index must cover every published, sidebar and G10 page')

    def examples(self):
        path = self.docs / '_data/examples.yml'
        rows = load_rows(path)
        blocks, errors = example_blocks(self.docs)
        for source, line, detail in errors:
            self.fail(source, 'examples', detail, line)
        if not rows or rows[0].get('anchor') != 'docs/user-guide.md#quickstart[1]' or rows[0].get('command') != 'go run ./cmd/mythhelm demo':
            self.fail(path, 'examples', 'first example must be guide quickstart')
        expected = {}
        for row in rows:
            if not {'anchor', 'command', 'expect_exit'} <= set(row) or set(row) - {'anchor', 'command', 'expect_exit', 'output_anchor'} or type(row['expect_exit']) is not int or not 0 <= row['expect_exit'] <= 255 or not isinstance(row['command'], str) or not row['command'].strip():
                raise InputError('invalid examples schema')
            for key in ('anchor', 'output_anchor'):
                if key not in row:
                    continue
                anchor = row[key]
                if not isinstance(anchor, str):
                    raise InputError('example anchor must be text')
                block = blocks.get(anchor)
                if anchor in expected or not block or block.annotation != anchor:
                    self.fail(path, 'examples', f'{anchor}: missing, duplicate or unannotated source fence')
                if key == 'anchor' and block and normalise_output(block.text) != normalise_output(row['command']):
                    self.fail(path, 'examples', f'{anchor}: command and fence drift')
                expected[anchor] = block
        flag = self.docs / '_data/example_checks.yml'
        enabled = False
        if flag.exists():
            value = re.sub(r'^\s*#.*$', '', read(flag), flags=re.M).strip()
            if value not in {'enabled: true', 'enabled: false'}:
                raise InputError('invalid example activation flag')
            enabled = value == 'enabled: true'
        rendered, markers = {}, {}
        for page, root in self.pages.items():
            for node in root.all(cls='example-block'):
                anchor = node.attrs.get('data-example-anchor', '')
                rendered.setdefault(anchor, []).append((page, node))
                block = blocks.get(anchor)
                codes = node.all('pre')
                if not block or len(codes) != 1 or normalise_output(codes[0].text).removesuffix('\n') != normalise_output(block.text):
                    self.fail(page, 'examples', 'rendered wrapper does not match source fence', node.line)
            for node in root.all(cls='tested-example'):
                anchor = node.attrs.get('data-example-anchor', '')
                markers.setdefault(anchor, []).append(node)
                if not enabled or anchor not in expected or node.ancestor('data-example-anchor') != anchor or words(node.text) != 'tested in CI':
                    self.fail(page, 'examples', 'marker lacks enabled command/output entry or wrapper', node.line)
        for anchor in expected:
            matches = rendered.get(anchor, [])
            if len(matches) != 1:
                self.fail(path, 'examples', f'{anchor}: expected one rendered block')
            elif self.source_page(anchor.split('#')[0]) != matches[0][0]:
                self.fail(path, 'examples', f'{anchor}: rendered on wrong page')
            if enabled and len(markers.get(anchor, [])) != 1:
                self.fail(path, 'examples', f'{anchor}: expected one enabled marker')

    def source_page(self, source):
        name = source.removeprefix('docs/')
        if name.endswith('README.md'):
            return self.site / name[:-9] / 'index.html'
        return self.site / (name[:-3] + '.html')

    def run(self):
        self.layout()
        self.resources()
        self.styles()
        self.landing()
        self.footer()
        self.chapters()
        self.states()
        self.search()
        self.examples()
        return self.findings


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--site', required=True, type=Path)
    parser.add_argument('--docs', required=True, type=Path)
    args = parser.parse_args()
    try:
        checker = Checker(args.site, args.docs)
        findings = checker.run()
    except (OSError, ValueError, TypeError, KeyError, subprocess.SubprocessError) as error:
        print(f'{args.site}:1: input: {error}')
        return 2
    print('\n'.join(findings) if findings else f'docs-site: PASS pages={len(checker.pages)}')
    return 1 if findings else 0


if __name__ == '__main__':
    raise SystemExit(main())
