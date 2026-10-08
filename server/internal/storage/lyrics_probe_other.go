//go:build !linux && !darwin

package storage

import (
	"errors"
	"os"
)

func probeLyricTags(source *os.File, argv []string) ([]map[string]string, error) {
	return nil, errors.New("descriptor-bound embedded lyric probing is not supported on this server platform")
}
