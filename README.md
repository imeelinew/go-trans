# go-trans

Generate and embed original-language subtitles for IINA using Volcengine Recording Recognition 2.0. No audio/video re-encoding.

```sh
brew install go ffmpeg cloudflared
go install github.com/imeelinew/go-trans@latest
go-trans config init
```

Set `api_key` in `~/.config/go-trans/config.conf`. Options: [config.example.conf](config.example.conf).

```sh
go-trans generate ./video.mkv
go-trans generate "/absolute/path/video.mp4"
go-trans remove ./video.mkv
```

Edits videos in place. Add `-o output.mkv` to keep the source. Generation replaces existing subtitle tracks; removal clears embedded subtitle tracks.
