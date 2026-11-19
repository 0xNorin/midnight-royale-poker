package main

import "embed"

// The bundle embeds the complete shipped artwork: all five scenes and face
// thumbnails for all 36 characters across the three collections, every
// collection plaque, the neon intro, and all SFX/music. Since 2.1.0 there is
// no paid-content subset: the game is free and every scene ships in plain form.
//
// This bundle is shared by the desktop client (main_entry.go) and the SSH
// guest server (mrssh_main.go). It lives in a build-tag-free file so both
// entrypoints read the same embedded content.
//
//go:embed assets/art/*.ans assets/art/*.metadata.json
//go:embed assets/faces/*.ans assets/collections/*.ans assets/midnight-royale-neon/midnight-royale.ans assets/sfx/*.ogg assets/music/*.ogg
var bundle embed.FS
