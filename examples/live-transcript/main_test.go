package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func clip() []byte {
	b := make([]byte, 44+32000)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 32000)
	return b
}
func upload(audio []byte, language string) *http.Request {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, _ := w.CreateFormFile("file", "voice.wav")
	f.Write(audio)
	w.WriteField("language", language)
	w.Close()
	r := httptest.NewRequest("POST", "http://app/api/transcribe", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}
func TestForwardAudio(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer server-only" {
			t.Error("wrong upstream target or auth")
		}
		if err := r.ParseMultipartForm(maxAudio); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("model") != "whisper" || r.FormValue("language") != "fr" {
			t.Error("wrong model or language")
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if !bytes.Equal(b, clip()) {
			t.Error("audio changed")
		}
		io.WriteString(w, `{"text":"Bonjour OpenShift."}`)
	}))
	defer upstream.Close()
	s, _ := newServer(upstream.URL+"/v1", "server-only")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, upload(clip(), "fr"))
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result["text"] != "Bonjour OpenShift." {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "server-only") {
		t.Fatal("key leaked")
	}
}
func TestInvalidAudioAndOrigin(t *testing.T) {
	s, _ := newServer("http://unreachable.invalid/v1", "")
	badRate := clip()
	binary.LittleEndian.PutUint32(badRate[24:], 44100)
	for _, tc := range []struct {
		name string
		req  *http.Request
		code int
	}{
		{"not-wav", upload([]byte("hello"), "fr"), 400}, {"rate", upload(badRate, "fr"), 400},
		{"language", upload(clip(), "../en"), 400}, {"oversized", upload(make([]byte, maxAudio+65537), "fr"), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.routes().ServeHTTP(w, tc.req)
			if w.Code != tc.code {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	r := upload(clip(), "en")
	r.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestUpstreamFailureAndTimeout(t *testing.T) {
	for _, mode := range []string{"failure", "invalid", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "timeout" {
					time.Sleep(30 * time.Millisecond)
				}
				if mode == "failure" {
					w.WriteHeader(401)
				}
				io.WriteString(w, "private upstream error")
			}))
			defer upstream.Close()
			s, _ := newServer(upstream.URL+"/v1", "")
			if mode == "timeout" {
				s.client.Timeout = 5 * time.Millisecond
			}
			w := httptest.NewRecorder()
			s.routes().ServeHTTP(w, upload(clip(), "en"))
			if w.Code != 502 || strings.Contains(w.Body.String(), "private upstream") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestBusyAndHealth(t *testing.T) {
	s, _ := newServer("http://unreachable.invalid/v1", "")
	s.slots <- struct{}{}
	s.slots <- struct{}{}
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, upload(clip(), "en"))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Start microphone") {
		t.Fatal("UI unavailable")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("missing CSP")
	}
}
