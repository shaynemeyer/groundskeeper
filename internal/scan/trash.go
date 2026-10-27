package scan

import (
	"path/filepath"
	"slices"
	"strings"
)

// CacheTrashable reports whether path is a cache row the Caches scanner
// marks for the Trash: a known cache, a folder in ~/Library/Caches, or a
// folder beside a browser's in a vendor folder such as Caches/Google.
// Browser folders, and folders holding one, are never trashable.
func CacheTrashable(home, path string) bool {
	if isKnownCache(home, path) {
		return true
	}
	caches := filepath.Join(home, "Library", "Caches")
	browsers := browserCacheRoots(filepath.Join(home, "Library"))
	if slices.Contains(browsers, path) || holdsBrowser(path, browsers) {
		return false
	}
	parent := filepath.Dir(path)
	return parent == caches || (filepath.Dir(parent) == caches && holdsBrowser(parent, browsers))
}

// ArtifactTrashable reports whether path is an artifact folder the Build
// artifacts scanner marks for the Trash. __pycache__ folders are totalled
// per project, so they never are.
func ArtifactTrashable(path string) bool {
	kind := artifactKind(path, filepath.Base(path))
	return kind != "" && kind != pycache
}

// HFTrashable reports whether a Hugging Face hub repo folder may be moved
// to the Trash: a models--* or datasets--* repo that links nothing into
// the shared blob store, since trashing it would leave those files behind.
func HFTrashable(hub, repo string) bool {
	name := filepath.Base(repo)
	kind := strings.HasPrefix(name, "models--") || strings.HasPrefix(name, "datasets--")
	return kind && filepath.Dir(repo) == hub && hfSharedBytes(hub, repo) == 0
}
