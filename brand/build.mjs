// Builds the Homeloom icon set from the mark in marks.mjs: SVG sources, PNGs and favicon.ico.
// Usage, from brand/: node build.mjs   (needs playwright and ImageMagick)
import { marks } from './mark.mjs';
import { createRequire } from 'module';
import { execFileSync } from 'child_process';
import fs from 'fs';
import path from 'path';
const { chromium } = createRequire(import.meta.url)('playwright');

const out = '.';
const mark = marks.gable;
const SPRUCE = '#1f4d47';
const onTile = { a: '#f0b93d', b: '#e5809f' };   // dark-theme threads, bright enough on spruce
const onLight = { a: '#b57c06', b: '#b63d64' };  // light-theme threads, for light surfaces

const svg = (body) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">${body}</svg>\n`;
// scale < 1 shrinks the mark around the centre, for the maskable safe zone.
const scaled = (s, c) => s === 1 ? mark(c) : `<g transform="translate(256 256) scale(${s}) translate(-256 -256)">${mark(c)}</g>`;
const rounded = (s = 1) => svg(`<rect width="512" height="512" rx="112" fill="${SPRUCE}"/>${scaled(s, onTile)}`);
const square = (s = 1) => svg(`<rect width="512" height="512" fill="${SPRUCE}"/>${scaled(s, onTile)}`);

const files = {
  'homeloom-icon.svg': rounded(),                 // app icon with rounded corners, general use
  'homeloom-mark.svg': svg(mark(onLight)),        // threads only, for light backgrounds
  'homeloom-mark-dark.svg': svg(mark(onTile)),    // threads only, for dark backgrounds
  'favicon.svg': rounded(),
};
fs.mkdirSync(out, { recursive: true });
for (const [name, body] of Object.entries(files)) fs.writeFileSync(path.join(out, name), body);

// PNGs: [file, svg, size, transparent corners?]
const pngs = [
  ['apple-touch-icon.png', square(), 180, false],  // iOS rounds it itself; must be opaque
  ['icon-192.png', rounded(), 192, true],
  ['icon-512.png', rounded(), 512, true],
  ['icon-maskable-192.png', square(0.8), 192, false],
  ['icon-maskable-512.png', square(0.8), 512, false],
  ['authentik-icon.png', rounded(), 256, true],
  ['favicon-16.png', rounded(), 16, true],
  ['favicon-32.png', rounded(), 32, true],
  ['favicon-48.png', rounded(), 48, true],
];
const browser = await chromium.launch();
const page = await browser.newPage();
for (const [name, body, size, transparent] of pngs) {
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(`<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${body}`);
  await page.screenshot({ path: path.join(out, name), omitBackground: transparent });
}
await browser.close();
// Opaque icons must carry no alpha channel at all (iOS turns transparency black).
for (const [name, , , transparent] of pngs) if (!transparent) execFileSync('convert', [path.join(out, name), '-alpha', 'off', path.join(out, name)]);

const ico = ['favicon-16.png', 'favicon-32.png', 'favicon-48.png'].map((f) => path.join(out, f));
execFileSync('convert', [...ico, path.join(out, 'favicon.ico')]);
for (const f of ico) fs.unlinkSync(f);
