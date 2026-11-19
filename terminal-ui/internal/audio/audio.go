// Package audio plays preloaded UI/game sound effects and the intro
// theme music asynchronously. It uses a pure-Go Ogg/Vorbis decoder and the
// cross-platform Oto backend, so no external player is needed and no process
// is spawned per sound. Every operation is non-blocking for the Bubble Tea
// update loop, and a missing output device degrades to a silent no-op instead
// of failing the game.
package audio

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/jfreymuth/oggvorbis"
)

// The shipped effects. Names are stable identifiers. The original WAV/OGG
// source material is preserved externally, out of this repository.
const (
	CardPlace    = "card-place"
	CardShuffle  = "card-shuffle"
	CardSlide    = "card-slide"
	ChipLay      = "chip-lay"
	Blip         = "blip"
	Lost         = "lost"
	TurnWin      = "turn-win"
	NewGameStart = "new-game-start"
	Win          = "win"
	Pause        = "esc"
)

// musicPath is the intro theme. It ships mono; loadMusic upmixes it to the
// effects' stereo layout so it can share Oto's single output context (Oto
// supports exactly one context per process).
const musicPath = "assets/music/midnight_royale_intro_music.ogg"

const maxConcurrent = 8

var paths = map[string]string{
	CardPlace:    "assets/sfx/card-place.ogg",
	CardShuffle:  "assets/sfx/card-shuffle.ogg",
	CardSlide:    "assets/sfx/card-slide.ogg",
	ChipLay:      "assets/sfx/chip-lay.ogg",
	Blip:         "assets/sfx/blip.ogg",
	Lost:         "assets/sfx/lost.ogg",
	TurnWin:      "assets/sfx/turn-win.ogg",
	NewGameStart: "assets/sfx/new-game-start.ogg",
	Win:          "assets/sfx/win.ogg",
	Pause:        "assets/sfx/esc.ogg",
}

// SFXPaths returns the stable SFX name → embedded-asset path mapping, for
// transports that serve the raw OGG over the wire (the web) rather than
// decoding it to PCM for a local output device.
func SFXPaths() map[string]string {
	out := make(map[string]string, len(paths))
	for name, path := range paths {
		out[name] = path
	}
	return out
}

// musicPlayer is the subset of *oto.Player the engine needs for the looping
// intro theme. It is an interface so tests can substitute a fake player
// without real audio hardware. *oto.Player satisfies it.
type musicPlayer interface {
	Play()
	PauseAndStopReading()
	IsPlaying() bool
	SetVolume(float64)
}

// Engine preloads the effects and plays them from memory. It is safe for
// concurrent use. When audio is unavailable the engine stays silent and keeps
// the reason in Err.
type Engine struct {
	mu         sync.Mutex
	ctx        *oto.Context
	enabled    bool
	ready      bool
	initErr    error
	sampleRate int
	channels   int
	pcm        map[string][]byte
	players    map[string][]*oto.Player

	musicPCM []byte
	music    musicPlayer
	fadeGen  int

	// newMusicPlayer is a test seam: when nil, PlayMusic creates a real
	// looping player from the shared Oto context. Tests inject a fake to
	// assert lifecycle behavior without audio hardware.
	newMusicPlayer func(src io.Reader) musicPlayer
}

