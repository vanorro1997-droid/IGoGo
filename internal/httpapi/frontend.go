package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func makeFrontendHandler(frontendDir string) http.Handler {
	files := http.FileServer(http.Dir(frontendDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, segment := range strings.Split(r.URL.Path, "/") {
			if segment == ".." {
				http.NotFound(w, r)
				return
			}
		}
		cleanPath := path.Clean("/" + r.URL.Path)
		filePath := filepath.Join(frontendDir, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		// Сохраняем SPA-маршруты, но не подменяем отсутствующие файлы страницей.
		if path.Ext(cleanPath) == "" {
			http.ServeFile(w, r, filepath.Join(frontendDir, "index.html"))
			return
		}
		http.NotFound(w, r)
	})
}
