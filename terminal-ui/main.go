// Midnight Royale is a local five-card draw game.
package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/charmbracelet/colorprofile"
	"io/fs"
	"midnight-royale/internal/adultcontent"
	"midnight-royale/internal/appdata"
	"midnight-royale/internal/audio"
	"midnight-royale/internal/logging"
	"midnight-royale/internal/progress"
	"midnight-royale/internal/simulation"
	"midnight-royale/internal/ui"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
)

var version = ui.Version

// diag is the optional client diagnostics logger, opened once the edition
// namespace is known. It is also used by main()'s panic handler.
var diag *logging.Logger

// Real payments remain globally fail-closed. A reviewed release injects a
// public signing pin, selects a commerce profile, and (for the stablecoin
// production profile only) explicitly sets productionCommerceApproved=true.
// The private signing key never enters this client or its build arguments.
// These string variables are release inputs, set with go build -ldflags -X.
// The gift-only profile approves no payment capability; the private signing
// key never enters this client or its build arguments.
var (
	productionCommerceProfile  = ""
	productionCommerceHost     = ""
	productionSigningKeyID     = ""
	productionSigningPublicKey = ""
	// Release signing identity is separate from content signing so the release
	// key can be rotated independently. Only the public key is ever compiled
	// in; the private release key stays in the operator's signing environment.
	releaseSigningKeyID     = ""
	releaseSigningPublicKey = ""
	// releaseSigningPreviousKeyID/PublicKey optionally pin the immediately-prior
	// release key during a rotation transition. They must be supplied together
	// (or both omitted) and must differ from the current key.
	releaseSigningPreviousKeyID     = ""
	releaseSigningPreviousPublicKey = ""
	// buildCommit/buildTime identify the exact source revision and build
	// timestamp (set via -ldflags -X main.buildCommit=... etc).
	buildCommit = "unknown"
	buildTime   = "unknown"
	// versionOverride lets the release pipeline stamp the user-facing version
	// without editing the edition source (set via -ldflags -X
	// main.versionOverride=...). Empty means use the edition default.
	versionOverride = ""
	// testFailStartupBakedIn (set via -ldflags) makes a build deliberately fail
	// at startup before acknowledgement, to exercise recovery in fault tests.
	testFailStartupBakedIn = ""
	// releaseChannel is the release channel this build selects. It is resolved
	// through resolveReleaseChannel(): empty (the default) falls back to the
	// build-tag default (stable for production/testnet, experimental for
	// staging); an explicit value is honoured, but a staging build is pinned to
	// experimental and fails closed on anything else. A test build may set
	// -X main.releaseChannel=test so it can never accept a public stable
	// release, and a public build never accepts test metadata.
	releaseChannel = ""
	// releaseBuildIdentity is stamped at release build time with a machine-readable
	// identity (via -X main.releaseBuildIdentity=MRI1:...). The offline signer reads
	// it back to verify the binary matches the envelope before signing.
	releaseBuildIdentity = ""
	// releaseBuildConfig is stamped at release build time with a machine-readable
	// production configuration (via -X main.releaseBuildConfig=MRCFG1:...). The
	// release gate reads it back to verify the commerce host, profile, signing
	// keys, licence pin and catalogue prices without executing the binary.
	releaseBuildConfig = ""
)

// stagingCommerceApproved is a string ldflag (empty = fail-closed), mirroring
// the production profile-string convention: a staging client may only install
// staging-signed content (once the content key is pinned) and never issues
// payments, gifts or recoveries until a later phase approves live stablecoin
// orders. The staging QA rehearsal build opts in via
//
//	-X main.stagingCommerceApproved=true
//
// to run exactly one non-money order; the fail-closed empty default is
// unchanged. (It must be a string: the Go linker's -X only sets string vars,
//
//	so a bool here would make -X a no-op or a link error.)

