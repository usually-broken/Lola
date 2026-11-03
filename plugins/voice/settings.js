(function () {
'use strict';

// The voice settings section's own behaviour, served by the plugin that supplies the
// section. It loads what is saved into the boxes and posts it back; nothing about voice
// settings lives in the page.

const box = (id) => document.getElementById(id);

async function load() {
  try {
    const s = await (await fetch('/api/voice/settings')).json();
    box('voiceEndpoint').value = s.endpoint || '';
    box('voiceModel').value = s.model || '';
    box('voiceName').value = s.voice || '';
    box('voiceListenUrl').value = s.listenUrl || '';
    box('voiceListenModel').value = s.listenModel || '';
    box('voiceOrbCore').value = s.orbCore || '';
    box('voiceOrbCloud').value = s.orbCloud || '';
    box('voiceOrbRim').value = s.orbRim || '';
  } catch {
    const note = box('voiceNote');
    if (note) note.textContent = 'could not read the settings';
  }
}

async function save() {
  const note = box('voiceNote');
  const button = box('voiceSave');
  button.disabled = true;

  const body = {
    endpoint: box('voiceEndpoint').value.trim(),
    model: box('voiceModel').value.trim(),
    voice: box('voiceName').value.trim(),
    listenUrl: box('voiceListenUrl').value.trim(),
    listenModel: box('voiceListenModel').value.trim(),
    orbCore: box('voiceOrbCore').value.trim(),
    orbCloud: box('voiceOrbCloud').value.trim(),
    orbRim: box('voiceOrbRim').value.trim(),
  };

  try {
    const res = await fetch('/api/voice/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (note) note.textContent = res.ok ? 'saved — takes effect on the next sentence' : 'could not save';
  } catch {
    if (note) note.textContent = 'could not save';
  }

  button.disabled = false;
}

// Sections are redrawn as things are switched, so this runs once per draw; check the
// boxes exist before wiring, and do not wire twice.
if (box('voiceSave') && !box('voiceSave').dataset.wired) {
  box('voiceSave').dataset.wired = '1';

  // The panel's own Save button saves this too. A section with a second button of its
  // own means two answers to "did it save?", which is the sort of thing nobody expects.
  box('voiceSave').style.display = 'none';
  document.addEventListener('lola:save', save);

  load();
}

})();
