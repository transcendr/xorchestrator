package sound

import (
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/xorchestrator/internal/flags"
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
	s := NewSystemSoundService(nil, nil)
	var _ SoundService = s
}

// TestSystemSoundService_FlagDisabled verifies master kill switch prevents all sounds.
func TestSystemSoundService_FlagDisabled(t *testing.T) {
	// Create registry with sound disabled
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: false,
	})
	s := NewSystemSoundService(registry, nil)

	// Play should return early without attempting playback
	// We can't easily verify no playback occurred, but we can verify no panic
	require.NotPanics(t, func() {
		s.Play("test", "test")
	})
}

// TestSystemSoundService_FlagEnabled verifies sound plays when flag is enabled.
func TestSystemSoundService_FlagEnabled(t *testing.T) {
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

	// Should not panic - sound may or may not play depending on audio availability
	require.NotPanics(t, func() {
		s.Play("test", "test")
	})
}

// TestSystemSoundService_NilFlagsDisablesSound verifies nil flags disables sounds (safe default).
func TestSystemSoundService_NilFlagsDisablesSound(t *testing.T) {
	s := NewSystemSoundService(nil, nil)

	// Should not panic - nil flags means sound is disabled
	require.NotPanics(t, func() {
		s.Play("test", "test")
	})
}

// TestSystemSoundService_SoundDisabled verifies granular config disables specific use case.
func TestSystemSoundService_SoundDisabled(t *testing.T) {
	// Enable master flag but disable specific use case
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	enabledSounds := map[string]bool{
		"test":    false, // explicitly disabled
		"success": true,  // explicitly enabled
	}
	s := NewSystemSoundService(registry, enabledSounds)

	// Both should not panic - disabled use case returns early
	require.NotPanics(t, func() {
		s.Play("test", "test")       // useCase disabled
		s.Play("success", "success") // useCase enabled
	})
}

// TestSystemSoundService_SoundNotInConfig verifies use cases not in config are allowed.
func TestSystemSoundService_SoundNotInConfig(t *testing.T) {
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	enabledSounds := map[string]bool{
		"test": false, // only test is disabled
	}
	s := NewSystemSoundService(registry, enabledSounds)

	// Use case not in config should be allowed
	require.NotPanics(t, func() {
		s.Play("other", "other") // not in config, should proceed
	})
}

// TestSystemSoundService_NilEnabledSoundsAllowsAll verifies nil enabledSounds allows all.
func TestSystemSoundService_NilEnabledSoundsAllowsAll(t *testing.T) {
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

	// All use cases should be allowed with nil config
	require.NotPanics(t, func() {
		s.Play("any-sound", "any-sound")
		s.Play("another", "another")
	})
}

// TestSystemSoundService_UnknownSoundFile verifies unknown files are handled gracefully.
func TestSystemSoundService_UnknownSoundFile(t *testing.T) {
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

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

	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

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
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

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
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

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
	s := NewSystemSoundService(nil, nil)

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
	registry := flags.New(map[string]bool{
		flags.FlagSoundEnabled: true,
	})
	s := NewSystemSoundService(registry, nil)

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
		name          string
		flags         *flags.Registry
		enabledSounds map[string]bool
	}{
		{
			name:          "nil flags and nil enabledSounds",
			flags:         nil,
			enabledSounds: nil,
		},
		{
			name:          "with flags and nil enabledSounds",
			flags:         flags.New(map[string]bool{flags.FlagSoundEnabled: true}),
			enabledSounds: nil,
		},
		{
			name:          "with flags and enabledSounds",
			flags:         flags.New(map[string]bool{flags.FlagSoundEnabled: true}),
			enabledSounds: map[string]bool{"test": true},
		},
		{
			name:          "with empty enabledSounds",
			flags:         flags.New(map[string]bool{flags.FlagSoundEnabled: true}),
			enabledSounds: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSystemSoundService(tt.flags, tt.enabledSounds)
			require.NotNil(t, s)
		})
	}
}

// TestSoundFiles_Exist verifies approve.wav and deny.wav are embedded.
func TestSoundFiles_Exist(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"approve sound exists", "sounds/approve.wav"},
		{"deny sound exists", "sounds/deny.wav"},
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

	files := []string{"sounds/approve.wav", "sounds/deny.wav"}

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
	files := []string{"sounds/approve.wav", "sounds/deny.wav"}

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

	files := []string{"sounds/approve.wav", "sounds/deny.wav"}
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
