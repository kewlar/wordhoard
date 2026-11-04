package epub

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	goreader "github.com/taylorskalyo/goreader/epub"
	"golang.org/x/net/html"
)

type Metadata struct {
	Title  string
	Author string
}

type Chapter struct {
	ID         string
	HREF       string
	Title      string
	Paragraphs []string
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

func ReadChapters(path string) ([]Chapter, error) {
	rc, err := goreader.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open epub %s: %w", path, err)
	}
	defer rc.Close()

	if len(rc.Rootfiles) == 0 {
		return nil, fmt.Errorf("epub %s has no rootfiles", path)
	}

	rf := rc.Rootfiles[0]
	var items []*goreader.Item
	for i := range rf.Spine.Itemrefs {
		if rf.Spine.Itemrefs[i].Item != nil {
			items = append(items, rf.Spine.Itemrefs[i].Item)
		}
	}
	if len(items) == 0 {
		for i := range rf.Manifest.Items {
			it := &rf.Manifest.Items[i]
			if isHTMLItem(it.HREF, it.MediaType) {
				items = append(items, it)
			}
		}
	}

	var chapters []Chapter
	for idx, item := range items {
		if !isHTMLItem(item.HREF, item.MediaType) {
			continue
		}
		r, err := item.Open()
		if err != nil {
			continue
		}
		title, paragraphs, parseErr := parseHTMLChapter(r)
		r.Close()
		if parseErr != nil {
			continue
		}
		if title == "" {
			title = fmt.Sprintf("Chapter %d", idx+1)
		}
		if len(paragraphs) > 0 {
			chapters = append(chapters, Chapter{
				ID:         item.ID,
				HREF:       item.HREF,
				Title:      title,
				Paragraphs: paragraphs,
			})
		}
	}

	return chapters, nil
}

func isHTMLItem(href, mediaType string) bool {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	if strings.Contains(mt, "html") || strings.Contains(mt, "xml") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(href))
	return ext == ".xhtml" || ext == ".html" || ext == ".htm"
}

func parseHTMLChapter(r io.Reader) (string, []string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", nil, err
	}

	var title string
	var paragraphs []string

	var extractText func(*html.Node) string
	extractText = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" {
				return ""
			}
		}
		var sb strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			part := extractText(c)
			if part != "" {
				if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") && !strings.HasPrefix(part, " ") {
					sb.WriteByte(' ')
				}
				sb.WriteString(part)
			}
		}
		return sb.String()
	}

	hasBlockChild := func(n *html.Node) bool {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode {
				switch strings.ToLower(c.Data) {
				case "p", "div", "li", "blockquote", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6":
					return true
				}
			}
		}
		return false
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "script", "style":
				return
			case "title":
				if title == "" {
					title = cleanWhitespace(extractText(n))
				}
				return
			case "head":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				return
			case "h1", "h2", "h3":
				txt := cleanWhitespace(extractText(n))
				if title == "" && txt != "" {
					title = txt
				}
				if txt != "" {
					paragraphs = append(paragraphs, txt)
				}
				return
			case "p", "li", "blockquote", "h4", "h5", "h6":
				txt := cleanWhitespace(extractText(n))
				if txt != "" {
					paragraphs = append(paragraphs, txt)
				}
				return
			case "div", "section", "article", "body":
				if !hasBlockChild(n) {
					txt := cleanWhitespace(extractText(n))
					if txt != "" {
						paragraphs = append(paragraphs, txt)
					}
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return title, paragraphs, nil
}

func cleanWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

