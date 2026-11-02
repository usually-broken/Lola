(function () {
'use strict';

// Voice mode: the ball, the microphone, and the two pauses -------------------------------------------------------------
//
// A ball of sky in the middle of the page. It swells while you talk and swells while
// she talks, because the same analyser measures both — one is what the microphone
// hears, the other what comes out of the speaker.
//
// The microphone is captured as raw samples rather than through MediaRecorder: the
// recogniser wants 16kHz WAV, and a browser codec would mean converting it somewhere
// else. Samples arrive at the sound card's rate, are averaged down to 16k, and are
// written into a WAV by hand.

// Filled by init(), once the markup is in the page. A script is loaded while its
// fragment is still being built, so nothing here may look anything up on the way past.
let orbBackdrop = null;
let orbHeard = null;
const orbHint = { textContent: '' };   // the stats line is gone; writes are now harmless

function init() {
  if (orbBackdrop) return;             // already wired

  orbBackdrop = document.getElementById('orbBackdrop');
  orbHeard = document.getElementById('orbHeard');
  if (!orbBackdrop) return;

  const exit = document.getElementById('orbExit');
  if (exit) exit.addEventListener('click', () => { if (voice.on) stopVoice(); });

  window.addEventListener('resize', () => { if (voice.on) placeOrb(); });

  // The chip is markup from a plugin too, so it is caught on the document rather than
  // wired when it is drawn.
  document.addEventListener('click', (e) => {
    const trigger = e.target.closest && e.target.closest('[data-voice-toggle]');
    if (trigger) toggleVoice();
  });
}

// The script is fetched while its fragment is still being built, so it can arrive
// either side of the markup: listen for the page's announcement, and also look for
// ourselves for a couple of seconds. A file that only works when the timing goes its
// way is a file that fails on a slow machine.
document.addEventListener('lola:fragment', init);
window.addEventListener('DOMContentLoaded', init);
init();

let attempts = 0;
const watcher = setInterval(() => {
  init();
  attempts += 1;
  if (orbBackdrop || attempts > 60) clearInterval(watcher);   // three seconds, then stop
}, 50);

const voice = {
  on: false,
  ctx: null,
  stream: null,
  source: null,
  proc: null,
  analyser: null,
  rate: 48000,
  samples: [],        // 16k samples of the utterance being built
  quiet: 0,           // seconds of silence since the last sound
  spoke: 0,           // seconds of speech in the sentence being built
  turnSpeech: 0,      // seconds of speech this turn, across sentences
  age: 0,             // seconds since the mic opened, for the startup click
  heard: '',          // text heard so far in this turn
  level: 0,
  sending: false,
  raf: null,
};

// Filled once playback is measured rather than guessed at; until then the ball
// answers to the microphone alone.
let speakingLevel = 0;

// Barge-in. The speaker leaks into the microphone, so the bar is deliberately far above
// ordinary speech and it has to stay there: a cough is not an interruption, a sentence is.
const BARGE_LEVEL = 0.42;
const BARGE_HOLD = 0.35;
let bargeFor = 0;

// Colours come from the voice settings; these are what an untouched harness gets.
voice.colours = { core: '#6ba6e8', cloud: '#ffffff', rim: 'rgba(255,255,255,0.55)' };
voice.palette = null;

async function loadColours() {
  // Applied immediately with the defaults, then again when the settings arrive.
  try {
    const s = await (await fetch('/api/voice/settings')).json();
    if (s.orbCore) voice.colours.core = s.orbCore;
    if (s.orbCloud) voice.colours.cloud = s.orbCloud;
    if (s.orbRim) voice.colours.rim = s.orbRim;
  } catch {
    // A ball in the default colours is not a failure worth showing anyone.
  }
}

// buildPalette turns three settings into the strings a canvas will accept.
function buildPalette(c) {
  // The centre is the core colour lightened, not the cloud colour: a ball whose middle
  // is white stops reading as a ball.
  return {
    pale: mix(c.core, c.cloud, 0.62),
    mid: mix(c.core, c.cloud, 0.22),
    core: c.core,
    cloudSoft: rgba(c.cloud, 0.42),
    cloudFaint: rgba(c.cloud, 0.18),
    cloudNone: rgba(c.cloud, 0),
    rim: c.rim,
  };
}

const VOICE_SENTENCE = 0.4;   // silence that ends a sentence
const VOICE_TURN = 1.0;       // silence that ends your turn
const VOICE_MIN = 0.35;       // ignore anything shorter than this
const VOICE_GRACE = 0.3;      // the mic opening is a noise, not a word



// hexToRgb reads #rgb or #rrggbb; anything else is taken as-is and used by the caller
// only where a colour is expected to be a plain CSS value.
function hexToRgb(hex) {
  const h = (hex || '').trim();
  if (!h.startsWith('#')) return null;
  // #rgb or #rrggbb only. Anything else is a typo, and a typo must fall back to the
  // default rather than quietly become black.
  if (h.length !== 4 && h.length !== 7) return null;
  const s = h.length === 4
    ? '#' + h[1] + h[1] + h[2] + h[2] + h[3] + h[3]
    : h;
  const n = parseInt(s.slice(1), 16);
  if (Number.isNaN(n)) return null;
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

function rgba(colour, alpha) {
  const c = hexToRgb(colour);
  if (!c) return colour;
  return 'rgba(' + c.r + ',' + c.g + ',' + c.b + ',' + alpha + ')';
}

function mix(a, b, amount) {
  const ca = hexToRgb(a), cb = hexToRgb(b);
  if (!ca || !cb) return a;
  const at = (x, y) => Math.round(x + (y - x) * amount);
  return 'rgb(' + at(ca.r, cb.r) + ',' + at(ca.g, cb.g) + ',' + at(ca.b, cb.b) + ')';
}

// The ball sits in the layout now, so there is nothing to place. Kept as a name
// because opening voice mode still calls it, and because a future layout with the ball
// somewhere else will want it again.
function placeOrb() {}



// Speaking belongs to the plugin. The page hands over text and forgets it; the plugin
// splits it into sentences, plays them one at a time, and knows what it is saying, which
// is the only way a cut can land in the right place.

const sayQueue = [];
let sayBuffer = '';
let saying = false;
let sayGen = 0;
let sayCurrent = '';

function trace(line) {
  fetch('/api/voice/trace', { method: 'POST', body: String(line) }).catch(() => {});
}

function sayMark(sentence, index) {
  sayCurrent = sentence;
  window.lolaSpeaking = true;
  fetch('/api/voice/spoken', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ speaking: true, sentence: sentence, index: index })
  }).catch(() => {});
}

