#!/usr/bin/env node
// Measure a built site in the runner's installed Chrome. Argument: site directory.
// Exit 0: conforming; 1: findings or unavailable browser; 2: invalid input.
// Starts a loopback-only static server and isolated browser, both closed on exit.
import { chromium } from 'playwright-core';
import { createServer } from 'node:http';
import { readFile, readdir, stat, realpath } from 'node:fs/promises';
import { resolve, relative, extname, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
const findings = [];
const report = (page, check, detail) => { const finding = `${page}:1: ${check}: ${detail}`; findings.push(finding); console.error(finding); };
let browser, server;
try {
  if (process.argv.length !== 3) throw new Error('usage: node scripts/ci/browser/check_layout.mjs _site');
  const site = await realpath(resolve(process.argv[2]));
  const pages = [];
  async function walk(dir) {
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isSymbolicLink()) throw new Error(`symlink not allowed: ${path}`);
      if (entry.isDirectory()) await walk(path);
      else if (entry.name.endsWith('.html')) pages.push(relative(site, path));
    }
  }
  await walk(site);
  if (!pages.length) throw new Error('site contains no HTML pages');
  const brand = await readFile(join(repo, 'docs/assets/brand/README.md'), 'utf8');
  const palette = [...brand.matchAll(/\|[^|]+\| `(#\w{6})` \|/g)].map(m => m[1].toLowerCase());
  if (palette.length !== 9) throw new Error('cannot parse the nine declared brand colors');
  const config = await readFile(join(repo, 'docs/_config.yml'), 'utf8');
  const base = config.match(/^baseurl:\s*["']?([^\s"']*)/m)?.[1] || '';
  const chrome = process.env.CHROME_BIN || '/usr/bin/google-chrome';
  let version;
  try { version = execFileSync(chrome, ['--version'], { encoding: 'utf8' }).trim(); }
  catch { throw new Error(`browser-source: preinstalled Chrome unavailable at ${chrome}`); }
  console.log(`browser-source: ${chrome}; version: ${version}`);
  const types = { '.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript', '.json': 'application/json', '.svg': 'image/svg+xml', '.png': 'image/png', '.gif': 'image/gif', '.woff2': 'font/woff2', '.webm': 'video/webm', '.mp4': 'video/mp4' };
  server = createServer(async (req, res) => {
    try {
      let path = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
      if (base && path.startsWith(`${base}/`)) path = path.slice(base.length);
      let file = resolve(site, `.${path}`);
      if (!file.startsWith(`${site}/`) && file !== site) throw new Error('outside site');
      if ((await stat(file)).isDirectory()) file = join(file, 'index.html');
      const data = await readFile(file);
      res.writeHead(200, { 'Content-Type': types[extname(file)] || 'application/octet-stream', 'Content-Length': data.length }); res.end(data);
    } catch { res.writeHead(404); res.end('not found'); }
  });
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const origin = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({ executablePath: chrome, headless: true });
  for (const file of pages.sort()) {
    for (const width of [390, 1280]) {
      for (const reducedMotion of ['no-preference', 'reduce']) {
        const context = await browser.newContext({ viewport: { width, height: width === 390 ? 844 : 800 }, reducedMotion, serviceWorkers: 'block' });
        const page = await context.newPage();
        const requests = [], responses = [];
        await context.route('**/*', async route => {
          const url = route.request().url(); requests.push(url);
          if (new URL(url).origin !== origin) { report(file, 'origin', url); await route.abort(); }
          else await route.continue();
        });
        page.on('response', response => responses.push(response));
        await page.goto(`${origin}${base}/${file}`, { waitUntil: 'networkidle' });
        await page.evaluate(() => document.fonts.ready);
        const measure = ({ palette, width, reducedMotion, initial = true }) => {
          const errors = [];
          const add = (kind, message) => errors.push([kind, message]);
          const name = el => `${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''}${el.className && typeof el.className === 'string' ? '.' + el.className.trim().replaceAll(' ', '.') : ''}`;
          const visible = el => el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) && el.getClientRects().length;
          const rgb = value => { const m = value.match(/^rgba?\(([^)]+)\)$/); if (!m) throw new Error(`unparsed color: ${value}`); return m[1].split(',').map(Number); };
          const hex = c => '#' + c.slice(0, 3).map(x => Math.round(x).toString(16).padStart(2, '0')).join('');
          const background = el => {
            for (let p = el; p; p = p.parentElement) {
              const color = rgb(getComputedStyle(p).backgroundColor);
              if (color.length < 4 || color[3] === 1) return color;
              if (color[3] !== 0) { add('undeclared', `${name(p)} translucent background`); return color; }
            }
            add('undeclared', `${name(el)} no declared opaque background`); return [255,255,255];
          };
          const luminance = color => color.slice(0,3).map(c => c/255).map(c => c <= .04045 ? c/12.92 : ((c+.055)/1.055)**2.4).reduce((a,c,i) => a+c*[.2126,.7152,.0722][i],0);
          const pairs = new Set();
          const contrast = (el, fg, bg, threshold, kind) => {
            if (fg.length === 4 && fg[3] !== 1) add('undeclared', `${name(el)} translucent foreground`);
            let opacity = 1;
            for (let p=el;p;p=p.parentElement) opacity *= Number(getComputedStyle(p).opacity);
            if (opacity !== 1) fg = fg.slice(0,3).map((v,i) => opacity*v+(1-opacity)*bg[i]);
            const key = `${hex(fg)}/${hex(bg)}/${threshold}`;
            if (pairs.has(key)) return; pairs.add(key);
            if (!palette.includes(hex(fg)) || !palette.includes(hex(bg))) add('undeclared', `${name(el)} ${kind} ${key}`);
            const a=luminance(fg), b=luminance(bg), ratio=(Math.max(a,b)+.05)/(Math.min(a,b)+.05);
            if (ratio + .001 < threshold) add('contrast', `${name(el)} ${kind} ${key}: ${ratio.toFixed(2)}:1`);
          };
          if (document.documentElement.scrollWidth > width) add('wide', `document scrollWidth ${document.documentElement.scrollWidth} > ${width}`);
          for (const el of document.querySelectorAll('body *')) {
            if (!visible(el)) continue;
            const style = getComputedStyle(el), rect=el.getBoundingClientRect();
            let scrollContainer=false;
            for (let p=el.parentElement;p && p!==document.body;p=p.parentElement) {
              if (['auto','scroll'].includes(getComputedStyle(p).overflowX)) { scrollContainer=true; break; }
            }
            if (!scrollContainer && (rect.left < -.5 || rect.right > width+.5)) add('clip', `${name(el)} box ${rect.left.toFixed(1)}..${rect.right.toFixed(1)} outside ${width}`);
            if (width===390 && el.matches('nav a, .on-this-page a, button, input:not([type=hidden]), select, textarea, [role=button]') && (rect.width<43.99 || rect.height<43.99)) add('target', `${name(el)} ${rect.width.toFixed(1)}x${rect.height.toFixed(1)}`);
            if ([...el.childNodes].some(n => n.nodeType===Node.TEXT_NODE && n.textContent.trim()) || el.matches('input:not([type=hidden]), textarea, select')) {
              const size=parseFloat(style.fontSize), large=size>=24 || (size>=18.66 && parseInt(style.fontWeight)>=700);
              contrast(el,rgb(style.color),background(el),large?3:4.5,'text');
              if (el.matches('input[placeholder], textarea[placeholder]') && !el.value) {
                const placeholder=getComputedStyle(el,'::placeholder');
                const color=rgb(placeholder.color), bg=background(el);
                const alpha=(color[3] ?? 1)*Number(placeholder.opacity);
                contrast(el,color.slice(0,3).map((v,i)=>alpha*v+(1-alpha)*bg[i]),bg,large?3:4.5,'placeholder');
              }
            }
            if (el.matches('button, input, select, textarea')) {
              if (parseFloat(style.borderTopWidth)>0 && style.borderTopStyle!=='none') contrast(el,rgb(style.borderTopColor),background(el.parentElement),3,'control boundary');
            }
          }
          if (document.querySelector('img[src$=".gif"], video[autoplay]')) add('gif','recording animates before request');
          for (const video of document.querySelectorAll('video')) {
            if (video.preload!=='none') add('preload','video must preload none');
            if (!video.paused) add('motion','video plays before request');
          }
          for (const recording of document.querySelectorAll('.recording')) {
            const img=recording.querySelector('img');
            if (!img || !img.getAttribute('src')?.includes('poster') || !img.alt.toLowerCase().includes('scripted')) add('poster','recording lacks scripted static poster');
          }
          if (reducedMotion==='reduce' && document.getAnimations().some(a=>a.playState==='running')) add('motion','unprompted animation with reduced motion');
          if (initial && document.querySelector('#landing-title')) {
            const quick=document.querySelector('#quickstart pre, pre#quickstart');
            if (!quick || quick.getBoundingClientRect().top<0 || quick.getBoundingClientRect().bottom>innerHeight) add('quickstart','command block not inside first viewport');
          }
          return {errors, pairs:pairs.size, landing:!!document.querySelector('#landing-title')};
        };
        const measured = await page.evaluate(measure, { palette, width, reducedMotion });
        for (const [kind, detail] of measured.errors) report(file,kind,`${width}px/${reducedMotion}: ${detail}`);
        for (const url of requests) if (/\.gif(?:[?#]|$)/i.test(url)) report(file,'gif',`initial request ${url}`);
        if (measured.landing) {
          let bytes=0;
          for (const response of responses) {
            if (new URL(response.url()).origin !== origin) continue;
            if (/\/hero\.(webm|mp4)(?:[?#]|$)/.test(response.url())) { report(file,'preload','hero fetched before request'); continue; }
            bytes += (await response.body()).length;
          }
          if (bytes>500000) report(file,'weight',`${bytes} initial bytes exceeds 500000`);
          console.log(`landing width=${width} motion=${reducedMotion} bytes=${bytes}`);
        }
        console.log(`measured page=${file} width=${width} motion=${reducedMotion} contrast_pairs=${measured.pairs}`);
        // Exercise shared controls after initial-load accounting on both surfaces.
        if (file === 'index.html' || file === 'user-guide.html') {
          for (const selector of ['.sidebar-toggle', '#search-open']) {
            const control = page.locator(selector);
            if (!await control.count() || !await control.isVisible()) continue;
            await control.click();
            await page.waitForLoadState('networkidle');
            const active = await page.evaluate(measure, { palette, width, reducedMotion, initial: false });
            for (const [kind, detail] of active.errors) report(file,kind,`${width}px/${reducedMotion}/${selector}: ${detail}`);
            if (selector === '#search-open') await page.keyboard.press('Escape');
          }
        }
        await context.close();
      }
    }
  }
  console.log(`browser-check pages=${pages.length} findings=${findings.length}`);
  process.exitCode = findings.length ? 1 : 0;
} catch (error) {
  console.error(`browser-check: ${error.message}`);
  process.exitCode = /usage:|contains no HTML|ENOENT|cannot parse/.test(error.message) ? 2 : 1;
} finally {
  if (browser) await browser.close();
  if (server) await new Promise(resolve => server.close(resolve));
}
