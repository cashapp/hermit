package system

import (
	"os"
	"os/user"
	"runtime"

	"github.com/cashapp/hermit/errors"
)

// UserHomeDir tries to determine the current user's home directory.
func UserHomeDir() (string, error) {
	dir, err := os.UserHomeDir() // nolint: forbidigo
	if err == nil {
		return dir, nil
	}
	if dir = os.Getenv("HERMIT_USER_HOME"); dir != "" {
		return dir, nil
	}
	user, err := user.Current()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return user.HomeDir, nil
}

// UserCacheDir tries to determine the location of the user's local cache directory.
func UserCacheDir() (dir string, err error) {
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("LocalAppData")
		if dir == "" {
			return "", errors.New("%LocalAppData% is not defined")
		}

	case "darwin", "ios":
		dir, err = UserHomeDir()
		if err != nil {
			return "", errors.WithStack(err)
		}
		dir += "/Library/Caches"

	default: // Unix
		dir = os.Getenv("XDG_CACHE_HOME")
		if dir == "" {
			dir, err = UserHomeDir()
			if err != nil {
				return "", errors.WithStack(err)
			}
			dir += "/.cache"
		}
	}

	return dir, nil
}