function sayClearMark() {
  window.lolaSpeaking = false;
  fetch('/api/voice/spoken', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ speaking: false })
  }).catch(() => {});
}

// One finished sentence, found the way the harness finds them: a full stop, exclamation or
// question mark followed by a space. A dot inside a number is not one.
function sayCut(text) {
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c !== '.' && c !== '!' && c !== '?') continue;
    const next = text[i + 1];
    if (next && next !== ' ' && next !== '\n') continue;
    if (c === '.' && i > 0 && /[0-9]/.test(text[i - 1])) continue;
    return i + 1;
  }
  return -1;
}

function playThroughGraph(buffer) {
  return new Promise((resolve) => {
    const src = voice.ctx.createBufferSource();
    src.buffer = buffer;
    src.connect(voice.ctx.destination);
    voice.stopPlayback = () => {
      try { src.stop(); } catch (e) {}
      resolve();
    };
    src.onended = () => { resolve(); };
    src.start();
  });
}

async function sayNext() {
  const gen = sayGen;
  const next = sayQueue.shift();
  if (!next) { saying = false; sayClearMark(); return; }
  saying = true;
  sayMark(next, sayQueue.length + 1);

  try {
    const res = await fetch('/api/voice/speak', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: next })
    });
    if (!res.ok) throw new Error('speak said ' + res.status);
    const buffer = await voice.ctx.decodeAudioData(await res.arrayBuffer());
    if (gen !== sayGen) return;
    await playThroughGraph(buffer);
  } catch (err) {
    trace('say: ' + (err && err.message ? err.message : err));
  }
  if (gen !== sayGen) return;
  sayNext();
}

