package epub

import (
	"fmt"
	"path/filepath"
	"strings"

	goreader "github.com/taylorskalyo/goreader/epub"
)

type Metadata struct {
	Title  string
	Author string
}

func ReadMetadata(path string) (Metadata, error) {
	rc, err := goreader.OpenReader(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open epub %s: %w", path, err)
	}
	defer rc.Close()

	if len(rc.Rootfiles) == 0 {
		return Metadata{}, fmt.Errorf("epub %s has no rootfiles", path)
	}

	rf := rc.Rootfiles[0]
	title := strings.TrimSpace(rf.Title)
	if title == "" {
		base := filepath.Base(path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	author := strings.TrimSpace(rf.Creator)
	if author == "" {
		author = "Unknown Author"
	}

	return Metadata{
		Title:  title,
		Author: author,
	}, nil
}