func validate(c ui.Config) error {
	if !slices.Contains(append(ui.ScreenOrder, "help", "pause"), c.Screen) {
		return fmt.Errorf("unknown screen %q", c.Screen)
	}
	if !slices.Contains(ui.PhaseOrder, c.Phase) {
		return fmt.Errorf("unknown phase %q", c.Phase)
	}
	if c.Stage < 1 || c.Stage > 5 {
		return fmt.Errorf("stage must be 1..5")
	}
	found := false
	for _, ch := range ui.ShippedCharacters() {
		if ch.ID == c.Character {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("unknown character %q", c.Character)
	}
	if c.Collection != "" {
		foundCollection := false
		for _, col := range ui.Collections() {
			if col.ID == ui.CollectionID(c.Collection) {
				foundCollection = true
			}
		}
		if !foundCollection {
			return fmt.Errorf("unknown collection %q", c.Collection)
		}
	}
	return nil
}

func run() error {
	var c ui.Config
	flag.StringVar(&c.Screen, "screen", "opening", "opening, collection, select, intro, game, gallery, settings, help, pause")
	flag.StringVar(&c.Character, "character", "amy", "character ID (lowercase)")
	flag.StringVar(&c.Collection, "collection", "", "collection ID for the collection selector (default: grand-riviera-hotel)")
	flag.StringVar(&c.Phase, "phase", "new", "new, hold, bet, opponent, win, loss, tie, match, victory")
	flag.IntVar(&c.Stage, "stage", 1, "approved image stage 1..5")
	flag.BoolVar(&c.ReducedMotion, "reduced-motion", false, "immediate drawer opening/closing")
	flag.BoolVar(&c.ASCII, "ascii", false, "ASCII UI symbols and card suits; artwork remains Unicode")
	flag.BoolVar(&c.Dev, "dev", false, "show fixture shortcuts")
	flag.BoolVar(&c.Demo, "fixture-demo", false, "development-only visual fixture mode (not real poker)")
	flag.Int64Var(&c.DeterministicSeed, "demo-seed", 0, "deterministic real-game seed for tests/VHS only")
	flag.BoolVar(&c.DisableSave, "no-save", false, "disable local progress writes for tests/demos")
	flag.StringVar(&c.SavePath, "save-path", "", "development override for local save path")
	check := flag.Bool("validate-assets", false, "validate embedded art and exit without entering terminal")
	ver := flag.Bool("version", false, "print prototype version")
	debugLog := flag.Bool("debug-log", false, "log UI/state transitions and content stages to the app log")
	dump := flag.String("dump", "", "write a diagnostic ANSI frame (not a terminal screenshot)")
	simulate := flag.Int("simulate", 0, "run this many headless matches per difficulty and print JSON")
	simulationSeed := flag.Int64("simulation-seed", 20260908, "deterministic headless simulation seed")
	maxSimulationHands := flag.Int("simulation-max-hands", 200, "maximum hands per simulated match")
	studyLevels := flag.Int("study-levels", 0, "if >0, run the pacing study with this many single-level matches per character/policy")
	studyAttempts := flag.Int("study-attempts", 0, "full multi-level attempts per character/policy for --study-levels")
	studyStack := flag.Int("study-stack", 0, "override starting stack for --study-levels (0 = default)")
	printSavePath := flag.Bool("print-save-path", false, "print the OS-specific local save path and exit")
	width := flag.Int("width", 104, "diagnostic dump width only")
	height := flag.Int("height", 40, "diagnostic dump height only")
	scroll := flag.Int("scroll", 0, "initial portrait row offset")
	closed := flag.Bool("drawer-closed", false, "start gameplay with drawer closed for visual comparison")
	buildInfo := flag.Bool("build-info", false, "print build identity as JSON and exit")
	flag.Parse()
	switch {
	case versionOverride != "":
		ui.Version = versionOverride
		version = versionOverride
	case false:
		if v := strings.TrimSpace(embeddedVersion); v != "" {
			ui.Version = v
			version = v
		}
	}
	if *ver {
		fmt.Println("Midnight Royale", version)
		return nil
	}
	if *buildInfo {
		return printBuildInfo()
	}
	if *studyLevels > 0 {
		results, err := simulation.RunStudy(context.Background(), simulation.StudyConfig{
			LevelMatches: *studyLevels, Attempts: *studyAttempts,
			MaxHandsPerLevel: 150, MaxHandsPerAttempt: 400,
			Seed: *simulationSeed, Stack: *studyStack,
		})
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	}
	if *simulate > 0 {
		results, err := simulation.RunDifficultySuite(context.Background(), simulation.Config{Matches: *simulate, MaxHandsPerMatch: *maxSimulationHands, Seed: *simulationSeed})
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	}
	if *printSavePath {
		path, err := progress.DefaultPath()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	}
	if e := validate(c); e != nil {
		return e
	}
	a, e := ui.LoadAssets(bundle)
	if e != nil {
		return e
	}
	dataNamespace := appdata.Production
	if c.Dev || c.Demo {
		dataNamespace = appdata.Development
	}
	c.AdultContent = adultcontent.NewDefaultStore(dataNamespace)

	dataRoot, err := appdata.Dir(dataNamespace)
	if err != nil {
		return err
	}
	diag, err = logging.Open(filepath.Join(dataRoot, "logs"), "app.log")
	if err != nil {
		return err
	}
	diag.SetDebug(*debugLog)
	defer diag.Close()
	diag.Printf("startup version=%s namespace=%s data=%s", ui.Version, dataNamespace, dataRoot)
	diag.Debugf("content: loading preinstalled content")
	preinstalled, _, e := applyPreinstalledContent(a)
	if e != nil {
		diag.Errorf("content load: %v", e)
		return e
	}
	diag.Printf("assets %d scenes %d faces", len(a.Art), len(a.Faces))
	diag.Printf("content %d characters embedded", len(preinstalled))
	if *check {
		fmt.Printf("Verified %d approved 96x72 scenes, %d separate 28x14 faces, %d 94x30 collections, and the 58x31 neon intro. SGR-only UTF-8 / TrueColor.\n", len(a.Art), len(a.Faces), len(a.Collections))
		wantScenes := 36 * 5
		if len(a.Art) != wantScenes {
			return fmt.Errorf("embedded content is incomplete: %d/%d scenes embedded", len(a.Art), wantScenes)
		}
		fmt.Printf("This build embeds the complete roster (36 characters, %d scenes).\n", wantScenes)
		return nil
	}
	if *dump != "" {
		return writePublicDiagnosticDump(*dump, bundle, c, *width, *height, *scroll, *closed)
	}
	c.Installed = preinstalled
	m := ui.New(c, a)
	m.Scroll = max(0, min(72-(*height-4), *scroll))
	if *closed {
		m.DrawerOpen = false
		m.Reveal = 0
		m.Focus = "art"
	}
	eng := audio.New(bundle)
	m.Audio = eng
	eng.SetEnabled(m.SoundEffects)

	// Stop audio cleanly if the terminal window is closed (SIGHUP). Bubble Tea
	// only handles SIGINT/SIGTERM; without this the looping theme can keep
	// playing through the WSLg audio bridge after the window is gone.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)
	go func() {
		<-sig
		eng.StopAll()
		os.Exit(0)
	}()

	if diag.DebugEnabled() {
		ui.Logf = diag.Debugf
	}
	diag.Printf("ui: starting")
	_, e = tea.NewProgram(m, tea.WithColorProfile(colorprofile.TrueColor), tea.WithFilter(ui.FilterUnchangedHover)).Run()
	eng.StopAll()
	if e != nil {
		diag.Errorf("ui: %v", e)
		return e
	}
	diag.Printf("clean shutdown")
	return nil
}

// writePublicDiagnosticDump builds a fresh presentation from the intentionally
// public embedded assets. Installed paid artwork and local installation
// authority never enter this diagnostic boundary, including in ReleaseTest.
func writePublicDiagnosticDump(path string, files fs.FS, c ui.Config, width, height, scroll int, closed bool) error {
	if width < 1 || width > 400 || height < 1 || height > 200 {
		return fmt.Errorf("diagnostic dimensions out of range")
	}
	assets, err := ui.LoadAssets(files)
	if err != nil {
		return err
	}
	c.Installed = nil
	m := ui.New(c, assets)
	m.Scroll = max(0, min(72-(height-4), scroll))
	if closed {
		m.DrawerOpen = false
		m.Reveal = 0
		m.Focus = "art"
	}
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return os.WriteFile(path, []byte(m.View().Content+"\x1b[0m\n"), 0644)
}

// applyOwnerTestProfile redirects LOCALAPPDATA and APPDATA to the established
// isolated owner-test profile so a plain launch of the development build lands
// in the same profile the launcher previously prepared. It is a no-op off
// Windows, where the ETNTestnet namespace already isolates the data.
func applyOwnerTestProfile() {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return // non-Windows: the ETNTestnet namespace isolates data
	}
	root := filepath.Join(local, "MidnightRoyale-ETN-Testnet-OwnerTests", "owner")
	os.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
	os.Setenv("APPDATA", filepath.Join(root, "Roaming"))
}

