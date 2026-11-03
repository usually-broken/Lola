# Voice

Voice is three plugins — a mouth, an ear, and the mode that puts them together — and both
halves are pointed at endpoints you choose. Nothing is assumed about your machine.

## Speaking

A speech endpoint that answers the OpenAI shape: `POST /v1/audio/speech` with a model, the
text, and a voice name, returning audio.

Ollama serves this at `http://127.0.0.1:11434/v1/audio/speech` if you have a speech model
running there; a hosted service works just as well.

## Hearing

A transcription endpoint that answers `POST /v1/audio/transcriptions`: multipart, a `file`
field, a `model` field, and JSON back with the text in it.

The ear records, posts what it recorded, and answers with what was said. No model runs on
your machine to make this work.

## Turning it on

```yaml
plugins:
  - speak
  - listen
  - voice
```

`voice` is the mode: the control, the ball, and the line the model is given while the
microphone is open. Without `speak` and `listen` there is nothing for it to do, so mount all
three or none.

## Settings

Settings → Voice is where both endpoints live, together with the voice name, the
transcription model, and the colours of the ball. They're kept in `~/.lola/voice.json` and
read fresh for every sentence, so a change takes effect on the next one rather than the
next restart.

Until a transcription endpoint is set, voice mode has no ear, and says so rather than
failing quietly.