let splitting = false;

// Where a sentence ends is decided by the harness, not here: /api/voice/sentences is the
// same rule that builds the spoken marks, so what is played and what is recorded cannot
// drift apart. The local test only decides when asking is worth the round trip.
async function splitByHarness() {
  if (splitting || !sayBuffer.trim()) return;
  if (!/[.!?]\s/.test(sayBuffer) && sayBuffer.length < 120) return;
  splitting = true;
  const text = sayBuffer;
  try {
    const res = await fetch('/api/voice/sentences', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text: text })
    });
    if (!res.ok) throw new Error('sentences said ' + res.status);
    const body = await res.json();
    const pieces = (body.sentences || []).filter(p => p && p.trim());
    if (!pieces.length) return;
    const last = pieces[pieces.length - 1];
    const finished = /[.!?]\s*$/.test(last);
    const ready = finished ? pieces : pieces.slice(0, -1);
    if (ready.length) {
      for (const r of ready) sayQueue.push(r.trim());
      // Only drop what was actually taken, so a partial answer keeps its beginning.
      const taken = ready.join(' ');
      const at = sayBuffer.indexOf(taken);
      sayBuffer = at === 0 ? sayBuffer.slice(taken.length).trimStart() : sayBuffer;
      if (finished && at === 0) sayBuffer = '';
    }
    if (!saying) sayNext();
  } catch (err) {
    trace('split: ' + (err && err.message ? err.message : err));
  } finally {
    splitting = false;
  }
}

window.lolaSay = function (chunk) {
  if (!voice.ctx) return false;
  sayBuffer += chunk;
  splitByHarness();
  if (!saying && sayQueue.length) sayNext();
  return true;
};

window.lolaSayEnd = function () {
  const rest = sayBuffer.trim();
  sayBuffer = '';
  if (rest) sayQueue.push(rest);
  if (!saying) sayNext();
};

// The cut. Called the moment your words are sent: the sentence being said stops, and
// everything queued behind it is never spoken. The answer to what you said becomes the
// only thing left to play, because it arrives as a new message with a new generation.
window.lolaCutIn = function () {
  sayQueue.length = 0;
  sayBuffer = '';
  sayGen += 1;
  const sentence = sayCurrent;
  if (voice.stopPlayback) voice.stopPlayback();
  saying = false;
  sayClearMark();
  fetch('/api/voice/interrupt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sentence: sentence, at: new Date().toISOString() })
  }).catch(() => {});
  return true;
};

async function toggleVoice() {
  voice.on ? await stopVoice() : await startVoice();
}