// printBuildInfo reports the compile-time build identity, separate from the
// user-facing version string.
func printBuildInfo() error {
	info := struct {
		Version                     string `json:"version"`
		Edition                     string `json:"edition"`
		Environment                 string `json:"environment"`
		Channel                     string `json:"channel"`
		Commit                      string `json:"commit"`
		Built                       string `json:"built"`
		OS                          string `json:"os"`
		Arch                        string `json:"arch"`
		MetadataFormat              int    `json:"metadata_format"`
		ReleaseSigningKeyID         string `json:"release_signing_key_id,omitempty"`
		ReleaseSigningPreviousKeyID string `json:"release_signing_previous_key_id,omitempty"`
		BuildIdentity               string `json:"build_identity,omitempty"`
		ReleaseBuildConfig          string `json:"release_build_config,omitempty"`
	}{
		Version: ui.Version, Edition: "open", Environment: "production",
		Channel: "stable", Commit: buildCommit, Built: buildTime,
		OS: runtime.GOOS, Arch: runtime.GOARCH, MetadataFormat: 1,
		ReleaseSigningKeyID: releaseSigningKeyID, ReleaseSigningPreviousKeyID: releaseSigningPreviousKeyID,
		BuildIdentity:      releaseBuildIdentity,
		ReleaseBuildConfig: releaseBuildConfig,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}
