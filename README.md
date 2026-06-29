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

Pass a direct URL to pull from any source:

```sh
ohmywhisper pull https://example.com/my-model.bin
```

Files larger than 50 MB are downloaded in 4 parallel chunks automatically.

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
NAME                     SIZE       SOURCE                   DESCRIPTION
large-v3-turbo           1.62 GB    ggerganov/whisper.cpp    large v3 turbo multilingual
PhoWhisper-tiny          73 MB      PhoWhisper-tiny          (synced from extra hub)
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

## Extra hubs

Register additional HuggingFace repos as model sources:

```sh
ohmywhisper add https://huggingface.co/vinai/PhoWhisper-tiny
```

URLs are normalized automatically — `/tree/main` and `/blob/main` suffixes are stripped.

Sync the catalog so models from extra hubs appear in `search` and `pull`:

```sh
ohmywhisper sync
```

```
syncing PhoWhisper-tiny ...
synced 4 model(s) from 1 hub(s)
```

Remove a registered hub:

```sh
ohmywhisper unadd https://huggingface.co/vinai/PhoWhisper-tiny
```

After syncing, pull models by name from extra hubs:

```sh
ohmywhisper pull PhoWhisper-tiny
```

---

## Serve

```sh
ohmywhisper serve
```

With a preloaded model, custom port, and auth token:

```sh
ohmywhisper serve --model small --port 3199 --token secret
ohmywhisper serve -p 8080
```

Force CPU-only (disable GPU):

```sh
ohmywhisper serve --no-gpu
```

Select a specific GPU on multi-GPU systems:

```sh
ohmywhisper serve --gpu-device 1
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

### Streaming

Add `stream=true` to receive segments as Server-Sent Events while the audio is being processed:

```sh
curl -N http://localhost:3199/v1/audio/transcriptions \
  -F file=@audio.mp3 \
  -F model=small \
  -F stream=true
```

Each SSE event is a JSON object:

```
data: {"type":"segment","id":0,"start":0.00,"end":3.20,"text":" Hello, how are you?"}

data: {"type":"segment","id":1,"start":3.20,"end":6.50,"text":" I am doing well."}

data: {"type":"done","text":"Hello, how are you? I am doing well.","duration":6.5}
```

Works on both `/v1/audio/transcriptions` and `/v1/audio/translations`. Combine with `timestamp_granularities[]=word` to include word timestamps in each segment event.

### Pull a model remotely (SSE)

```sh
curl -N -X POST http://localhost:3199/api/pull \
  -H "Content-Type: application/json" \
  -d '{"name": "small"}'
```

```
data: {"type":"progress","label":"ggml-small.bin","written":10485760,"total":511606912}

data: {"type":"done","name":"small"}
```

### List loaded models

```sh
curl http://localhost:3199/v1/models
```

### List downloaded models

```sh
curl http://localhost:3199/api/tags
```

```json
{"models":[{"name":"small","size":511606912,"modified_at":"2024-01-15T10:00:00Z"}]}
```

### Prometheus metrics

```sh
curl http://localhost:3199/metrics
```

```
# HELP ohmywhisper_models_total Total downloaded models
# TYPE ohmywhisper_models_total gauge
ohmywhisper_models_total 3

# HELP ohmywhisper_models_loaded Currently loaded models
# TYPE ohmywhisper_models_loaded gauge
ohmywhisper_models_loaded 1

# HELP ohmywhisper_gpu_vram_used_bytes GPU VRAM used in bytes
# TYPE ohmywhisper_gpu_vram_used_bytes gauge
ohmywhisper_gpu_vram_used_bytes{name="NVIDIA GeForce RTX 3080"} 3.35544e+09

# HELP ohmywhisper_cpu_temp_celsius CPU temperature in Celsius
# TYPE ohmywhisper_cpu_temp_celsius gauge
ohmywhisper_cpu_temp_celsius{zone="thermal_zone0"} 45
```

Full metrics table:

| Metric | Type | Description |
|--------|------|-------------|
| `ohmywhisper_models_total` | gauge | Total downloaded models |
| `ohmywhisper_models_loaded` | gauge | Currently loaded models |
| `ohmywhisper_models_disk_bytes` | gauge | Total size of all downloaded models |
| `ohmywhisper_rss_bytes` | gauge | Process RAM usage |
| `ohmywhisper_cpu_usage_percent` | gauge | Process CPU usage |
| `ohmywhisper_cpu_threads` | gauge | CPU thread count |
| `ohmywhisper_cpu_temp_celsius{zone}` | gauge | CPU temperature per thermal zone |
| `ohmywhisper_gpu_count` | gauge | Number of GPUs detected |
| `ohmywhisper_gpu_vram_used_bytes{name}` | gauge | GPU VRAM used |
| `ohmywhisper_gpu_usage_percent{name}` | gauge | GPU utilization |
| `ohmywhisper_network_rx_bytes_per_sec` | gauge | Network receive speed |
| `ohmywhisper_network_tx_bytes_per_sec` | gauge | Network transmit speed |
| `ohmywhisper_disk_used_bytes` | gauge | Disk space used |
| `ohmywhisper_disk_free_bytes` | gauge | Disk space free |
| `ohmywhisper_requests_total` | counter | Total API requests received |
| `ohmywhisper_processing_speed_rtf` | gauge | Real-time factor (audio secs / proc secs) |

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
extra_hubs: []
server_url: http://localhost:3199
whisper_src: ""
gpu: true
gpu_device: 0
```

`extra_hubs` is populated by `ohmywhisper add` and cleared by `ohmywhisper unadd`. Run `ohmywhisper sync` after adding hubs to populate the local catalog at `~/.ohmywhisper/external_catalog.yml`.

---
## credit
Backend ohmywhisper use [Whisper.cpp](https://github.com/ggml-org/whisper.cpp)

## Going on

- .....