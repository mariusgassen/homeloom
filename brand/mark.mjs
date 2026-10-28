// The Homeloom mark on a 512 grid. c = {a: saffron, b: berry}.
// Over/under: where a thread passes under, it is cut by a mask along the other thread,
// so the gap is real transparency and the mark works on any background.
const W = 44, GAP = 84;
let uid = 0;
const line = (d, col, w = W, extra = '') => `<path d="${d}" fill="none" stroke="${col}" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round"${extra}/>`;
// Thread d, cut wherever it passes under the thread `top` near the given crossings.
const under = (d, col, top, crossings) => {
  const id = `m${uid++}`;
  const cuts = crossings.map(([x, y], i) =>
    `<clipPath id="${id}c${i}"><circle cx="${x}" cy="${y}" r="56"/></clipPath><g clip-path="url(#${id}c${i})">${line(top, '#000', GAP)}</g>`).join('');
  return `<mask id="${id}" maskUnits="userSpaceOnUse" x="0" y="0" width="512" height="512"><rect width="512" height="512" fill="#fff"/>${cuts}</mask>` +
    line(d, col, W, ` mask="url(#${id})"`);
};

export const marks = {
  // A: Gable. Two threads cross at the roof peak; their tails stick out like crossed gable boards.
  gable(c) {
    const s = 'M 328 116 L 136 276 L 136 404';
    const b = 'M 184 116 L 376 276 L 376 404';
    return under(s, c.a, b, [[256, 176]]) + line(b, c.b);
  },
};
