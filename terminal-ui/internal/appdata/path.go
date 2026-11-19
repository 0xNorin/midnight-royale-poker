// Package appdata resolves Midnight Royale's conventional shared user-data
// root. New independent records belong here rather than beside the executable
// or in the legacy progress/configuration directory.
package appdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Namespace string

const (
	Production  Namespace = "production"
	Development Namespace = "development"
	ReleaseTest Namespace = "release-test"
	ETNTestnet  Namespace = "etn-testnet"
	Staging     Namespace = "staging"
)

// Dir returns the application user-data root for the requested namespace.
// Production deliberately uses the stable MidnightRoyale directory; all
// developer modes use a visibly separate sibling directory.
func Dir(namespace Namespace) (string, error) {
	return dirFor(runtime.GOOS, namespace, os.Getenv, os.UserHomeDir)
}

func dirFor(goos string, namespace Namespace, getenv func(string) string, userHomeDir func() (string, error)) (string, error) {
	appDir := "MidnightRoyale"
	switch namespace {
	case Production:
	case Development:
		appDir += "-Development"
	case ReleaseTest:
		appDir += "-ReleaseTest"
	case ETNTestnet:
		appDir += "-ETN-Testnet"
	case Staging:
		appDir += "-Staging"
	default:
		return "", fmt.Errorf("app data: unknown namespace %q", namespace)
	}

	home := func() (string, error) {
		dir, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("app data: user home directory: %w", err)
		}
		if dir == "" {
			return "", fmt.Errorf("app data: user home directory is empty")
		}
		return dir, nil
	}

	switch goos {
	case "windows":
		base := getenv("LOCALAPPDATA")
		if base == "" {
			var err error
			base, err = home()
			if err != nil {
				return "", err
			}
			base = filepath.Join(base, "AppData", "Local")
		}
		return filepath.Join(base, appDir), nil
	case "darwin":
		base, err := home()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "Library", "Application Support", appDir), nil
	default:
		base := getenv("XDG_DATA_HOME")
		if base == "" || !filepath.IsAbs(base) {
			var err error
			base, err = home()
			if err != nil {
				return "", err
			}
			base = filepath.Join(base, ".local", "share")
		}
		return filepath.Join(base, appDir), nil
	}
}
