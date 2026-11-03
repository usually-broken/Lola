# Voice

Speech in and speech out are plugins, and they need things that aren't part of the binary.

## Speech out

A TTS endpoint that speaks the OpenAI shape: `POST /v1/audio/speech` with a model, an input
string and a voice name, answering with audio.

Ollama serves this on `http://127.0.0.1:11434/v1/audio/speech`, and Phonon is a reasonable
default. Any endpoint that answers the same way will do, including a proprietary one — the
URL, model and voice are settings.

## Speech in

The ear records, then asks something to turn audio into text. Two ways to give it one:

**A transcription endpoint.** `POST /v1/audio/transcriptions`, multipart, the same shape
OpenAI uses. Nothing needs installing on the machine.

**A local worker.** On Apple silicon, Parakeet through MLX is quick and stays off the
network. The plugin starts one process and keeps it resident, because loading a recogniser
for every sentence costs seconds. Point `LOLA_LISTEN_PYTHON` and `LOLA_LISTEN_SCRIPT` at
your interpreter and script.

The second is what this was developed against; the first is what most machines will want.

## Turning it on

Both plugins are ordinary entries in the config. Add them when the machine can serve them:

```yaml
plugins:
  - speak
  - listen
  - voice
```

`voice` is the mode: the control, the ball, and the extra line the model is given while the
microphone is open. Without `speak` and `listen` there is nothing for it to do, so mount all
three or none.

## Settings

Endpoint, model, voice and the colours of the ball live in `~/.lola/voice.json`, and are
edited in Settings. They're read fresh for every sentence, so a change takes effect on the
next one rather than the next restart.
