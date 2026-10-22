# Live Transcript reference

Browser microphone → Go backend → internal OpenShift AI / KServe → Red Hat vLLM / Whisper on one T4. DeepSeek on Foundry remains the coding model; microphone audio does not go to Foundry.

This is a tested reference for the workshop's AI-generated application. Run `go test -race ./...`, then `WHISPER_URL=http://localhost:8000/v1 go run .` with a reachable Whisper API. Open `http://localhost:8080` (localhost is a browser secure context). Use HTTPS for remote access.

The app captures mono PCM, produces independent 16 kHz WAV clips, skips quiet audio, sends clips sequentially and shows text before recording stops. Stop flushes the tail and releases the microphone. It retains neither audio nor transcripts on the server; Download saves text in the browser. There is a capture/inference delay, and fixed clip boundaries can split words. This is a workshop demo without user authentication, diarization or a native streaming Whisper protocol.

The backend accepts only short PCM16 WAV clips, limits body size and concurrent requests, sets upstream timeouts, and keeps any `WHISPER_API_KEY` server-side. The default internal service uses port 80. Access to the model is restricted by `deploy/speech/networkpolicy.yaml`.