async function startVoice() {
  try {
    voice.stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, channelCount: 1 },
    });
  } catch (err) {
    orbBackdrop.style.display = 'flex';
    orbHeard.textContent = 'no microphone: ' + err.message;
    voice.on = true;
    voice.mute = true;          // draws, but never listens
    loadColours();
    tickOrb();
    return;
  }

  voice.on = true;
  orbBackdrop.style.display = 'flex';
  placeOrb();
  loadFillers();
  loadColours();
  orbHeard.textContent = '';
  orbHint.textContent = 'listening';

  await fetch('/api/voice/mode', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ on: true }),
  });

  try {
  voice.ctx = new (window.AudioContext || window.webkitAudioContext)();
  if (voice.ctx.state === 'suspended') {
    voice.ctx.resume().catch(() => {});
  }
  voice.rate = voice.ctx.sampleRate;
  voice.source = voice.ctx.createMediaStreamSource(voice.stream);

  // An analyser for the level, and a processor for the samples. Both hang off the
  // same source: one to draw with, one to transcribe.
  voice.analyser = voice.ctx.createAnalyser();
  voice.analyser.fftSize = 1024;
  voice.source.connect(voice.analyser);

  voice.proc = voice.ctx.createScriptProcessor(4096, 1, 1);
  voice.proc.onaudioprocess = (e) => {
    const input = e.inputBuffer.getChannelData(0);
    const down = downsample(input, voice.rate, 16000);

    // Appended one at a time rather than spread: spreading a few thousand values into
    // push allocates an argument list every block, and this runs fifty times a second.
    for (let i = 0; i < down.length; i++) voice.samples.push(down[i]);

    // A hard ceiling. Without one, an open microphone in a quiet room grows this
    // array for as long as the tab is open — which is what ate the memory.
    const cap = 16000 * 20;
    if (voice.samples.length > cap) voice.samples.splice(0, voice.samples.length - cap);
  };
  voice.source.connect(voice.proc);
  voice.proc.connect(voice.ctx.destination);   // required for it to run; silent input

  tickOrb();
  } catch (err) {
    tickOrg('voice failed: ' + (err && err.message ? err.message : err));
  }
}

async function stopVoice() {
  voice.on = false;
  orbBackdrop.style.display = 'none';

  if (voice.proc) { voice.proc.onaudioprocess = null; voice.proc.disconnect(); }
  if (voice.source) voice.source.disconnect();
  if (voice.ctx) await voice.ctx.close();
  if (voice.stream) voice.stream.getTracks().forEach((t) => t.stop());

  voice.ctx = voice.stream = voice.source = voice.proc = voice.analyser = null;
  voice.samples.length = 0;
  voice.heard = '';
  voice.quiet = 0;
  voice.spoke = 0;
  voice.turnSpeech = 0;

  fetch('/api/voice/mode', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ on: false }),
  });

  if (voice.raf) cancelAnimationFrame(voice.raf);
  voice.raf = null;
}

// downsample averages groups of samples: crude, and completely adequate for speech.
function downsample(input, from, to) {
  if (from === to) return Array.from(input);
  const ratio = from / to;
  const out = [];
  for (let i = 0; i < input.length; i += ratio) {
    let sum = 0, n = 0;
    for (let j = 0; j < ratio && i + j < input.length; j++) { sum += input[i + j]; n++; }
    out.push(sum / n);
  }
  return out;
}

