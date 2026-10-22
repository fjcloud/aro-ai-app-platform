package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed web/*
var assets embed.FS

const maxAudio = 2 << 20

type server struct {
	endpoint string
	token    string
	client   *http.Client
	slots    chan struct{}
}

func newServer(endpoint, token string) (*server, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("WHISPER_URL must be an HTTP(S) URL without credentials, query or fragment")
	}
	return &server{strings.TrimRight(endpoint, "/"), token, &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, make(chan struct{}, 2)}, nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/transcribe", s.transcribe)
	web, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "microphone=(self)")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; worker-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}

// Accept independent, short PCM WAV clips. Browser WebM recorder slices are not
// independent files and are deliberately not forwarded to the inference server.
func validWAV(data []byte) bool {
	if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || int(binary.LittleEndian.Uint32(data[4:8])) != len(data)-8 {
		return false
	}
	format, audio := false, false
	for at := 12; at+8 <= len(data); {
		n := int(binary.LittleEndian.Uint32(data[at+4 : at+8]))
		start := at + 8
		if n > len(data)-start {
			return false
		}
		switch string(data[at : at+4]) {
		case "fmt ":
			if n < 16 {
				return false
			}
			b := data[start : start+n]
			format = binary.LittleEndian.Uint16(b) == 1 && binary.LittleEndian.Uint16(b[2:]) == 1 && binary.LittleEndian.Uint32(b[4:]) == 16000 && binary.LittleEndian.Uint32(b[8:]) == 32000 && binary.LittleEndian.Uint16(b[12:]) == 2 && binary.LittleEndian.Uint16(b[14:]) == 16
			if !format {
				return false
			}
		case "data":
			if audio || n < 3200 || n > 320000 || n%2 != 0 {
				return false
			}
			audio = true
		}
		at = start + n + n%2
	}
	return format && audio
}

func (s *server) transcribe(w http.ResponseWriter, r *http.Request) {
	// Browser requests stay on this origin. Reject cross-origin form submissions.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			fail(w, 403, "Cross-origin audio requests are not allowed")
			return
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		fail(w, 503, "Transcription is busy. Try again shortly.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudio+65536)
	if err := r.ParseMultipartForm(maxAudio + 65536); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, 413, "Audio clip is too large")
		} else {
			fail(w, 400, "Send a multipart WAV file")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	language := r.FormValue("language")
	if language != "" && language != "fr" && language != "en" {
		fail(w, 400, "Choose English, French or automatic language detection")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "Audio file is required")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, maxAudio+1))
	if err != nil || len(audio) > maxAudio || !validWAV(audio) {
		fail(w, 400, "Use a 0.1–10 second mono 16 kHz PCM16 WAV clip")
		return
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "speech.wav")
	_, _ = part.Write(audio)
	_ = form.WriteField("model", "whisper")
	_ = form.WriteField("response_format", "json")
	_ = form.WriteField("temperature", "0")
	if language != "" {
		_ = form.WriteField("language", language)
	}
	_ = form.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/audio/transcriptions", &body)
	if err != nil {
		fail(w, 502, "Speech service is unavailable")
		return
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	started := time.Now()
	response, err := s.client.Do(req)
	if err != nil {
		fail(w, 502, "Speech service is unavailable. Check the Whisper deployment.")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		fail(w, 502, "Speech service rejected the audio. Check the Whisper deployment.")
		return
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result) != nil {
		fail(w, 502, "Speech service returned an invalid response")
		return
	}
	reply(w, 200, map[string]any{"text": strings.TrimSpace(result.Text), "latency_ms": time.Since(started).Milliseconds()})
}

func main() {
	endpoint := os.Getenv("WHISPER_URL")
	if endpoint == "" {
		endpoint = "http://whisper-predictor.speech-serving.svc.cluster.local/v1"
	}
	app, err := newServer(endpoint, os.Getenv("WHISPER_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":8080", Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	log.Print("Live Transcript listening on :8080")
	log.Fatal(server.ListenAndServe())
}
