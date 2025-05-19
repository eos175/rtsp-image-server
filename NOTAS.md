
https://medium.com/@kurtesy_/opencv-for-go-is-a-lot-tricky-9c58464a9127


```go


// Serve MJPEG stream
func serveMJPEG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// para q la muestre de inmediato
	for i := 0; i < 3; i++ {
		frame, _ := currentFrame.Load().([]byte)
		if len(frame) > 0 {
			_, _ = w.Write([]byte("--frame\r\nContent-Type: image/jpeg\r\n\r\n"))
			_, _ = w.Write(frame)
			_, _ = w.Write([]byte("\r\n"))
			flusher.Flush()
		}
	}

	ticker := time.NewTicker(5000 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ticker.C:
			frame, _ := currentFrame.Load().([]byte)
			if len(frame) == 0 {
				continue
			}
			_, _ = w.Write([]byte("--frame\r\nContent-Type: image/jpeg\r\n\r\n"))
			_, _ = w.Write(frame)
			_, _ = w.Write([]byte("\r\n"))
			flusher.Flush()
		case <-ctx.Done():
			log.Info().Msg("MJPEG stream closed by client")
			return
		}
	}
}


```