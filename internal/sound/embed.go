// Package sound provides audio feedback for orchestration events.
// It supports cross-platform playback via OS-native audio commands.
package sound

import "embed"

// soundFiles contains embedded WAV files for audio feedback.
// The sounds directory should contain WAV assets at build time.
//
//go:embed sounds/*.wav
var soundFiles embed.FS
