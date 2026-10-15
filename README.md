# go-trans

Generate original-language video subtitles with Volcengine Recording Recognition **2.0**, then embed a default subtitle track for IINA. Audio and video are copied without re-encoding.

## Install

Requires Go 1.26+, FFmpeg and cloudflared.

```sh
brew install go ffmpeg cloudflared
go install github.com/imeelinew/go-trans@latest
go-trans config init
```

Set `api_key` in `~/.config/go-trans/config.conf`, or export `GO_TRANS_API_KEY`. Enable Recording Recognition 2.0 for that key.

## Use

```sh
go-trans generate ./video.mkv
go-trans generate "/absolute/path/video.mp4"
go-trans remove ./video.mkv

# Keep the source file:
go-trans generate ./video.mp4 --output ./subtitled.mp4
```

By default, commands replace the input atomically after verification. `generate` replaces existing subtitle tracks; `remove` removes all embedded subtitle tracks. Burned-in text cannot be removed.

MKV uses SubRip; MP4/MOV/M4V use mov_text. IINA receives a default track named `go-trans`. Player preferences can override automatic selection.

Language tags in the first audio track are used when available; otherwise the service detects the language. No translation is performed. Set `language` in the config to override detection. Whispered ASMR can produce missing or incorrect words.

## Config

`config.conf` is TOML. See [config.example.conf](config.example.conf) for recognition options, polling, chunk size and subtitle layout. `--config PATH` selects another file; `XDG_CONFIG_HOME` changes the default directory.

The standard API requires a public audio URL. By default, cloudflared temporarily serves generated audio chunks through a random URL; it closes when the command exits. Only audio chunks are exposed. A Cloudflare DNS fallback handles missing local IPv4 records. Set `tunnel_dns_url = ""` to disable it.

For your own storage, set `upload_put_url` and `upload_get_url`. Both accept `{id}` for a unique object name. URLs must remain valid throughout recognition; storage retention is your responsibility.

## Completion

Native file completion works for relative and absolute paths. Optional zsh command completion:

```sh
source <(go-trans completion zsh)
```

Bash and fish completion are also available through `go-trans completion`.

## Test

```sh
go test -race ./...
go vet ./...
```

Tests cover ASR submission/polling, cancellation, subtitle timing, isolated audio serving, and lossless MKV/MP4 embed/remove. Live tunnel checks are opt-in: `GO_TRANS_NETWORK_TEST=1 go test -run TestLiveTunnel`.

[Volcengine API](https://docs.volcengine.com/docs/DoubaoVoice/task-submission-http-1?lang=zh)
