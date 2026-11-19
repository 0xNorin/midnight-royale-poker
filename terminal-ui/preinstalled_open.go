package main

import (
	"fmt"

	"midnight-royale/internal/ui"
)

// Every scene of every character in every collection is part of the shared
// embedded bundle as plain, unencrypted artwork: there is no package
// decryption, no key material, no entitlement check and no server round-trip.
// Loading the game therefore grants the complete roster.
//
// The function verifies that the embedded artwork for every catalogued
// character is actually present (all five scenes) before reporting it, so a
// packaging mistake fails loudly at startup instead of showing a character the
// player cannot play.
func applyPreinstalledContent(a *ui.Assets) (map[string]bool, string, error) {
	installed := make(map[string]bool)
	for _, ch := range ui.ShippedCharacters() {
		for stage := 1; stage <= 5; stage++ {
			key := fmt.Sprintf("%s%02d", ch.ID, stage)
			if a.Art[key] == nil {
				return nil, "", fmt.Errorf("embedded content is missing scene %s", key)
			}
		}
		installed[ch.ID] = true
		if diag != nil {
			diag.Debugf("content: %s included", ch.ID)
		}
	}
	if len(installed) == 0 {
		return nil, "", fmt.Errorf("embedded content is empty: no catalogued characters")
	}
	return installed, "All collections included", nil
}
