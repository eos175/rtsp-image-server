package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"gocv.io/x/gocv"
)

// Shared frame
var currentFrame atomic.Pointer[gocv.Mat]
var streamURL string

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
	pflag.Parse()

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
		retry.Delay(2*time.Second),
		retry.Attempts(10),
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

		// Reutilizar memoria
		if clonedPtr.Empty() || clonedPtr.Cols() != img.Cols() || clonedPtr.Rows() != img.Rows() {
			clonedPtr.Close()
			*clonedPtr = gocv.NewMatWithSize(img.Rows(), img.Cols(), img.Type())
		}

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

	params := []int{gocv.IMWriteJpegQuality, 90}
	buf, err := gocv.IMEncodeWithParams(".jpg", *framePtr, params)
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
	if r.URL.Query().Get("download") == "1" {
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

	params := []int{gocv.IMWriteWebpQuality, 90}
	buf, err := gocv.IMEncodeWithParams(".webp", *framePtr, params)
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

	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	}

	_, _ = w.Write(buf.GetBytes())
}

// esto es muy lento para esenarios de la vida real
func captureSnapshot(url string) ([]byte, error) {
	cmd := exec.Command("ffmpeg",
		"-rtsp_transport", "tcp", // Mejor para RTSP
		"-i", url,
		"-frames:v", "1", // Solo un frame
		"-f", "image2",
		"-q:v", "2", // Calidad JPG (1-31, 1=mejor)
		"pipe:1", // Salida en stdout
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard

	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
