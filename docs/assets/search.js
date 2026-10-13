/* Static docs search. The index is loaded once, only when search is opened. */
(() => {
  const opener = document.getElementById('search-open');
  const dialog = document.getElementById('site-search');
  if (!opener || !dialog || typeof dialog.showModal !== 'function') return;

  const input = document.getElementById('search-query');
  const status = document.getElementById('search-status');
  const results = document.getElementById('search-results');
  const incomplete = document.getElementById('search-incomplete');
  const decoder = new DOMParser();
  let indexPromise;
  let revision = 0;
  // Parse only individual entity tokens, never document text as markup.
  const text = (value) => value.replace(/&(?:#\d+|#x[0-9a-f]+|[a-z][a-z0-9]+);/gi,
    (entity) => decoder.parseFromString(entity, 'text/html').body.textContent);
  const normalize = (value) => value.toLocaleLowerCase().normalize('NFKC');

  function loadIndex() {
    if (!indexPromise) {
      indexPromise = fetch(dialog.dataset.index)
        .then((response) => {
          if (!response.ok) throw new Error('Index unavailable');
          return response.json();
        })
        .then((data) => Object.entries(data).map(([url, record]) => ({
          url,
          title: text(record.title),
          group: record.group,
          body: normalize(text(record.body)),
          truncated: record.truncated,
          headings: record.headings.map((heading) => ({
            text: text(heading.text), anchor: heading.anchor,
          })),
        })))
        .catch((error) => {
          indexPromise = undefined;
          throw error;
        });
    }
    return indexPromise;
  }

  function linkItem(url, label, detail) {
    const item = document.createElement('li');
    const link = document.createElement('a');
    link.href = url;
    link.textContent = label;
    const context = document.createElement('span');
    context.className = 'search-detail';
    context.textContent = detail;
    link.append(context);
    item.append(link);
    return item;
  }

  async function search() {
    const current = ++revision;
    const query = normalize(input.value.trim());
    results.replaceChildren();
    incomplete.replaceChildren();
    status.textContent = 'Loading search index…';
    try {
      const pages = await loadIndex();
      if (current !== revision || !dialog.open) return;
      if (!query) {
        status.textContent = 'Search page titles, headings and indexed text.';
        return;
      }
      const terms = query.split(/\s+/);
      const matches = (value) => terms.every((term) => normalize(value).includes(term));
      const found = [];
      const partial = [];
      for (const page of pages) {
        const context = `${page.group === 'none' ? 'Documentation' : page.group}${page.truncated ? ' · Partial text indexed' : ''}`;
        const titleMatch = matches(page.title);
        const headings = page.headings.filter((heading) => matches(heading.text));
        if (titleMatch) found.push({score: 3, url: page.url, label: page.title, context});
        for (const heading of headings) {
          found.push({score: 2, url: heading.anchor, label: heading.text, context: `${page.title} · ${context}`});
        }
        if (!titleMatch && !headings.length && matches(page.body)) {
          found.push({score: 1, url: page.url, label: page.title, context: `${context} · Text match`});
        }
        if (page.truncated) partial.push(page);
      }
      found.sort((a, b) => b.score - a.score || a.label.localeCompare(b.label) || a.url.localeCompare(b.url));
      for (const result of found) results.append(linkItem(result.url, result.label, result.context));
      status.textContent = found.length ? `${found.length} result${found.length === 1 ? '' : 's'}.` : 'No matches in indexed text.';
      if (partial.length) {
        const note = document.createElement('p');
        note.textContent = 'Some page text is omitted from the index. A missing result does not rule out a match on these pages; open a page and use your browser’s Find command.';
        const list = document.createElement('ul');
        for (const page of partial) list.append(linkItem(page.url, page.title, 'Partial text indexed'));
        incomplete.append(note, list);
        status.textContent += ` ${partial.length} page${partial.length === 1 ? ' has' : 's have'} partial text indexed.`;
      }
    } catch {
      if (current !== revision || !dialog.open) return;
      status.textContent = 'Search could not load. Check your connection, then type again or reopen search to retry. The documentation links remain available.';
    }
  }

  function open() {
    if (!dialog.open) dialog.showModal();
    input.focus();
    search();
  }
  opener.hidden = false;
  opener.addEventListener('click', open);
  document.getElementById('search-close').addEventListener('click', () => dialog.close());
  dialog.addEventListener('close', () => { revision += 1; opener.focus({preventScroll: true}); });
  input.addEventListener('input', search);
  dialog.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      dialog.close();
    }
  });
  dialog.addEventListener('submit', (event) => event.preventDefault());
  dialog.addEventListener('click', (event) => {
    if (event.target.closest('a')) dialog.close();
  });
  document.addEventListener('keydown', (event) => {
    const target = event.target;
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey ||
        target.isContentEditable || target.closest('input, textarea, select, [role="textbox"]')) return;
    event.preventDefault();
    open();
  });
})();
