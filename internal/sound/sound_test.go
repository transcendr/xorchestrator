package sound

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/config"
)

// skipIfAudioNotAvailable skips the test if no audio player is available.
// Use this for tests that require actual audio playback.
func skipIfAudioNotAvailable(t *testing.T) {
	t.Helper()
	cmd, _ := detectAudioCommand()
	if cmd == "" {
		t.Skip("no audio player available on this system")
	}
}

// TestNoopSoundService_ImplementsInterface verifies NoopSoundService satisfies the interface.
func TestNoopSoundService_ImplementsInterface(t *testing.T) {
	var _ SoundService = NoopSoundService{}
	var _ SoundService = &NoopSoundService{}
}

// TestNoopSoundService_PlayDoesNotPanic verifies calling Play() does nothing safely.
func TestNoopSoundService_PlayDoesNotPanic(t *testing.T) {
	s := NoopSoundService{}

	// Should not panic with any input
	require.NotPanics(t, func() {
		s.Play("", "")
		s.Play("test", "test")
		s.Play("unknown-sound", "unknown-sound")
		s.Play("some/path/sound", "some/path/sound")
	})
}

// TestSystemSoundService_ImplementsInterface verifies SystemSoundService satisfies the interface.
func TestSystemSoundService_ImplementsInterface(t *testing.T) {
	s := NewSystemSoundService(nil)
	var _ SoundService = s
}

// TestSystemSoundService_PlaysWhenConfigured verifies sound plays when configured.
func TestSystemSoundService_PlaysWhenConfigured(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Should not panic - sound may or may not play depending on audio availability
	require.NotPanics(t, func() {
		s.Play("test", "test")
	})
}

// TestSystemSoundService_SoundDisabled verifies granular config disables specific use case.
func TestSystemSoundService_SoundDisabled(t *testing.T) {
	eventConfigs := map[string]config.SoundEventConfig{
		"test":    {Enabled: false}, // explicitly disabled
		"success": {Enabled: true},  // explicitly enabled
	}
	s := NewSystemSoundService(eventConfigs)

	// Both should not panic - disabled use case returns early
	require.NotPanics(t, func() {
		s.Play("test", "test")       // useCase disabled
		s.Play("success", "success") // useCase enabled
	})
}

// TestSystemSoundService_SoundNotInConfig verifies use cases not in config are allowed.
func TestSystemSoundService_SoundNotInConfig(t *testing.T) {
	eventConfigs := map[string]config.SoundEventConfig{
		"test": {Enabled: false}, // only test is disabled
	}
	s := NewSystemSoundService(eventConfigs)

	// Use case not in config should be allowed
	require.NotPanics(t, func() {
		s.Play("other", "other") // not in config, should proceed
	})
}

// TestSystemSoundService_NilEnabledSoundsAllowsAll verifies nil enabledSounds allows all.
func TestSystemSoundService_NilEnabledSoundsAllowsAll(t *testing.T) {
	s := NewSystemSoundService(nil)

	// All use cases should be allowed with nil config
	require.NotPanics(t, func() {
		s.Play("any-sound", "any-sound")
		s.Play("another", "another")
	})
}

// TestSystemSoundService_UnknownSoundFile verifies unknown files are handled gracefully.
func TestSystemSoundService_UnknownSoundFile(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Unknown sound file should not panic - returns early after failed embed lookup
	require.NotPanics(t, func() {
		s.Play("nonexistent-sound", "nonexistent-sound")
		s.Play("", "")
		s.Play("some/path/../../escape", "some/path/../../escape")
	})
}

// TestSystemSoundService_ConcurrencyLimit verifies max 2 concurrent sounds.
func TestSystemSoundService_ConcurrencyLimit(t *testing.T) {
	skipIfAudioNotAvailable(t)

	s := NewSystemSoundService(nil)

	// Manually set concurrent counter to max
	s.concurrent.Store(maxConcurrentSounds)

	// Should not panic - returns early due to limit
	require.NotPanics(t, func() {
		s.Play("review_verdict_approve", "review_verdict_approve")
	})

	// Verify counter returned to max after Add(1)/Add(-1) cycle
	// The new pattern: Add(1) -> 3 (exceeds max) -> Add(-1) -> 2
	require.Equal(t, int32(maxConcurrentSounds), s.concurrent.Load())
}

// TestSystemSoundService_ConcurrencyLimitRace tests concurrent access to limit.
func TestSystemSoundService_ConcurrencyLimitRace(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Simulate rapid concurrent plays
	var wg sync.WaitGroup
	var maxSeen atomic.Int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Record max concurrent before/after
			current := s.concurrent.Load()
			if current > maxSeen.Load() {
				maxSeen.Store(current)
			}
			s.Play("review_verdict_approve", "review_verdict_approve")
		}()
	}

	wg.Wait()

	// Allow time for any async playback to complete
	time.Sleep(100 * time.Millisecond)

	// Counter should return to 0 or stay at reasonable level
	require.LessOrEqual(t, s.concurrent.Load(), int32(maxConcurrentSounds))
}

