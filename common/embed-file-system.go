package common

import (
	"bytes"
	"embed"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/static"
)

// Credit: https://github.com/gin-contrib/static/issues/19

var (
	indexTitlePattern     = []byte("<title>New API</title>")
	indexMetaTitlePattern = []byte(`<meta name="title" content="New API" />`)
	indexDescriptionPattern = []byte(`content="Unified AI API gateway and admin dashboard."`)
	// Injected by InjectUmamiAnalytics/InjectGoogleAnalytics at startup; they
	// carry the organization identity into every served page, so strip them
	// from the wire bytes (the source strings in main.go stay untouched).
	indexUmamiCommentPattern = []byte("<!--Umami QuantumNous-->")
	indexGACommentPattern    = []byte("<!--Google Analytics QuantumNous-->")
)

// RenderIndexPage substitutes the configured system name into the embedded
// index.html at serve time. The static template hardcodes the default brand,
// and asset scanners never execute the frontend's client-side title update,
// so the replacement must happen before the bytes leave the server.
func RenderIndexPage(page []byte) []byte {
	name := html.EscapeString(SystemName)
	page = bytes.ReplaceAll(page, indexTitlePattern, []byte("<title>"+name+"</title>"))
	page = bytes.ReplaceAll(page, indexMetaTitlePattern, []byte(`<meta name="title" content="`+name+`" />`))
	page = bytes.ReplaceAll(page, indexDescriptionPattern, []byte(`content="`+name+`"`))
	page = bytes.ReplaceAll(page, indexUmamiCommentPattern, nil)
	page = bytes.ReplaceAll(page, indexGACommentPattern, nil)
	return page
}

// inMemoryFile serves replaced index.html bytes through the http.File interface.
type inMemoryFile struct {
	*bytes.Reader
	name    string
	modTime time.Time
}

func (f *inMemoryFile) Close() error { return nil }

func (f *inMemoryFile) Readdir(count int) ([]fs.FileInfo, error) { return nil, nil }

func (f *inMemoryFile) Stat() (fs.FileInfo, error) {
	return &inMemoryFileInfo{name: f.name, size: f.Size(), modTime: f.modTime}, nil
}

type inMemoryFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i *inMemoryFileInfo) Name() string       { return i.name }
func (i *inMemoryFileInfo) Size() int64        { return i.size }
func (i *inMemoryFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i *inMemoryFileInfo) ModTime() time.Time { return i.modTime }
func (i *inMemoryFileInfo) IsDir() bool        { return false }
func (i *inMemoryFileInfo) Sys() interface{}   { return nil }

type embedFileSystem struct {
	http.FileSystem
}

func (e *embedFileSystem) Exists(prefix string, path string) bool {
	_, err := e.Open(path)
	if err != nil {
		return false
	}
	return true
}

func (e *embedFileSystem) Open(name string) (http.File, error) {
	if name == "/" {
		// This will make sure the index page goes to NoRouter handler,
		// which will use the replaced index bytes with analytic codes.
		return nil, os.ErrNotExist
	}
	if name == "/index.html" {
		// Serve the system-name-rendered copy for direct /index.html hits too,
		// otherwise the raw template would leak the default brand.
		file, err := e.FileSystem.Open(name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		return &inMemoryFile{
			Reader:  bytes.NewReader(RenderIndexPage(data)),
			name:    "index.html",
			modTime: time.Now(),
		}, nil
	}
	return e.FileSystem.Open(name)
}

func EmbedFolder(fsEmbed embed.FS, targetPath string) static.ServeFileSystem {
	efs, err := fs.Sub(fsEmbed, targetPath)
	if err != nil {
		panic(err)
	}
	return &embedFileSystem{
		FileSystem: http.FS(efs),
	}
}
