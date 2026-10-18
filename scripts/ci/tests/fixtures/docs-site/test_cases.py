#!/usr/bin/env python3
"""Synthetic built-tree fixtures; deliberately broken inputs never enter docs/."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

CHECK = Path(__file__).resolve().parents[3] / 'check_docs_site.py'
sys.path.insert(0, str(CHECK.parent))
import check_docs_site as docs_check  # noqa: E402


class SiteChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='docs-site-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.docs = self.root / 'docs'
        self.site = self.root / '_site'
        self.baseline()

    def test_rendered_state_fallback_fixture(self):
        cases = {'reported': 'reported', 'estimated': 'estimated',
                 'unknown': 'unknown', 'missing': 'unknown', 'empty': 'unknown',
                 'typo': 'unknown', 'observed': 'unknown'}
        def rendered():
            return '<!-- mythhelm-fixture -->' + ''.join(
                f'<div id="{case}"><span class="state state-{value}">{value}</span></div>'
                for case, value in cases.items())
        path = self.root / 'state.html'
        path.write_text(rendered())
        self.assertEqual([], docs_check.state_fixture_findings(path))
        for value in ('', '0', 'reported'):
            cases['empty'] = value
            path.write_text(rendered())
            self.assertTrue(docs_check.state_fixture_findings(path), value)
        cases['empty'] = 'unknown'
        good = rendered()
        for broken in (good.replace('id="missing"', 'id="other"'),
                       good + '<div id="empty">unknown</div>',
                       good.replace('class="state state-unknown"', 'class="state"'),
                       good.replace('<!-- mythhelm-fixture -->', ''),
                       good.replace('>unknown</span>', '>unknown extra</span>')):
            path.write_text(broken)
            self.assertTrue(docs_check.state_fixture_findings(path))

    def baseline(self):
        self.put('docs/_config.yml', 'url: https://example.com\nbaseurl: /mythhelm\n')
        self.put('docs/assets/brand/README.md', '| Night | `#090f1a` | Background |\n| Amber (dark) | `#f0c27b` | Needs you |\n')
        self.put('site/assets/site.css', ':root { --color-night: #090f1a; --color-amber-dark: #f0c27b; } body {color: var(--color-night);}')
        self.put('docs/_data/site-scripts.yml', '- src: assets/site.js\n  purpose: sidebar\n')
        self.put('site/assets/site.js', '// Synthetic fixture\n')
        names = ['index', 'user-guide', 'limitations', 'contributing', 'mirror/LICENSE', 'mirror/SECURITY', 'mirror/CHANGELOG', 'mirror/GOVERNANCE']
        nav = []
        footer = ''
        index = {}
        for name in names:
            nav.append({'name': name, 'path': name + '.md', 'group': 'Start'})
            if name not in ('index', 'user-guide'):
                nav[-1]['footer'] = True
                footer += '<a href="/mythhelm/' + name + '.html">' + name + '</a>'
        self.put('docs/_data/navigation.yml', json.dumps(nav))
        self.footer = '<footer class="site-footer">' + footer + '</footer>'
        for name in names:
            title = 'Quickstart' if name == 'user-guide' else 'Try the scripted demo'
            slug = 'quickstart' if name == 'user-guide' else 'try-the-scripted-demo'
            content = '<h2 id="' + slug + '">' + title + '</h2>'
            source = '## ' + title + '\n'
            if name in ('index', 'user-guide'):
                anchor = 'docs/' + name + '.md#' + slug + '[1]'
                wrapper = '<div class="example-block" data-example-anchor="' + anchor + '"'
                source += wrapper + ' markdown="1">\n\n```sh\ngo run ./cmd/mythhelm demo\n```\n\n</div>\n'
                content += wrapper + '><pre><code>go run ./cmd/mythhelm demo\n</code></pre></div>'
            if name == 'user-guide':
                for state in ('reported', 'estimated', 'unknown'):
                    source += '{% include state.html state="' + state + '" %}\n'
                    content += '<span class="state state-' + state + '">' + state + '</span>'
            self.put('docs/' + name + '.md', source)
            self.put('site/' + name + '.html', '<!-- mythhelm-layout:v1 --><html><head><title>' + name + '</title></head><body><main>' + content + '</main>' + self.footer + '</body></html>')
            url = '/mythhelm/' + ('' if name == 'index' else name + '.html')
            index[url] = {'title': name, 'group': 'Start', 'headings': [{'text': title, 'anchor': url + '#' + slug}], 'body': title, 'truncated': False}
        index['/mythhelm/']['headings'].append({'text': 'Headline', 'anchor': '/mythhelm/#landing-title'})
        self.put('site/search.json', json.dumps(index))
        self.put('docs/_data/examples.yml', json.dumps([
            {'anchor': 'docs/user-guide.md#quickstart[1]', 'command': 'go run ./cmd/mythhelm demo', 'expect_exit': 0},
            {'anchor': 'docs/index.md#try-the-scripted-demo[1]', 'command': 'go run ./cmd/mythhelm demo', 'expect_exit': 0}]))
        self.put('docs/limitations.md', '## Supported today\nWorks offline.\n## Not supported yet\nRouting is planned (#42).\n')
        status = [dict(id='working', section='Supported today', quote='Works offline.', column='works', tracker=None),
                  dict(id='routing', section='Not supported yet', quote='Routing is planned (#42).', column='in-progress', tracker='#42')]
        self.put('docs/_data/status.yml', json.dumps(status))
        panel = '<h1 id="landing-title">Headline</h1><p class="status-link"><a href="#status">What ships</a></p><section id="status">'
        for item in status:
            panel += '<section data-status-column="' + item['column'] + '"><li data-status-id="' + item['id'] + '" data-source-section="' + item['section'] + '"><div class="status-quote">' + item['quote'] + '</div></li></section>'
        panel += '</section>'
        for _ in range(4):
            panel += '<article class="capability" data-status-id="working" data-status="works"><span class="capability-label">works</span><div class="capability-evidence">Works offline.</div></article>'
        self.replace('site/index.html', '<main>', '<main>' + panel)
        self.put('site/demos/manifest.json', json.dumps({'recordings': [{'name': 'hero', 'chapters': [{'name': n, 'at_seconds': t} for n, t in zip(['title', 'run', 'kill', 'recover', 'review'], [0, 2, 13, 19, 26])]}]}))
        self.put('site/demos/hero.webm', 'synthetic boundary input')
        self.put('site/demos/hero.mp4', 'synthetic boundary input')
        binary = self.root / 'bin'
        binary.mkdir()
        probe = binary / 'ffprobe'
        probe.write_text('#!/usr/bin/env python3\nprint("{\\"format\\":{\\"duration\\":\\"32.24\\"}}")\n')
        probe.chmod(0o755)
        self.env = dict(os.environ, PATH=str(binary) + os.pathsep + os.environ['PATH'])

    def replace(self, name, before, after):
        root, relative = name.split('/', 1)
        path = (self.site if root == 'site' else self.docs) / relative
        text = path.read_text()
        assert before in text, (name, before)
        path.write_text(text.replace(before, after))

    def put(self, name, text):
        root, relative = name.split('/', 1)
        path = (self.site if root == 'site' else self.docs) / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        if root == 'site' and path.suffix == '.html':
            text = '<!-- mythhelm-fixture -->' + text
        path.write_text(text)
        return path

    def run_check(self, category=None, code=1):
        result = subprocess.run(['python3', str(CHECK), '--site', str(self.site),
                                 '--docs', str(self.docs)], capture_output=True, text=True, env=self.env)
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        if category:
            self.assertIn(': ' + category + ':', result.stdout + result.stderr)

    def test_conforming(self):
        self.run_check(code=0)

    def test_relative_mirror_footer_and_script(self):
        self.replace('site/mirror/LICENSE.html', '/mythhelm/', '../')
        self.replace('site/mirror/LICENSE.html', '<head>', '<head><script src="../assets/site.js"></script>')
        self.run_check(code=0)

    def test_site_path_with_parent_segment(self):
        self.site = self.site / 'mirror' / '..'
        self.run_check(code=0)

    def test_numbered_list_fence_indentation(self):
        self.replace('docs/user-guide.md', '```sh\ngo run ./cmd/mythhelm demo\n```', '1. Run:\n\n   ```sh\n   go run ./cmd/mythhelm demo\n   ```')
        self.run_check(code=0)

    def test_invalid_manifest_shapes_are_input_errors(self):
        for manifest in [[], {'recordings': {}}, {'recordings': ['hero']},
                         {'recordings': [{'name': 'hero', 'chapters': {}}]},
                         {'recordings': [{'name': 'hero', 'chapters': ['title']}]}]:
            with self.subTest(manifest=manifest):
                self.put('site/demos/manifest.json', json.dumps(manifest))
                self.run_check('input', code=2)

    def test_unlisted_hex(self):
        self.replace('site/assets/site.css', 'color: var(--color-night)', 'color: #123456')
        self.run_check('tokens')

    def test_table_drift(self):
        self.replace('docs/assets/brand/README.md', '#090f1a', '#090f1b')
        self.run_check('tokens')

    def test_amber_reference(self):
        self.replace('site/assets/site.css', 'color: var(--color-night)', 'color: var(--color-amber-dark)')
        self.run_check('amber')

    def test_off_origin_script(self):
        self.replace('site/index.html', '<main>', '<script src="https://example.org/track.js"></script><main>')
        self.run_check('origins')

    def test_off_origin_import(self):
        self.replace('site/assets/site.css', ':root', '@import "https://example.org/font.css"; :root')
        self.run_check('origins')

    def test_unlisted_script(self):
        self.put('site/assets/metrics.js', '// unlisted')
        self.run_check('scripts')

    def test_removed_panel_phrase(self):
        self.replace('docs/limitations.md', 'Works offline.', 'Unavailable.')
        self.run_check('panel')

    def test_wrong_column(self):
        self.replace('docs/_data/status.yml', '"column": "in-progress"', '"column": "works"')
        self.run_check('panel')

    def test_tracker_outside_quote(self):
        self.replace('docs/_data/status.yml', '"tracker": "#42"', '"tracker": "#99"')
        self.replace('docs/limitations.md', 'Routing is planned (#42).', 'Routing is planned (#42). Other work (#99).')
        self.run_check('panel')

    def test_cell_label_mismatch(self):
        self.replace('site/index.html', 'class="capability-label">works', 'class="capability-label">not yet')
        self.run_check('panel')

    def test_missing_footer_target(self):
        (self.site / 'mirror/LICENSE.html').unlink()
        self.run_check('footer')

    def test_zero_panel_links(self):
        self.replace('site/index.html', '<a href="#status">What ships</a>', '')
        self.run_check('panel-link')

    def test_two_panel_links(self):
        self.replace('site/index.html', '<a href="#status">What ships</a>', '<a href="#status">What ships</a><a href="#status">Again</a>')
        self.run_check('panel-link')

    def test_nonincreasing_chapters(self):
        self.replace('site/demos/manifest.json', '"at_seconds": 2}', '"at_seconds": 0}')
        self.run_check('chapters')

    def test_chapter_exceeds_video(self):
        self.replace('site/demos/manifest.json', '"at_seconds": 26', '"at_seconds": 33')
        self.run_check('chapters')

    def test_wrapped_non_state(self):
        self.replace('site/user-guide.html', '>reported</span>', '>ready</span>')
        self.run_check('states')

    def test_empty_state_argument(self):
        self.replace('docs/user-guide.md', 'state="reported"', 'state=""')
        self.run_check('states')

    def test_misspelled_state_argument(self):
        self.replace('docs/user-guide.md', 'state="estimated"', 'state="estimeted"')
        self.run_check('states')

    def test_observed_state_argument(self):
        self.replace('docs/user-guide.md', 'state="estimated"', 'state="observed"')
        self.run_check('states')

    def test_no_states(self):
        self.replace('site/user-guide.html', 'class="state state-', 'class="removed-')
        self.run_check('states')

    def test_missing_sidebar_page(self):
        path = self.site / 'search.json'
        data = json.loads(path.read_text())
        del data['/mythhelm/user-guide.html']
        path.write_text(json.dumps(data))
        self.run_check('search')

    def test_missing_g10_page(self):
        path = self.site / 'search.json'
        data = json.loads(path.read_text())
        del data['/mythhelm/mirror/LICENSE.html']
        path.write_text(json.dumps(data))
        self.run_check('search')

    def test_index_too_large(self):
        self.replace('site/search.json', '"body": "Quickstart"', '"body": "' + 'x' * 300001 + '"')
        self.run_check('search')

    def test_unknown_group(self):
        self.replace('site/search.json', '"group": "Start"', '"group": "Other"')
        self.run_check('search')

    def test_listed_page_group_none(self):
        self.replace('site/search.json', '"group": "Start"', '"group": "none"')
        self.run_check('search')

    def test_unresolved_liquid(self):
        self.replace('site/search.json', '"body": "Quickstart"', '"body": "{{ missing }}"')
        self.run_check('search')

    def enable_examples(self):
        self.put('docs/_data/example_checks.yml', 'enabled: true\n')
        for page, slug in [('user-guide', 'quickstart'), ('index', 'try-the-scripted-demo')]:
            anchor = 'docs/' + page + '.md#' + slug + '[1]'
            opening = '<div class="example-block" data-example-anchor="' + anchor + '">'
            self.replace('site/' + page + '.html', opening, opening + '<span class="tested-example" data-example-anchor="' + anchor + '">tested in CI</span>')

    def test_enabled_examples(self):
        self.enable_examples()
        self.run_check(code=0)

    def test_hand_marked_block(self):
        self.replace('site/index.html', '<main>', '<main><span class="tested-example" data-example-anchor="docs/index.md#other[1]">tested in CI</span>')
        self.run_check('examples')

    def test_empty_examples(self):
        self.put('docs/_data/examples.yml', '[]')
        self.run_check('examples')

    def test_first_entry_not_quickstart(self):
        path = self.docs / '_data/examples.yml'
        path.write_text(json.dumps(list(reversed(json.loads(path.read_text())))))
        self.run_check('examples')

    def test_entry_command_drift(self):
        self.replace('docs/_data/examples.yml', 'go run ./cmd/mythhelm demo', 'go run ./cmd/mythhelm demo --changed')
        self.run_check('examples')

    def test_source_block_drift(self):
        self.replace('docs/user-guide.md', 'go run ./cmd/mythhelm demo', 'go run ./cmd/mythhelm demo --changed')
        self.run_check('examples')

    def test_wrong_source_annotation(self):
        self.replace('docs/user-guide.md', '#quickstart[1]', '#quickstart[2]')
        self.run_check('examples')

    def test_wrong_page_annotation(self):
        self.replace('docs/user-guide.md', 'docs/user-guide.md#quickstart[1]', 'docs/index.md#quickstart[1]')
        self.run_check('examples')

    def test_marked_output_without_output_anchor(self):
        self.enable_examples()
        self.replace('site/user-guide.html', '</main>', '<div class="example-block" data-example-anchor="docs/user-guide.md#quickstart[2]"><span class="tested-example" data-example-anchor="docs/user-guide.md#quickstart[2]">tested in CI</span><pre><code>output</code></pre></div></main>')
        self.run_check('examples')

    def test_raw_indented_code_does_not_shift_fences(self):
        self.enable_examples()
        self.replace('docs/user-guide.md', '## Quickstart\n', '## Quickstart\n\n<pre><code>raw</code></pre>\n\n    indented code\n\n')
        self.run_check(code=0)

    def test_missing_enabled_marker(self):
        self.put('docs/_data/example_checks.yml', 'enabled: true\n')
        self.run_check('examples')

    def test_imported_colour_literal(self):
        self.replace('site/assets/site.css', ':root', '@import "extra.css"; :root')
        self.put('site/assets/extra.css', 'body {color: #123456;}')
        self.run_check('tokens')

    def test_variable_colour_fallback(self):
        self.replace('site/assets/site.css', 'var(--color-night)', 'var(--color-night, red)')
        self.run_check('tokens')

    def test_colour_bearing_shorthands_reject_named_colours(self):
        for declaration in ('text-decoration: underline red', 'text-emphasis: filled red',
                            'column-rule: 1px solid red', 'border-inline: 1px solid red',
                            'border-block-end: 1px solid red'):
            with self.subTest(declaration=declaration):
                self.put('site/assets/site.css', ':root { --color-night: #090f1a; --color-amber-dark: #f0c27b; } body {' + declaration + ';}')
                self.run_check('tokens')

    def test_colour_bearing_shorthands_accept_tokens(self):
        self.replace('site/assets/site.css', 'color: var(--color-night)',
                     'text-decoration: underline wavy var(--color-night); text-emphasis: filled var(--color-night); column-rule: 1px solid var(--color-night); border-inline: 1px solid var(--color-night)')
        self.run_check(code=0)

    def test_mixed_backdrop_opacity_rejected(self):
        self.replace('site/assets/site.css', 'body {color: var(--color-night);}',
                     'body, dialog::backdrop { opacity: 0.5; }')
        self.run_check('tokens')

    def test_exclusive_backdrop_opacity_allowed(self):
        self.replace('site/assets/site.css', 'body {color: var(--color-night);}',
                     'dialog::backdrop, #viewer::backdrop { opacity: 0.5; }')
        self.run_check(code=0)

    def test_backdrop_inside_negation_does_not_exempt_opacity(self):
        self.replace('site/assets/site.css', 'body {color: var(--color-night);}',
                     'body:not(::backdrop) { opacity: 0.5; }')
        self.run_check('tokens')

    def test_border_shorthand_named_colour(self):
        self.replace('site/assets/site.css', 'color: var(--color-night)', 'border-top: 1px solid red')
        self.run_check('tokens')

    def test_off_origin_footer(self):
        self.replace('site/index.html', 'href="/mythhelm/mirror/LICENSE.html"', 'href="https://example.org/mythhelm/mirror/LICENSE.html"')
        self.run_check('footer')

    def test_off_origin_index_url(self):
        self.replace('site/search.json', '"/mythhelm/user-guide.html":', '"https://example.org/mythhelm/user-guide.html":')
        self.run_check('search')

    def test_bad_json(self):
        self.put('site/search.json', '{ broken')
        self.run_check('input', code=2)

    def test_bad_yaml(self):
        self.put('docs/_data/examples.yml', '- anchor: &reference value')
        self.run_check('input', code=2)

    def second_fence(self, output=False):
        anchor = 'docs/user-guide.md#quickstart[2]'
        source = self.docs / 'user-guide.md'
        source.write_text(source.read_text() + '\n<pre><code>raw between</code></pre>\n\n    indented between\n\n<div class="example-block" data-example-anchor="' + anchor + '" markdown="1">\n\n```text\necho sample\n```\n\n</div>\n')
        self.replace('site/user-guide.html', '</main>', '<div class="example-block" data-example-anchor="' + anchor + '"><span class="tested-example" data-example-anchor="' + anchor + '">tested in CI</span><pre><code>echo sample\n</code></pre></div></main>')
        path = self.docs / '_data/examples.yml'
        rows = json.loads(path.read_text())
        if output:
            rows[0]['output_anchor'] = anchor
        else:
            rows.append({'anchor': anchor, 'command': 'echo sample', 'expect_exit': 0})
        path.write_text(json.dumps(rows))

    def test_raw_indented_between_two_listed_fences(self):
        self.enable_examples()
        self.second_fence()
        self.run_check(code=0)

    def test_output_anchor_marker(self):
        self.enable_examples()
        self.second_fence(output=True)
        self.run_check(code=0)

    def test_unlisted_annotation_cannot_earn_marker(self):
        self.enable_examples()
        self.second_fence()
        path = self.docs / '_data/examples.yml'
        path.write_text(json.dumps(json.loads(path.read_text())[:2]))
        self.run_check('examples')

    def test_rendered_command_drift(self):
        self.replace('site/user-guide.html', 'go run ./cmd/mythhelm demo', 'go run ./cmd/mythhelm wrong')
        self.run_check('examples')

    def test_duplicate_marker(self):
        self.enable_examples()
        marker = '<span class="tested-example" data-example-anchor="docs/user-guide.md#quickstart[1]">tested in CI</span>'
        self.replace('site/user-guide.html', marker, marker + marker)
        self.run_check('examples')

    def test_missing_ffprobe_is_error(self):
        (self.root / 'bin/ffprobe').write_text('#!/usr/bin/env python3\nraise SystemExit(7)\n')
        self.run_check('input', code=2)

    def test_unquoted_imported_colour_literal(self):
        self.replace('site/assets/site.css', ':root', '@import url(extra.css); :root')
        self.put('site/assets/extra.css', 'body {color: #123456;}')
        self.run_check('tokens')

    def test_unquoted_imported_amber(self):
        self.replace('site/assets/site.css', ':root', '@import url(extra.css); :root')
        self.put('site/assets/extra.css', 'body {color: var(--color-amber-dark);}')
        self.run_check('amber')

    def test_gradient_named_colours(self):
        self.replace('site/assets/site.css', 'color: var(--color-night)', 'background-image: linear-gradient(red, blue)')
        self.run_check('tokens')

    def test_fence_preserves_html_and_comments(self):
        command = 'printf "<pre>hello</pre>"\n# <!-- literal -->'
        self.put('docs/extra.md', '## Example\n```sh\n' + command + '\n```\n')
        blocks, errors = docs_check.example_blocks(self.docs)
        self.assertEqual(errors, [])
        self.assertEqual(blocks['docs/extra.md#example[1]'].text, command)

    def test_off_origin_stylesheet_exits_one(self):
        self.replace('site/index.html', '<head>', '<head><link rel="stylesheet" href="https://example.org/theme.css">')
        self.run_check('origins')

    def test_inline_imported_colour_literal(self):
        self.replace('site/index.html', '<head>', '<head><style>@import url(assets/extra.css);</style>')
        self.put('site/assets/extra.css', 'body {color: #123456;}')
        self.run_check('tokens')

    def test_missing_layout(self):
        self.put('site/index.html', '<html><body>Home</body></html>')
        self.run_check('layout')


if __name__ == '__main__':
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(SiteChecks)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'checks={result.testsRun} failed={len(result.failures) + len(result.errors)}')
    raise SystemExit(not result.wasSuccessful())
