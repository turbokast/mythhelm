/* The hero loads its chapter offsets from the recording manifest on request. */
(() => {
  const chapters = document.querySelector('.hero-chapters');
  const video = document.querySelector('#hero-player');
  if (!chapters || !video) return;
  const status = document.querySelector('.chapter-status');
  const buttons = [...chapters.querySelectorAll('[data-chapter]')];
  let manifest;
  let request = 0;

  async function offsets() {
    if (!manifest) {
      manifest = fetch(chapters.dataset.manifest)
        .then(response => {
          if (!response.ok) throw new Error('manifest unavailable');
          return response.json();
        })
        .then(data => {
          const beats = data.recordings.find(recording => recording.name === 'hero').chapters;
          if (beats.length !== buttons.length || beats.some((beat, index) =>
            beat.name !== buttons[index].dataset.chapter ||
            !Number.isFinite(beat.at_seconds) || beat.at_seconds < 0 ||
            (index > 0 && beat.at_seconds <= beats[index - 1].at_seconds))) {
            throw new Error('invalid chapter offsets');
          }
          return beats;
        })
        .catch(error => { manifest = null; throw error; });
    }
    return manifest;
  }

  function metadata() {
    if (video.readyState >= 1) return Promise.resolve();
    return new Promise((resolve, reject) => {
      function cleanup() {
        video.removeEventListener('loadedmetadata', loaded);
        video.removeEventListener('error', failed);
      }
      function loaded() { cleanup(); resolve(); }
      function failed() { cleanup(); reject(new Error('video unavailable')); }
      video.addEventListener('loadedmetadata', loaded);
      video.addEventListener('error', failed);
      video.preload = 'metadata';
      video.load();
    });
  }

  chapters.hidden = false;
  for (const button of buttons) {
    button.addEventListener('click', async () => {
      const current = ++request;
      status.textContent = 'Loading chapter…';
      try {
        const beats = await offsets();
        if (current !== request) return;
        await metadata();
        if (current !== request) return;
        if (!Number.isFinite(video.duration) || beats.some(beat => beat.at_seconds >= video.duration)) {
          throw new Error('chapter outside recording');
        }
        const beat = beats.find(chapter => chapter.name === button.dataset.chapter);
        video.currentTime = beat.at_seconds;
        status.textContent = `${button.textContent.trim()} chapter selected. Use the player to play.`;
      } catch (_) {
        if (current === request) status.textContent = 'Chapters unavailable. Use the video controls or read the transcript.';
      }
    });
  }
})();

/* Navigation remains a plain list until its control has been wired. */
(() => {
  const button = document.querySelector('.sidebar-toggle');
  const nav = document.querySelector('#docs-navigation');
  if (!button || !nav) return;
  const narrow = matchMedia('(max-width: 1023px)');
  function sync() {
    nav.hidden = narrow.matches && button.getAttribute('aria-expanded') !== 'true';
    button.hidden = !narrow.matches;
  }
  button.addEventListener('click', () => {
    button.setAttribute('aria-expanded', String(button.getAttribute('aria-expanded') !== 'true'));
    sync();
  });
  nav.addEventListener('keydown', event => {
    if (event.key === 'Escape' && narrow.matches) {
      button.setAttribute('aria-expanded', 'false');
      sync();
      button.focus();
    }
  });
  narrow.addEventListener('change', sync);
  sync();
})();

/* A GIF URL is never an image source until a reader requests motion. */
(() => {
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  for (const recording of document.querySelectorAll('.recording')) {
    const button = recording.querySelector('.recording-toggle');
    const poster = recording.querySelector('.recording-poster');
    const video = recording.querySelector('video');
    const still = poster.src;
    const name = recording.dataset.recording;
    function stop() {
      if (video) {
        video.pause();
        video.hidden = true;
      }
      poster.src = still;
      poster.hidden = false;
      button.setAttribute('aria-pressed', 'false');
      button.textContent = `Start ${name} recording`;
    }
    function preference() {
      if (reduced.matches && !video) stop();
      button.disabled = reduced.matches && !video;
      if (button.disabled) button.textContent = `${name} poster — reduced motion enabled`;
      else if (button.getAttribute('aria-pressed') !== 'true') button.textContent = `Start ${name} recording`;
    }
    button.addEventListener('click', () => {
      if (button.getAttribute('aria-pressed') === 'true') { stop(); return; }
      if (video) {
        for (const source of video.querySelectorAll('source')) source.src = source.dataset.src;
        poster.hidden = true;
        video.hidden = false;
        video.load();
        video.play().catch(() => { /* Native controls remain usable if playback is refused. */ });
      } else {
        if (reduced.matches) return;
        poster.src = button.dataset.src;
      }
      button.setAttribute('aria-pressed', 'true');
      button.textContent = `Stop ${name} recording`;
    });
    reduced.addEventListener('change', preference);
    preference();
    button.hidden = false;
  }
})();
