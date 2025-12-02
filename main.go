package main

import (
	"cmp"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"gocv.io/x/gocv"
)

func Clamp[T cmp.Ordered](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Shared frame
var currentFrame atomic.Pointer[gocv.Mat]
var streamURL string
var imgQuality int

var matPool = sync.Pool{
	New: func() any {
		m := gocv.NewMat()
		return &m
	},
}

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Flags
	var addr string
	pflag.StringVar(&streamURL, "url", "rtsp://rtsp.jeosgram.io:8554/video/camera", "RTSP stream URL")
	pflag.StringVar(&addr, "addr", ":8080", "HTTP server address")
	pflag.IntVar(&imgQuality, "quality", 90, "Image encoding quality (1-100)")
	pflag.Parse()

	imgQuality = Clamp(imgQuality, 1, 100)

	log.Info().Int("effective_img_quality", imgQuality).Msg("Using Img quality for encoding.")

	// Start RTSP reader
	go captureFrames(streamURL)

	// Setup HTTP server
	http.HandleFunc("/image.jpg", serveJPEG)
	http.HandleFunc("/image.webp", serveWebP)

	log.Info().Str("address", addr).Msg("HTTP server listening")
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal().Err(err).Msg("HTTP server failed")
	}
}

// Continuously captures frames and stores latest JPEG
func captureFrames(streamURL string) {
	for {
		webcam, err := openCameraWithRetry(streamURL)
		if err != nil {
			log.Fatal().Err(err).Msg("Could not initialize camera")
		}
		processCameraStream(webcam)
	}
}

func openCameraWithRetry(url string) (*gocv.VideoCapture, error) {
	return retry.DoWithData(
		func() (*gocv.VideoCapture, error) {
			log.Info().Str("url", url).Msg("Attempting to open RTSP stream")
			return gocv.VideoCaptureFile(url)
		},
		retry.Attempts(0),
		retry.Delay(3*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) { // Logging en cada intento
			log.Warn().Err(err).Uint("attempt", n).Msg("retry connection")
		}),
	)
}

func processCameraStream(webcam *gocv.VideoCapture) {
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	for {
		if ok := webcam.Read(&img); !ok || img.Empty() {
			log.Warn().Msg("Failed to read frame — attempting to reconnect...")
			return
		}

		// Obtener un Mat reciclado
		clonedPtr := matPool.Get().(*gocv.Mat)

		img.CopyTo(clonedPtr)

		// Limpiar el frame anterior y guardar el nuevo
		if prev := currentFrame.Swap(clonedPtr); prev != nil {
			matPool.Put(prev) // En vez de Close, lo mandamos al Pool
		}
	}
}

// Serve latest JPEG snapshot
func serveJPEG(w http.ResponseWriter, r *http.Request) {
	framePtr := currentFrame.Load()
	if framePtr == nil || framePtr.Empty() {
		http.Error(w, "No frame available", http.StatusServiceUnavailable)
		return
	}

	// Clone frame to minimize race condition window during encoding
	local := matPool.Get().(*gocv.Mat)
	defer matPool.Put(local)

	framePtr.CopyTo(local)

	params := []int{gocv.IMWriteJpegQuality, imgQuality}
	buf, err := gocv.IMEncodeWithParams(".jpg", *local, params)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode JPEG")
		http.Error(w, "Failed to encode frame", http.StatusInternalServerError)
		return
	}
	defer buf.Close()

	now := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("snapshot-%s.jpg", now)

	w.Header().Set("Content-Type", "image/jpeg")

	log.Info().
		Str("filename", filename).
		Str("client", r.RemoteAddr).
		Str("url", streamURL).
		Msg("Snapshot downloaded")

	// Si hay ?download=1, forzar descarga
	if shouldDownload, _ := strconv.ParseBool(r.URL.Query().Get("download")); shouldDownload {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	}

	_, _ = w.Write(buf.GetBytes())
}

func serveWebP(w http.ResponseWriter, r *http.Request) {
	framePtr := currentFrame.Load()
	if framePtr == nil || framePtr.Empty() {
		http.Error(w, "No frame available", http.StatusServiceUnavailable)
		return
	}

	// Clone frame to minimize race condition window during encoding
	local := matPool.Get().(*gocv.Mat)
	defer matPool.Put(local)

	framePtr.CopyTo(local)

	params := []int{gocv.IMWriteWebpQuality, imgQuality}
	buf, err := gocv.IMEncodeWithParams(".webp", *local, params)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode WebP")
		http.Error(w, "Failed to encode frame", http.StatusInternalServerError)
		return
	}
	defer buf.Close()

	now := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("snapshot-%s.webp", now)

	w.Header().Set("Content-Type", "image/webp")

	log.Info().
		Str("filename", filename).
		Str("client", r.RemoteAddr).
		Str("url", streamURL).
		Msg("Snapshot downloaded")

	if shouldDownload, _ := strconv.ParseBool(r.URL.Query().Get("download")); shouldDownload {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	}

	_, _ = w.Write(buf.GetBytes())
}