// toWav writes 16-bit mono PCM, which is what the recogniser reads.
function toWav(samples, rate) {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  const write = (offset, str) => { for (let i = 0; i < str.length; i++) view.setUint8(offset + i, str.charCodeAt(i)); };

  write(0, 'RIFF');
  view.setUint32(4, 36 + samples.length * 2, true);
  write(8, 'WAVE');
  write(12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, rate, true);
  view.setUint32(28, rate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  write(36, 'data');
  view.setUint32(40, samples.length * 2, true);

  let offset = 44;
  for (const s of samples) {
    const clamped = Math.max(-1, Math.min(1, s));
    view.setInt16(offset, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true);
    offset += 2;
  }
  return new Blob([view], { type: 'audio/wav' });
}

// tickOrb draws the ball and watches the level for the two pauses that matter.
function tickOrg(text) {
  if (orbHeard) orbHeard.textContent = text;
}

function paintOrb(draw, w, h, angle) {
  const cx = w / 2, cy = h / 2;
  const r = Math.min(w, h) * (0.40 + voice.level * 0.07);

  draw.clearRect(0, 0, w, h);

  const c = voice.colours;
  const core = hexToRgb(c.core) || { r: 107, g: 166, b: 232 };
  const cloud = hexToRgb(c.cloud) || { r: 255, g: 255, b: 255 };
  const lift = (a, b, amount) => ({
    r: Math.round(a.r + (b.r - a.r) * amount),
    g: Math.round(a.g + (b.g - a.g) * amount),
    b: Math.round(a.b + (b.b - a.b) * amount),
  });
  const rgb = (x, a) => 'rgba(' + x.r + ',' + x.g + ',' + x.b + ',' + a + ')';

  draw.save();
  draw.beginPath();
  draw.arc(cx, cy, r, 0, Math.PI * 2);
  draw.clip();

  const body = draw.createRadialGradient(cx - r * 0.4, cy - r * 0.45, r * 0.1, cx, cy, r * 1.05);
  body.addColorStop(0, rgb(lift(core, cloud, 0.62), 1));
  body.addColorStop(0.45, rgb(lift(core, cloud, 0.22), 1));
  body.addColorStop(1, rgb(core, 1));
  draw.fillStyle = body;
  draw.fillRect(cx - r, cy - r, r * 2, r * 2);

  // Mist: many small, soft blobs, each drifting at its own speed. A few big ones read as
  // eggs; many small ones read as weather.
  for (let i = 0; i < 14; i++) {
    const phase = angle * (0.5 + i * 0.09) + i * 1.31;
    const spread = r * (0.75 - i * 0.02);
    const mx = cx + Math.sin(phase) * spread;
    const my = cy + Math.cos(phase * 0.75 + i) * spread * 0.85;
    const size = r * (0.26 + (i % 4) * 0.05);

    const puff = draw.createRadialGradient(mx, my, 1, mx, my, size);
    puff.addColorStop(0, rgb(cloud, 0.34));
    puff.addColorStop(0.55, rgb(cloud, 0.14));
    puff.addColorStop(1, rgb(cloud, 0));
    draw.fillStyle = puff;
    draw.beginPath();
    draw.ellipse(mx, my, size, size * (0.6 + (i % 3) * 0.12), phase * 0.5, 0, Math.PI * 2);
    draw.fill();
  }

  draw.restore();

  draw.strokeStyle = c.rim || 'rgba(255,255,255,0.55)';
  draw.lineWidth = 2;
  draw.beginPath();
  draw.arc(cx, cy, r, 0, Math.PI * 2);
  draw.stroke();
}

function tickOrb() {
  const canvas = document.getElementById('orbCanvas');
  if (!canvas) return;
  const draw = canvas.getContext('2d');
  const w = canvas.width, h = canvas.height;

  let angle = 0;
  let last = performance.now();

  const frame = (now) => {
    if (!voice.on) return;

    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;

    let level = 0;
    if (!voice.mute && voice.analyser) {
      const data = new Uint8Array(voice.analyser.frequencyBinCount);
      voice.analyser.getByteTimeDomainData(data);
      let peak = 0;
      for (const v of data) peak = Math.max(peak, Math.abs(v - 128) / 128);
      level = peak;
    }
    if (speakingLevel > level) level = speakingLevel;
    voice.level += (level - voice.level) * 0.25;

    // If the harness is talking and the microphone stays loud, someone is talking over
    // it. The mark says which sentence, so the interruption knows what it cut into.
    if (window.lolaSpeaking && window.lolaInterrupt && voice.level > BARGE_LEVEL) {
      bargeFor += dt;
      if (bargeFor > BARGE_HOLD) { bargeFor = 0; window.lolaInterrupt(); }
    } else {
      bargeFor = 0;
    }

    voice.age += dt;
    const loud = voice.age > VOICE_GRACE && voice.level > 0.03;
    if (loud) { voice.quiet = 0; voice.spoke += dt; voice.turnSpeech += dt; }
    else { voice.quiet += dt; }

    // A preview is the whole recording transcribed again, so its cost grows with how
    // long you have been talking. Twice a second was making long turns slow; once every
    // two and a half seconds, and none at all past fifteen seconds, keeps the words
    // appearing without spending the machine on them.
    const previewEvery = 2.5;
    const previewUpTo = 15;
    if (!voice.sending && voice.turnSpeech > 0.4
        && voice.age - (voice.lastPreview || 0) > previewEvery
        && voice.samples.length > 16000 * 0.6
        && voice.samples.length < 16000 * previewUpTo
        && voice.quiet < 0.05) {
      voice.lastPreview = voice.age;
      previewTurn();
    }
    if (voice.quiet > VOICE_TURN && voice.turnSpeech > 0.3
        && voice.samples.length > 16000 * VOICE_MIN) {
      finaliseTurn();
    }

    angle += dt * 0.5;
    try {
      paintOrb(draw, w, h, angle);
    } catch (err) {
      voice.on = false;
      if (orbHeard) orbHeard.textContent = 'drawing failed: ' + err.message;
      return;
    }

    voice.raf = requestAnimationFrame(frame);
  };

  voice.raf = requestAnimationFrame(frame);
}

// Fillers are the small noises a person makes while thinking, made once when voice
// mode opens so the wait before an answer sounds like thought rather than loading.
const FILLERS = ['Mm.', 'Uh huh.', 'Right.', 'So, um.', 'Okay.', 'Well.'];
voice.fillers = [];

async function loadFillers() {
  if (voice.fillers && voice.fillers.length) return;

  const made = await Promise.all(FILLERS.map(async (line) => {
    try {
      const res = await fetch('/api/voice/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: line }),
      });
      if (!res.ok) return null;
      return URL.createObjectURL(await res.blob());
    } catch {
      return null;
    }
  }));

  voice.fillers = made.filter(Boolean);
}