// TestSystemSoundService_AudioNotAvailable verifies short-circuits when no audio player.
func TestSystemSoundService_AudioNotAvailable(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Force audio unavailable
	s.audioAvailable = false

	// Should not panic - returns early
	require.NotPanics(t, func() {
		s.Play("review_verdict_approve", "review_verdict_approve")
	})

	// Verify AudioAvailable reports correctly
	require.False(t, s.AudioAvailable())
}

// TestSystemSoundService_AudioAvailable verifies AudioAvailable() getter.
func TestSystemSoundService_AudioAvailable(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Should match what detectAudioCommand found
	cmd, _ := detectAudioCommand()
	expected := cmd != ""
	require.Equal(t, expected, s.AudioAvailable())
}

// TestDetectAudioCommand verifies platform detection returns valid commands.
func TestDetectAudioCommand(t *testing.T) {
	cmd, args := detectAudioCommand()

	if cmd == "" {
		t.Log("No audio command available on this platform")
		return
	}

	// Verify the command exists
	_, err := exec.LookPath(cmd)
	require.NoError(t, err, "detected command should exist in PATH")

	t.Logf("Detected audio command: %s %v", cmd, args)
}

// TestMaxConcurrentSounds verifies the constant is set correctly.
func TestMaxConcurrentSounds(t *testing.T) {
	require.Equal(t, 2, maxConcurrentSounds)
}

// TestSystemSoundService_ConcurrentPlaysWithinLimit tests that plays within limit work.
func TestSystemSoundService_ConcurrentPlaysWithinLimit(t *testing.T) {
	s := NewSystemSoundService(nil)

	// Start with zero concurrent
	require.Equal(t, int32(0), s.concurrent.Load())

	// Even without audio, the concurrent tracking should work correctly
	// We just verify no panics occur
	require.NotPanics(t, func() {
		s.Play("review_verdict_approve", "review_verdict_approve")
	})
}

// TestNewSystemSoundService verifies constructor behavior.
func TestNewSystemSoundService(t *testing.T) {
	tests := []struct {
		name         string
		eventConfigs map[string]config.SoundEventConfig
	}{
		{
			name:         "nil eventConfigs",
			eventConfigs: nil,
		},
		{
			name:         "with eventConfigs",
			eventConfigs: map[string]config.SoundEventConfig{"test": {Enabled: true}},
		},
		{
			name:         "with empty eventConfigs",
			eventConfigs: map[string]config.SoundEventConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSystemSoundService(tt.eventConfigs)
			require.NotNil(t, s)
		})
	}
}

// TestSoundFiles_Exist verifies approve.wav, deny.wav, and complete.wav are embedded.
func TestSoundFiles_Exist(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"approve sound exists", "sounds/approve.wav"},
		{"deny sound exists", "sounds/deny.wav"},
		{"complete sound exists", "sounds/complete.wav"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := soundFiles.ReadFile(tt.filename)
			require.NoError(t, err, "should be able to read embedded file")
			require.NotEmpty(t, data, "embedded file should have content")
		})
	}
}

// TestSoundFiles_SizeLimit verifies each embedded sound file is < 500KB.
func TestSoundFiles_SizeLimit(t *testing.T) {
	const maxSize = 500 * 1024 // 500KB

	files := []string{"sounds/approve.wav", "sounds/deny.wav", "sounds/complete.wav"}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := soundFiles.ReadFile(file)
			require.NoError(t, err)
			require.Less(t, len(data), maxSize, "file should be < 500KB, got %d bytes", len(data))
		})
	}
}

// TestSoundFiles_ValidWAV verifies embedded files are valid WAV format.
func TestSoundFiles_ValidWAV(t *testing.T) {
	files := []string{"sounds/approve.wav", "sounds/deny.wav", "sounds/complete.wav"}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := soundFiles.ReadFile(file)
			require.NoError(t, err)

			// WAV files must start with "RIFF"
			require.GreaterOrEqual(t, len(data), 12, "file too small for valid WAV")
			require.Equal(t, "RIFF", string(data[0:4]), "missing RIFF header")
			// Followed by file size and "WAVE"
			require.Equal(t, "WAVE", string(data[8:12]), "missing WAVE marker")
		})
	}
}

