package sound

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/log"
)

// SoundService plays audio feedback. Implementations handle all errors
// internally - Play is fire-and-forget.
type SoundService interface {
	// Play plays the sound file asynchronously if the use case is enabled.
	// soundFile is the filename (without extension) to play from embedded sounds.
	// useCase is checked against the enabledSounds config map for permission.
	// Errors are logged, not returned. Unknown sound files are silently ignored.
	Play(soundFile, useCase string)
}

// NoopSoundService is a SoundService that does nothing.
// Use this as a safe default when sound is disabled or unavailable.
type NoopSoundService struct{}

// Play does nothing. Safe to call with any input.
func (NoopSoundService) Play(_, _ string) {}

// maxConcurrentSounds limits simultaneous audio playback to prevent audio overload.
const maxConcurrentSounds = 2

// SystemSoundService plays sounds via OS-native audio commands.
// It supports granular per-event configuration.
type SystemSoundService struct {
	eventConfigs   map[string]config.SoundEventConfig
	audioAvailable bool
	audioCommand   string
	audioArgs      []string
	concurrent     atomic.Int32
}

// NewSystemSoundService creates a sound service with the given configuration.
// eventConfigs maps event names to their configurations (nil uses all defaults).
func NewSystemSoundService(eventConfigs map[string]config.SoundEventConfig) *SystemSoundService {
	cmd, args := detectAudioCommand()
	available := cmd != ""

	log.Debug(log.CatConfig, "Sound service initialized",
		"audioAvailable", available,
		"audioCommand", cmd,
		"platform", runtime.GOOS,
	)

	return &SystemSoundService{
		eventConfigs:   eventConfigs,
		audioAvailable: available,
		audioCommand:   cmd,
		audioArgs:      args,
	}
}

// Play plays the sound file asynchronously if the use case is enabled.
// soundFile is the filename (without extension) to play from embedded sounds.
// useCase is checked against the eventConfigs map for permission and override sounds.
// Does nothing if:
//   - The specific use case is disabled in eventConfigs
//   - No audio player is available on this platform
//   - The sound file is unknown (not in embedded files)
//   - Maximum concurrent sounds limit is reached
func (s *SystemSoundService) Play(soundFile, useCase string) {
	// Check event configuration
	if s.eventConfigs != nil {
		if eventConfig, exists := s.eventConfigs[useCase]; exists {
			// Per-event enabled check
			if !eventConfig.Enabled {
				log.Debug(log.CatConfig, "Sound disabled by config", "soundFile", soundFile, "useCase", useCase)
				return
			}

			// Check for override sounds
			if len(eventConfig.OverrideSounds) > 0 {
				// Random selection for variety using math/rand/v2 (auto-seeded)
				// Using math/rand for non-security-critical random selection
				idx := rand.IntN(len(eventConfig.OverrideSounds)) //nolint:gosec // Random sound selection, not security-critical
				overridePath := eventConfig.OverrideSounds[idx]
				s.playExternalFile(overridePath, soundFile, useCase)
				return
			}
		}
	}

	// Audio availability check
	if !s.audioAvailable {
		log.Debug(log.CatConfig, "No audio player available", "soundFile", soundFile, "useCase", useCase)
		return
	}

	// Look up the sound file
	filename := "sounds/" + soundFile + ".wav"
	data, err := soundFiles.ReadFile(filename)
	if err != nil {
		log.Debug(log.CatConfig, "Unknown sound file", "soundFile", soundFile, "useCase", useCase, "error", err)
		return
	}

	// Concurrency limit check - atomic increment then check
	if s.concurrent.Add(1) > maxConcurrentSounds {
		s.concurrent.Add(-1)
		log.Debug(log.CatConfig, "Concurrent sound limit reached", "soundFile", soundFile, "useCase", useCase)
		return
	}

	// Play asynchronously
	go s.playAsync(soundFile, data)
}

// playExternalFile plays a sound file from the filesystem (for override sounds).
// soundFile is the original embedded sound name used for fallback mapping.
// Falls back to embedded default if file is unavailable at runtime.
func (s *SystemSoundService) playExternalFile(filePath, soundFile, useCase string) {
	if !s.audioAvailable {
		log.Debug(log.CatConfig, "No audio player available for external file", "path", filePath, "useCase", useCase)
		return
	}

	// Concurrency limit check - atomic increment then check
	if s.concurrent.Add(1) > maxConcurrentSounds {
		s.concurrent.Add(-1)
		log.Debug(log.CatConfig, "Concurrent sound limit reached", "path", filePath, "useCase", useCase)
		return
	}

	go s.playExternalAsync(filePath, soundFile, useCase)
}

