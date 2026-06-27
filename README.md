# ohmywhisper

Whisper as a service. Like Ollama, but for speech.

Pull a model, start a server, call an OpenAI-compatible API — done.

---

## Install

Download the binary from [releases](../../releases) or build from source:

```sh
make
```

Requires `ffmpeg`.

---

## Quickstart

```sh
ohmywhisper pull small
ohmywhisper serve
```

---

## Models

### Pull a model

```sh
ohmywhisper pull small
ohmywhisper pull large-v3-turbo-q5_0
```

### List downloaded models

```sh
ohmywhisper ls
```

```
NAME                    SIZE       MODIFIED
small                   488.0 MB   2024-01-15 10:00:00
large-v3-turbo-q5_0    574.0 MB   2024-01-15 09:30:00
```

### Search available models

```sh
ohmywhisper search
ohmywhisper search large
```

```
NAME                     SIZE       DESCRIPTION
large-v3-turbo           1.62 GB    large v3 turbo multilingual
large-v3-turbo-q5_0      574 MB     large v3 turbo q5_0
large-v3-turbo-q8_0      874 MB     large v3 turbo q8_0
```

### Show model info

```sh
ohmywhisper show small
```

### Remove a model

```sh
ohmywhisper rm small
```

---

## Serve

```sh
ohmywhisper serve
```

With a preloaded model and auth token:

```sh
ohmywhisper serve --model small --port 3199 --token secret
```

### Load / unload at runtime

```sh
ohmywhisper start small
ohmywhisper start large-v3-turbo-q5_0
ohmywhisper stop small
```

### Show running models

```sh
ohmywhisper ps
```

```
NAME                    SINCE                PATH
small                   2024-01-15 10:30:00  /home/user/.ohmywhisper/models/ggml-small.bin
large-v3-turbo-q5_0     2024-01-15 10:31:00  /home/user/.ohmywhisper/models/ggml-large-v3-turbo-q5_0.bin

RAM:  1.18 GB
CPU:  2.3%
GPU:  NVIDIA GeForce RTX 3080  3%  VRAM 3.20 GB
```

---

## API

### Transcribe

```sh
curl http://localhost:3199/v1/audio/transcriptions \
  -F file=@audio.mp3 \
  -F model=small \
  -F language=en \
  -F response_format=verbose_json
```

### Translate to English

```sh
curl http://localhost:3199/v1/audio/translations \
  -F file=@audio.mp3 \
  -F model=large-v3-turbo-q5_0
```

### List loaded models

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

## Create a custom model

Write a `Modelfile`:

```
FROM small
LANGUAGE en
```

Then:

```sh
ohmywhisper create my-english -f Modelfile
ohmywhisper start my-english
```

---

## Convert a model

Convert a HuggingFace safetensors or PyTorch model to whisper.cpp format:

```sh
ohmywhisper cover /path/to/model-dir my-model
```

Requires `whisper_src` set in `~/.ohmywhisper/config.yml` pointing to a whisper.cpp clone.

---

## Config

`~/.ohmywhisper/config.yml` is created automatically with defaults:

```yaml
model_dir: ~/.ohmywhisper/models
hub: https://huggingface.co/ggerganov/whisper.cpp/resolve/main
server_url: http://localhost:3199
whisper_src: ""
```

---

## Going on

- Streaming response