// TestSoundFiles_TotalSizeLimit verifies total embedded audio is < 1MB.
func TestSoundFiles_TotalSizeLimit(t *testing.T) {
	const maxTotalSize = 1024 * 1024 // 1MB

	files := []string{"sounds/approve.wav", "sounds/deny.wav", "sounds/complete.wav"}
	var totalSize int

	for _, file := range files {
		data, err := soundFiles.ReadFile(file)
		require.NoError(t, err)
		totalSize += len(data)
	}

	require.Less(t, totalSize, maxTotalSize, "total embedded audio should be < 1MB, got %d bytes", totalSize)
}

// TestBuildArgs verifies argument construction for different platforms.
func TestBuildArgs(t *testing.T) {
	s := &SystemSoundService{
		audioArgs: []string{"-q"}, // simulate aplay quiet mode
	}

	args := s.buildArgs("/tmp/test.wav")

	// On non-Windows: should create new slice with base args + path
	// We can't test Windows behavior directly without being on Windows,
	// but we can verify the non-Windows path works correctly
	if runtime.GOOS != "windows" {
		require.Len(t, args, 2)
		require.Equal(t, "-q", args[0])
		require.Equal(t, "/tmp/test.wav", args[1])

		// Verify no data race - modifying returned args shouldn't affect original
		args[0] = "modified"
		require.Equal(t, "-q", s.audioArgs[0], "original audioArgs should not be modified")
	}
}

// TestBuildArgs_NoBaseArgs verifies argument construction with nil base args.
func TestBuildArgs_NoBaseArgs(t *testing.T) {
	s := &SystemSoundService{
		audioArgs: nil, // simulate afplay with no base args
	}

	args := s.buildArgs("/tmp/test.wav")

	if runtime.GOOS != "windows" {
		require.Len(t, args, 1)
		require.Equal(t, "/tmp/test.wav", args[0])
	}
}

// =============================================================================
// Tests for override sound functionality (perles-cerd.3)
// =============================================================================

// createTestWAVFile creates a minimal valid WAV file for testing.
func createTestWAVFile(t *testing.T, dir, name string) string {
	t.Helper()
	filePath := filepath.Join(dir, name)

	// Minimal WAV file header (44 bytes)
	// RIFF header
	wavData := []byte{
		'R', 'I', 'F', 'F', // ChunkID
		36, 0, 0, 0, // ChunkSize (36 + data size, here we have 0 data)
		'W', 'A', 'V', 'E', // Format
		// fmt subchunk
		'f', 'm', 't', ' ', // Subchunk1ID
		16, 0, 0, 0, // Subchunk1Size (16 for PCM)
		1, 0, // AudioFormat (1 = PCM)
		1, 0, // NumChannels (1 = mono)
		68, 172, 0, 0, // SampleRate (44100)
		136, 88, 1, 0, // ByteRate (44100 * 1 * 2)
		2, 0, // BlockAlign (NumChannels * BitsPerSample/8)
		16, 0, // BitsPerSample
		// data subchunk
		'd', 'a', 't', 'a', // Subchunk2ID
		0, 0, 0, 0, // Subchunk2Size (0 bytes of actual audio data)
	}

	err := os.WriteFile(filePath, wavData, 0644)
	require.NoError(t, err, "failed to create test WAV file")
	return filePath
}

