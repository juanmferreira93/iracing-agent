package watcher

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanmferreira93/iracing-agent/internal/state"
)

type FileWatcher struct {
	watchPaths []string
}

func NewFileWatcher(watchPaths []string) *FileWatcher {
	return &FileWatcher{watchPaths: watchPaths}
}

func (f *FileWatcher) Discover(index *state.Index) ([]string, error) {
	files := make([]string, 0)
	for _, root := range f.watchPaths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".ibt") {
				return nil
			}

			fingerprint, err := state.FileFingerprint(path)
			if err != nil {
				return nil
			}
			if index.Seen(fingerprint) {
				return nil
			}

			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}