// New loads and decodes the embedded effects and starts the output context. It
// never returns an error for missing audio hardware; such an engine is a silent
// no-op and records the reason in Err. Device initialization happens in the
// background so startup never blocks: Play is simply a no-op until the driver
// is ready.
func New(fsys fs.FS) *Engine {
	e := &Engine{enabled: true, pcm: map[string][]byte{}, players: map[string][]*oto.Player{}}
	if fsys == nil {
		e.initErr = errors.New("audio: no asset filesystem")
		e.warn()
		return e
	}
	sampleRate, channels := 44100, 2
	for name, path := range paths {
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			e.initErr = err
			e.warn()
			return e
		}
		samples, format, err := oggvorbis.ReadAll(bytes.NewReader(raw))
		if err != nil {
			e.initErr = err
			e.warn()
			return e
		}
		if format.Channels == 1 {
			samples = upmixMonoToStereo(samples)
			format.Channels = 2
		}
		if format.SampleRate != sampleRate || format.Channels != channels {
			e.initErr = errors.New("audio: mixed sample formats across effects")
			e.warn()
			return e
		}
		e.pcm[name] = float32ToBytes(samples)
	}
	e.sampleRate, e.channels = sampleRate, channels
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:      sampleRate,
		ChannelCount:    channels,
		Format:          oto.FormatFloat32LE,
		ApplicationName: "Midnight Royale",
	})
	if err != nil {
		e.initErr = err
		e.warn()
	} else {
		e.ctx = ctx
		go func() {
			select {
			case <-ready:
				e.mu.Lock()
				if ctx.Err() != nil {
					e.initErr = ctx.Err()
				} else {
					e.ready = true
				}
				e.mu.Unlock()
				if e.initErr != nil {
					e.warn()
				}
			case <-time.After(5 * time.Second):
				e.mu.Lock()
				e.initErr = errors.New("audio: output device initialization timed out")
				e.mu.Unlock()
				e.warn()
			}
		}()
	}
	e.loadMusic(fsys)
	return e
}

// loadMusic decodes the intro theme and upmixes it to the effects' channel
// layout so it can play on the shared output context. Failure is non-fatal:
// the game runs silently without the theme.
func (e *Engine) loadMusic(fsys fs.FS) {
	raw, err := fs.ReadFile(fsys, musicPath)
	if err != nil {
		log.Printf("midnight-royale: intro music unavailable: %v", err)
		return
	}
	samples, format, err := oggvorbis.ReadAll(bytes.NewReader(raw))
	if err != nil {
		log.Printf("midnight-royale: intro music decode failed: %v", err)
		return
	}
	if format.SampleRate != e.sampleRate {
		log.Printf("midnight-royale: intro music sample rate %d != effects %d; skipping", format.SampleRate, e.sampleRate)
		return
	}
	if format.Channels == 1 && e.channels == 2 {
		samples = upmixMonoToStereo(samples)
	} else if format.Channels != e.channels {
		log.Printf("midnight-royale: intro music channels %d != effects %d; skipping", format.Channels, e.channels)
		return
	}
	e.musicPCM = float32ToBytes(samples)
}

func (e *Engine) warn() { log.Printf("midnight-royale: audio disabled: %v", e.initErr) }

// Err returns why audio is unavailable, or nil when playback works.
func (e *Engine) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initErr
}

// Enabled reports whether sound effects are turned on.
func (e *Engine) Enabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.enabled
}

// SetEnabled toggles sound. Disabling stops all active effects and music
// immediately.
func (e *Engine) SetEnabled(on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = on
	if !on {
		e.stopAllLocked()
	}
}

// Stop cancels any active playback of one named sound.
func (e *Engine) Stop(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked(name)
}

// StopAll cancels every active effect and the theme. Used on quit.
func (e *Engine) StopAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopAllLocked()
}

// Play starts one sound asynchronously and returns immediately. It is a no-op
// when sound is disabled or audio is unavailable, and it bounds concurrent
// playback so rapid input cannot build an increasingly loud queue. Non-slide
// sounds replace any still-playing copy of themselves rather than stacking.
func (e *Engine) Play(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.enabled || !e.ready || e.ctx == nil {
		return
	}
	pcm, ok := e.pcm[name]
	if !ok || len(pcm) == 0 {
		return
	}
	if name != CardSlide {
		e.stopLocked(name)
	}
	e.pruneLocked()
	if e.totalLocked() >= maxConcurrent {
		e.dropOldestLocked()
	}
	p := e.ctx.NewPlayer(bytes.NewReader(pcm))
	p.Play()
	e.players[name] = append(e.players[name], p)
}

// PlayMusic starts the intro theme looping. It is a no-op when sound is
// disabled, audio is unavailable, the theme failed to load, or it is already
// playing. The theme is upmixed to stereo and shares the effects' output
// context and the master sound toggle.
func (e *Engine) PlayMusic() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.enabled || !e.ready || len(e.musicPCM) == 0 {
		return
	}
	if e.newMusicPlayer == nil && e.ctx == nil {
		return
	}
	if e.music != nil && e.music.IsPlaying() {
		return
	}
	e.fadeGen++
	var p musicPlayer
	if e.newMusicPlayer != nil {
		p = e.newMusicPlayer(&loopReader{data: e.musicPCM})
	} else {
		p = e.ctx.NewPlayer(&loopReader{data: e.musicPCM})
	}
	p.SetVolume(1.0)
	p.Play()
	e.music = p
}