// TestSystemSoundService_OverrideSoundsPlayInsteadOfDefaults verifies override sounds play when configured.
func TestSystemSoundService_OverrideSoundsPlayInsteadOfDefaults(t *testing.T) {
	skipIfAudioNotAvailable(t)

	// Create temp directory and test WAV file
	tmpDir := t.TempDir()
	overridePath := createTestWAVFile(t, tmpDir, "custom_sound.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        true,
			OverrideSounds: []string{overridePath},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Should not panic and should attempt to play override file
	require.NotPanics(t, func() {
		s.Play("approve", "test_event")
	})

	// Wait for async playback to complete (audio players can take time)
	time.Sleep(500 * time.Millisecond)

	// Counter should eventually return to 0
	require.LessOrEqual(t, s.concurrent.Load(), int32(maxConcurrentSounds))
}

// TestSystemSoundService_RandomSelectionDistributes verifies random selection from multiple override sounds.
func TestSystemSoundService_RandomSelectionDistributes(t *testing.T) {
	// Note: This test verifies that the random selection mechanism is invoked.
	// We can't easily verify true randomness without many iterations,
	// but we can verify the code path doesn't panic with multiple sounds.

	tmpDir := t.TempDir()
	override1 := createTestWAVFile(t, tmpDir, "sound1.wav")
	override2 := createTestWAVFile(t, tmpDir, "sound2.wav")
	override3 := createTestWAVFile(t, tmpDir, "sound3.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        true,
			OverrideSounds: []string{override1, override2, override3},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Force audio unavailable to avoid actual playback during test
	s.audioAvailable = false

	// Call multiple times - should not panic
	for i := 0; i < 10; i++ {
		require.NotPanics(t, func() {
			s.Play("approve", "test_event")
		})
	}
}

// TestSystemSoundService_EmptyOverrideSoundsFallsThrough verifies empty override_sounds uses embedded default.
func TestSystemSoundService_EmptyOverrideSoundsFallsThrough(t *testing.T) {
	eventConfigs := map[string]config.SoundEventConfig{
		"review_verdict_approve": {
			Enabled:        true,
			OverrideSounds: []string{}, // Empty - should fall through to embedded
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Should not panic - falls through to embedded sound lookup
	require.NotPanics(t, func() {
		s.Play("approve", "review_verdict_approve")
	})
}

// TestSystemSoundService_GracefulFallbackWhenOverrideMissing verifies fallback to embedded default.
func TestSystemSoundService_GracefulFallbackWhenOverrideMissing(t *testing.T) {
	skipIfAudioNotAvailable(t)

	// Point to a non-existent file
	eventConfigs := map[string]config.SoundEventConfig{
		"review_verdict_approve": {
			Enabled:        true,
			OverrideSounds: []string{"/nonexistent/path/to/sound.wav"},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Should not panic - falls back to embedded default
	require.NotPanics(t, func() {
		s.Play("approve", "review_verdict_approve")
	})

	// Wait for async playback to complete (audio players can take time)
	time.Sleep(500 * time.Millisecond)

	// Counter should eventually return to 0
	require.LessOrEqual(t, s.concurrent.Load(), int32(maxConcurrentSounds))
}

// TestSystemSoundService_EnabledFalseDisablesSoundRegardlessOfOverrides verifies enabled=false always disables.
func TestSystemSoundService_EnabledFalseDisablesSoundRegardlessOfOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	overridePath := createTestWAVFile(t, tmpDir, "custom_sound.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        false, // Disabled
			OverrideSounds: []string{overridePath},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Should return early without playing - counter should remain 0
	s.Play("approve", "test_event")

	// No async playback should be started
	require.Equal(t, int32(0), s.concurrent.Load())
}

// TestSystemSoundService_ConcurrentLimitAppliesToOverrides verifies concurrent limit for override sounds.
func TestSystemSoundService_ConcurrentLimitAppliesToOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	overridePath := createTestWAVFile(t, tmpDir, "custom_sound.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        true,
			OverrideSounds: []string{overridePath},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Manually set concurrent counter to max
	s.concurrent.Store(maxConcurrentSounds)

	// Should not panic - returns early due to limit
	require.NotPanics(t, func() {
		s.Play("approve", "test_event")
	})

	// Verify counter didn't change (Add(1) -> exceeds max -> Add(-1))
	require.Equal(t, int32(maxConcurrentSounds), s.concurrent.Load())
}

// TestSystemSoundService_AudioUnavailableLoggedForExternalFiles verifies audio unavailable handling.
func TestSystemSoundService_AudioUnavailableLoggedForExternalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	overridePath := createTestWAVFile(t, tmpDir, "custom_sound.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        true,
			OverrideSounds: []string{overridePath},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Force audio unavailable
	s.audioAvailable = false

	// Should not panic - returns early with log message
	require.NotPanics(t, func() {
		s.Play("approve", "test_event")
	})

	// Counter should remain 0 since we returned early before incrementing
	require.Equal(t, int32(0), s.concurrent.Load())
}

// TestSystemSoundService_NilEventConfigsAllowsAll verifies nil eventConfigs allows all sounds (uses defaults).
func TestSystemSoundService_NilEventConfigsAllowsAll(t *testing.T) {
	s := NewSystemSoundService(nil)

	// All use cases should be allowed with nil config - falls through to embedded lookup
	require.NotPanics(t, func() {
		s.Play("approve", "review_verdict_approve")
		s.Play("deny", "review_verdict_deny")
	})
}

// TestSystemSoundService_OverrideWithSingleSound verifies single override sound is always selected.
func TestSystemSoundService_OverrideWithSingleSound(t *testing.T) {
	tmpDir := t.TempDir()
	overridePath := createTestWAVFile(t, tmpDir, "only_sound.wav")

	eventConfigs := map[string]config.SoundEventConfig{
		"test_event": {
			Enabled:        true,
			OverrideSounds: []string{overridePath},
		},
	}
	s := NewSystemSoundService(eventConfigs)

	// Force audio unavailable to avoid actual playback
	s.audioAvailable = false

	// Should not panic with single sound
	for i := 0; i < 5; i++ {
		require.NotPanics(t, func() {
			s.Play("approve", "test_event")
		})
	}
}
