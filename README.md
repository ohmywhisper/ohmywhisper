# ohmywhisper

Whisper as a service. Like Ollama, but for speech.

Drop a model, start a server, call an OpenAI-compatible API — done.

---

## Install

Download the binary from [releases](../../releases) or build from source:

```sh
make
```

Requires `ffmpeg` and a [whisper.cpp ggml model](https://huggingface.co/ggerganov/whisper.cpp).

---

## Run

```sh
ohmywhisper serve --model /path/to/ggml-small.bin
```

With auth token:

```sh
ohmywhisper serve --model /path/to/ggml-small.bin --port 3199 --token secret
```

---

## API

### Transcribe

```sh
curl http://localhost:3199/v1/audio/transcriptions \
  -F file=@audio.mp3 \
  -F language=en \
  -F response_format=verbose_json
```

### Translate to English

```sh
curl http://localhost:3199/v1/audio/translations \
  -F file=@audio.mp3
```

### List models

```sh
curl http://localhost:3199/v1/models
```

---

## Response formats

| format         | description                              |
|----------------|------------------------------------------|
| `json`         | `{"text": "..."}` (default)              |
| `text`         | plain transcript                         |
| `verbose_json` | segments, timestamps, language, duration |
| `segment`      | segment list only                        |
| `model`        | verbose with model metadata              |

Word-level timestamps:

```sh
curl http://localhost:3199/v1/audio/transcriptions \
  -F file=@audio.mp3 \
  -F "timestamp_granularities[]=word" \
  -F response_format=verbose_json
```

---

## Going on

- GPU support
- Streaming response
- Multiple models
- Web UI