// playExternalAsync handles external file playback in a goroutine.
// Falls back to embedded default if file is unavailable at runtime.
func (s *SystemSoundService) playExternalAsync(filePath, soundFile, useCase string) {
	defer s.concurrent.Add(-1)

	// Verify file exists at runtime (for graceful fallback)
	if _, err := os.Stat(filePath); err != nil {
		log.Debug(log.CatConfig, "Override sound file not found, falling back to default",
			"path", filePath, "soundFile", soundFile, "useCase", useCase, "error", err)
		// Fall back to embedded default
		s.playEmbeddedFallback(soundFile, useCase)
		return
	}

	// Build command args and execute
	args := s.buildArgs(filePath)
	cmd := exec.Command(s.audioCommand, args...) //nolint:gosec // audioCommand validated at construction
	if err := cmd.Run(); err != nil {
		log.Debug(log.CatConfig, "External audio playback failed", "path", filePath, "useCase", useCase, "error", err)
		return
	}
}

// playEmbeddedFallback plays the embedded default sound as a fallback.
// Used when an override file is unavailable at runtime.
func (s *SystemSoundService) playEmbeddedFallback(soundFile, useCase string) {
	// Look up the embedded sound file
	filename := "sounds/" + soundFile + ".wav"
	data, err := soundFiles.ReadFile(filename)
	if err != nil {
		log.Debug(log.CatConfig, "Fallback sound file not found", "soundFile", soundFile, "useCase", useCase, "error", err)
		return
	}

	// Note: We don't increment concurrent again - already counted by playExternalFile
	// Just do the playback synchronously in this goroutine
	tmpFile, err := os.CreateTemp("", "perles-sound-*.wav")
	if err != nil {
		log.Debug(log.CatConfig, "Failed to create temp file for fallback", "soundFile", soundFile, "error", err)
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			log.Debug(log.CatConfig, "Failed to remove temp file", "name", soundFile, "error", err)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			log.Debug(log.CatConfig, "Failed to close temp file after write error", "name", soundFile, "error", closeErr)
		}
		log.Debug(log.CatConfig, "Failed to write temp file for fallback", "soundFile", soundFile, "error", err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		log.Debug(log.CatConfig, "Failed to close temp file for fallback", "soundFile", soundFile, "error", err)
		return
	}

	args := s.buildArgs(tmpPath)
	cmd := exec.Command(s.audioCommand, args...) //nolint:gosec // audioCommand validated at construction
	if err := cmd.Run(); err != nil {
		log.Debug(log.CatConfig, "Fallback audio playback failed", "soundFile", soundFile, "error", err)
		return
	}
}

// playAsync handles the actual playback in a goroutine.
func (s *SystemSoundService) playAsync(name string, data []byte) {
	defer s.concurrent.Add(-1)

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "xorchestrator-sound-*.wav")
	if err != nil {
		log.Debug(log.CatConfig, "Failed to create temp file", "name", name, "error", err)
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			log.Debug(log.CatConfig, "Failed to remove temp file", "name", name, "error", err)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			log.Debug(log.CatConfig, "Failed to close temp file after write error", "name", name, "error", closeErr)
		}
		log.Debug(log.CatConfig, "Failed to write temp file", "name", name, "error", err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		log.Debug(log.CatConfig, "Failed to close temp file", "name", name, "error", err)
		return
	}

	// Build command args - create new slice to avoid data race from append
	args := s.buildArgs(tmpPath)

	// Execute audio command - audioCommand is validated at construction time
	// via detectAudioCommand which only returns commands found in PATH
	cmd := exec.Command(s.audioCommand, args...) //nolint:gosec // audioCommand validated at construction
	if err := cmd.Run(); err != nil {
		log.Debug(log.CatConfig, "Audio playback failed", "name", name, "error", err)
		return
	}
}

// buildArgs constructs command arguments for the audio player.
// Creates a new slice to avoid data races from shared backing arrays.
func (s *SystemSoundService) buildArgs(tmpPath string) []string {
	// Windows PowerShell needs path interpolation into the command
	if runtime.GOOS == "windows" {
		return []string{"-c", fmt.Sprintf("(New-Object System.Media.SoundPlayer '%s').PlaySync()", tmpPath)}
	}

	// Other platforms: append path to base args
	args := make([]string, len(s.audioArgs)+1)
	copy(args, s.audioArgs)
	args[len(args)-1] = tmpPath
	return args
}

// detectAudioCommand returns the audio command and base arguments for the current platform.
// Returns empty string if no audio player is available.
// Note: Windows args are constructed in buildArgs() due to path interpolation needs.
func detectAudioCommand() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		// macOS: afplay is always available
		if path, err := exec.LookPath("afplay"); err == nil {
			return path, nil
		}
	case "linux":
		// Linux: prefer paplay (PulseAudio), fall back to aplay (ALSA)
		if path, err := exec.LookPath("paplay"); err == nil {
			return path, nil
		}
		if path, err := exec.LookPath("aplay"); err == nil {
			return path, []string{"-q"} // quiet mode
		}
	case "windows":
		// Windows: use PowerShell with System.Media.SoundPlayer
		// Args are constructed in buildArgs() due to path interpolation
		if path, err := exec.LookPath("powershell.exe"); err == nil {
			return path, nil
		}
	}
	return "", nil
}

// AudioAvailable returns true if an audio player was detected on this platform.
// Useful for testing and conditional UI feedback.
func (s *SystemSoundService) AudioAvailable() bool {
	return s.audioAvailable
}
