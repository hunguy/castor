package whisper

// Config holds settings for the in-process whisper.cpp transcriber. It is
// the sole mechanism for enabling subtitle generation; there is no CLI
// override. Set Enable: true in config.yaml (or CASTOR_WHISPER__ENABLE=true)
// to opt in. Transcription runs alongside the cast off the puller's audio
// feed; playback waits only for a small opening lead (seconds, not minutes).
type Config struct {
	Enable       bool   `yaml:"enable"`
	ModelPath    string `yaml:"model_path"`
	Language     string `yaml:"language"`      // BCP-47, "auto" to detect
	ChunkSeconds int    `yaml:"chunk_seconds"` // length of each audio window passed to the whisper context
	Threads      int    `yaml:"threads"`       // whisper threads (0 = bindings default)
}
