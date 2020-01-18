package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/andybalholm/cascadia"
	"github.com/kardianos/task"
	"golang.org/x/net/html"
)

func main() {
	p := &P{
		DownloadExt: []string{""},
	}
	flag.StringVar(&p.URL, "url", "", "HTML URL to download and get links from.")
	flag.StringVar(&p.DownloadTo, "folder", "", "folder to download to")
	flag.StringVar(&p.DownloadExt[0], "ext", ".pdf", "Limit downloads to this ext")
	flag.Prarse()

	if len(p.URL) == 0 {
		log.Fatal("missing url flag")
	}
	if len(p.DownloadTo) == 0 {
		log.Fatal("missing folder flag")
	}

	err := task.Start(context.Background(), time.Second*10, p.run)
	if err != nil {
		log.Fatal(err)
	}
}

// P is the main program type.
type P struct {
	URL         string
	DownloadTo  string
	DownloadExt []string
}

func get(ctx context.Context, rel, at string, w io.Writer) error {
	if len(rel) > 0 {
		switch {
		default:
			if strings.HasSuffix(rel, "/") {
				at = rel + at
			} else {
				at = rel + "/" + at
			}
		case strings.HasPrefix(at, "http://"), strings.HasPrefix(at, "https://"):
			// Nothing.
		}
	}
	req, err := http.NewRequest("GET", at, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to get %q: %v", at, resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

var safeFilename = strings.NewReplacer(
	"/", "_",
	"<", "_",
	">", "_",
	":", "_",
	"\"", "_",
	"\\", "_",
	"|", "_",
	"?", "_",
	"*", "_",
)

func getTo(ctx context.Context, rel, at, saveFolder string) error {
	_, filename := path.Split(at)
	filename, err := url.PathUnescape(filename)
	if err != nil {
		return err
	}
	filename = safeFilename.Replace(filename)
	savePath := filepath.Join(saveFolder, filename)
	f, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer f.Close()

	bf := bufio.NewWriter(f)
	defer bf.Flush()

	return get(ctx, rel, at, bf)
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func (p *P) fix(ctx context.Context) error {
	dir, err := os.Open(p.DownloadTo)
	if err != nil {
		return err
	}
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if strings.Contains(name, "%") {
			next, err := url.PathUnescape(name)
			if err != nil {
				return err
			}
			next = safeFilename.Replace(next)
			fmt.Printf("fix %q -> %q\n", name, next)
			err = os.Rename(filepath.Join(p.DownloadTo, name), filepath.Join(p.DownloadTo, next))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *P) run(ctx context.Context) error {
	buf := &bytes.Buffer{}
	err := get(ctx, "", p.URL, buf)
	if err != nil {
		return err
	}
	root, err := html.Parse(buf)
	if err != nil {
		return err
	}

	sel, err := cascadia.Compile("a[href]")
	if err != nil {
		return err
	}

	for _, node := range sel.MatchAll(root) {
		href := attr(node, "href")
		hrefLower := strings.ToLower(href)
		for _, ck := range p.DownloadExt {
			if strings.HasSuffix(hrefLower, ck) {
				err := getTo(ctx, p.URL, href, p.DownloadTo)
				if err != nil {
					return err
				}
				break
			}
		}
	}

	return nil
}
