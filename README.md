# RTSP Image Server

HTTP server that captures images from an RTSP stream and serves them as `.jpg` and `.webp` files.

## Usage

```bash
./stream-server --url rtsp://rtsp.jeosgram.io:8554/video/camera --addr :8080 --quality 90
```

* `--url`: RTSP stream URL (default: `rtsp://rtsp.jeosgram.io:8554/video/camera`)
* `--addr`: HTTP server address (default: `:8080`)
* `--quality`: Quality encode image (default: 90)

## Endpoints

* `/image.jpg` → Current frame as JPEG
* `/image.webp` → Current frame as WebP

## Docker

```bash
docker build -t stream-server .
docker run -p 8080:8080 stream-server
```

Export image:

```bash
docker save stream-server:latest | gzip > stream-server.tar.gz
```
