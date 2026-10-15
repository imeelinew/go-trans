# go-trans

Generate, embed, and remove video subtitles. Go, FFmpeg, and Volcengine Recording Recognition 2.0.

## Install

Requires Go 1.26+.

```sh
brew install go ffmpeg cloudflared
go install github.com/imeelinew/go-trans@latest
```

## Configure

```sh
go-trans config init
```

Set `api_key` in `~/.config/go-trans/config.json` (or `$XDG_CONFIG_HOME/go-trans/config.json`):

```json
{
  "api_key": "your-key"
}
```

Enable Recording Recognition 2.0 for the key. `GO_TRANS_API_KEY` overrides the file; `--config PATH` selects a different file.

See [config.example.json](config.example.json) for all defaults. Options include language, punctuation, number formatting, speaker identification, sentence segmentation, polling, chunk size, and subtitle layout. Omitted options keep their defaults.

## Use

```sh
go-trans generate ./video.mkv
go-trans generate "/absolute/path/video.mp4"
go-trans remove ./video.mkv

# Write a separate video:
go-trans generate ./video.mp4 -o ./subtitled.mp4
```

Commands replace the input after successful verification. `-o` writes a new file in the same container format and refuses to overwrite an existing file. Audio and video are copied without re-encoding.

`generate` replaces existing subtitle tracks with one default track. `remove` clears all embedded subtitle tracks; it cannot remove burned-in text. Supported containers: MKV, MP4, MOV, and M4V.

The first audio track's language tag is used when available; otherwise the service detects the language. No translation is performed. Set `language` to override detection, such as `en-US`, `zh-CN`, or `ja-JP`. Long videos are processed in chunks.

The API needs a public audio URL. By default, a temporary Cloudflare tunnel serves only generated audio chunks and closes when the command exits. For your own storage, set `upload_put_url` and `upload_get_url`; both accept `{id}` for a unique object name. Keep URLs valid throughout recognition and manage retention in your storage.

## Completion

Relative and absolute video paths support native file completion. Optional zsh command completion:

```sh
source <(go-trans completion zsh)
```

Bash and fish are also supported through `go-trans completion`.

## Development

```sh
go test -race ./...
go vet ./...
go build -o go-trans .
```