// FadeMusic ramps the theme volume to silence over duration, then stops it.
// It returns immediately. Starting the music again or stopping it cancels an
// in-flight fade.
func (e *Engine) FadeMusic(d time.Duration) {
	e.mu.Lock()
	if e.music == nil {
		e.mu.Unlock()
		return
	}
	e.fadeGen++
	gen := e.fadeGen
	p := e.music
	e.mu.Unlock()
	steps := int((d + 50*time.Millisecond) / (100 * time.Millisecond))
	if steps < 1 {
		steps = 1
	}
	go func() {
		for i := 1; i <= steps; i++ {
			vol := 1.0 - float64(i)/float64(steps)
			e.mu.Lock()
			if e.fadeGen != gen || e.music != p {
				e.mu.Unlock()
				return
			}
			p.SetVolume(vol)
			e.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
		e.mu.Lock()
		if e.fadeGen == gen && e.music == p {
			e.stopMusicLocked()
		}
		e.mu.Unlock()
	}()
}

// StopMusic stops the intro theme immediately with no fade.
func (e *Engine) StopMusic() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fadeGen++
	e.stopMusicLocked()
}

func (e *Engine) stopMusicLocked() {
	if e.music != nil {
		// oto v3.4+ made Player.Close a no-op: the underlying player keeps
		// playing until it is garbage-collected, and the looping reader keeps
		// reading forever. PauseAndStopReading actually halts audible playback
		// and stops the read loop; dropping the reference then lets oto's
		// finalizer release the mux player.
		e.music.PauseAndStopReading()
		e.music = nil
	}
}

func (e *Engine) totalLocked() int {
	n := 0
	for _, ps := range e.players {
		n += len(ps)
	}
	return n
}

func (e *Engine) pruneLocked() {
	for name, ps := range e.players {
		kept := ps[:0]
		for _, p := range ps {
			if p.IsPlaying() {
				kept = append(kept, p)
			} else {
				p.Close()
			}
		}
		if len(kept) == 0 {
			delete(e.players, name)
		} else {
			e.players[name] = kept
		}
	}
}

func (e *Engine) dropOldestLocked() {
	for name, ps := range e.players {
		// UI/deal bursts must not cut off the one-shot victory fanfare.
		if name == Win {
			continue
		}
		if len(ps) > 0 {
			ps[0].Close()
			ps = ps[1:]
			if len(ps) == 0 {
				delete(e.players, name)
			} else {
				e.players[name] = ps
			}
			return
		}
	}
}

func (e *Engine) stopLocked(name string) {
	for _, p := range e.players[name] {
		p.Close()
	}
	delete(e.players, name)
}

func (e *Engine) stopAllLocked() {
	for name := range e.players {
		for _, p := range e.players[name] {
			p.Close()
		}
	}
	e.players = map[string][]*oto.Player{}
	e.fadeGen++
	e.stopMusicLocked()
}

// loopReader presents a finite PCM buffer as an infinite stream by wrapping to
// the start, so the theme loops until stopped. It always fills the requested
// buffer, avoiding short reads the player might misinterpret.
type loopReader struct {
	data []byte
	pos  int
}

func (r *loopReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		c := copy(p[n:], r.data[r.pos:])
		n += c
		r.pos += c
		if r.pos >= len(r.data) {
			r.pos = 0
		}
	}
	return n, nil
}

// float32ToBytes converts interleaved float32 samples to little-endian bytes,
// the PCM representation Oto consumes for FormatFloat32LE.
func float32ToBytes(samples []float32) []byte {
	b := make([]byte, len(samples)*4)
	for i, s := range samples {
		bits := math.Float32bits(s)
		b[i*4] = byte(bits)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return b
}

// upmixMonoToStereo duplicates a mono float32 sample stream into interleaved
// stereo, so a mono track can play on a stereo output context.
func upmixMonoToStereo(mono []float32) []float32 {
	out := make([]float32, len(mono)*2)
	for i, s := range mono {
		out[i*2] = s
		out[i*2+1] = s
	}
	return out
}
