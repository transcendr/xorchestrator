package sound

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"

	"github.com/zjrosen/xorchestrator/internal/flags"
	"github.com/zjrosen/xorchestrator/internal/log"
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
// It supports a master kill switch via flags and granular per-sound configuration.
type SystemSoundService struct {
	flags          *flags.Registry
	enabledSounds  map[string]bool
	audioAvailable bool
	audioCommand   string
	audioArgs      []string
	concurrent     atomic.Int32
}

// NewSystemSoundService creates a sound service with the given configuration.
// flags provides the master kill switch (FlagSoundEnabled).
// enabledSounds controls which sounds are enabled (nil enables all sounds).
func NewSystemSoundService(flagsRegistry *flags.Registry, enabledSounds map[string]bool) *SystemSoundService {
	cmd, args := detectAudioCommand()
	available := cmd != ""

	log.Debug(log.CatConfig, "Sound service initialized",
		"audioAvailable", available,
		"audioCommand", cmd,
		"platform", runtime.GOOS,
	)

	return &SystemSoundService{
		flags:          flagsRegistry,
		enabledSounds:  enabledSounds,
		audioAvailable: available,
		audioCommand:   cmd,
		audioArgs:      args,
	}
}

// Play plays the sound file asynchronously if the use case is enabled.
// soundFile is the filename (without extension) to play from embedded sounds.
// useCase is checked against the enabledSounds config map for permission.
// Does nothing if:
//   - Master flag FlagSoundEnabled is disabled
//   - The specific use case is disabled in enabledSounds config
//   - No audio player is available on this platform
//   - The sound file is unknown (not in embedded files)
//   - Maximum concurrent sounds limit is reached
func (s *SystemSoundService) Play(soundFile, useCase string) {
	// Master kill switch - if no flags registry, default to disabled
	if s.flags == nil || !s.flags.Enabled(flags.FlagSoundEnabled) {
		log.Debug(log.CatConfig, "Sound disabled by flag", "soundFile", soundFile, "useCase", useCase)
		return
	}

	// Granular config check - use useCase for permission
	if s.enabledSounds != nil {
		if enabled, exists := s.enabledSounds[useCase]; exists && !enabled {
			log.Debug(log.CatConfig, "Sound disabled by config", "soundFile", soundFile, "useCase", useCase)
			return
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