function playFiller() {
  if (!voice.fillers || !voice.fillers.length) return;
  const choice = voice.fillers[Math.floor(Math.random() * voice.fillers.length)];
  const audio = new Audio(choice);
  audio.volume = 0.85;
  audio.play().catch(() => {});
}

// transcribeAll sends the whole turn's audio and gives back the words for it.
async function transcribeAll() {
  const wav = toWav(voice.samples, 16000);
  const res = await fetch('/api/voice/listen', {
    method: 'POST',
    headers: { 'Content-Type': 'audio/wav' },
    body: wav,
  });
  const data = await res.json();
  return (data.text || '').trim();
}

// previewTurn shows what has been heard so far, without committing to it.
async function previewTurn() {
  if (voice.sending) return;
  voice.sending = true;
  try {
    const text = await transcribeAll();
    if (text) {
      voice.heard = text;
      const field = document.querySelector('[data-composer] textarea');
      if (field) {
        field.value = text;
        field.dispatchEvent(new Event('input', { bubbles: true }));
      }
      orbHeard.textContent = text.slice(-90);
    }
  } catch {
    // A failed preview is not a failed turn: the recording is still there.
  }
  voice.sending = false;
}

// finaliseTurn is what actually sends: one recording, one message, no cuts.
async function finaliseTurn() {
  if (voice.sending) return;
  voice.sending = true;

  try {
    const text = await transcribeAll();
    if (text) voice.heard = text;
  } catch {
    // Fall through to whatever the last preview heard.
  }

  voice.sending = false;
  if (voice.heard) sendTurn();
}

// sendTurn hands the finished prompt to the harness, the same way typing would.
function sendTurn() {
  const field = document.querySelector('[data-composer] textarea');
  if (!field || !voice.heard) return;

  const text = voice.heard;
  voice.heard = '';
  voice.samples.length = 0;
  voice.quiet = 0;
  voice.spoke = 0;
  voice.turnSpeech = 0;
  orbHeard.textContent = '';
  orbHint.textContent = 'thinking';

  field.value = text;
  field.dispatchEvent(new Event('input', { bubbles: true }));

  playFiller();

  const form = field.closest('form') || document.querySelector('[data-composer]');
  const send = form && form.querySelector('button[type="submit"]');
  if (send) send.click();
  else field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
}


})();
