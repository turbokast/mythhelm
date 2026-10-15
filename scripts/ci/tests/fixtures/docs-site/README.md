# Built-site rejection fixtures

`test_cases.py` creates a small synthetic rendered site and matching source data
in a temporary directory, then changes one contract per named test. The shell
entrypoint reports every assertion and a `checks=N failed=M` summary. No fixture
is copied into `docs/`, indexed, or deployed.

The fixture uses a process-boundary `ffprobe` stub returning a known duration;
chapter tests change only offsets, and a failed probe must return input error 2.
The checker itself invokes real `ffprobe` on both shipped hero formats when run
against a Pages build. No example command is executed by this checker or suite.

The conforming shape includes the layout marker, six footer destinations,
status source/column/tracker bindings, four cells, all three states, a complete
search index, and explicit Markdown-fence annotations. Both default unmarked
builds and temporarily enabled example markers are exercised. Raw HTML and
indented code before and between fenced examples do not consume fence ordinals;
literal HTML inside a fence remains command/output text.

Colour checks follow linked and imported stylesheet files plus inline styles.
Unreferenced theme CSS emitted by Pages is permitted by AC-1.1; origin checks
still inspect every emitted CSS file. Quotes preserve underscore-bearing code
identifiers and normalize only inline backticks/emphasis and whitespace.
