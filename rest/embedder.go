package rest

import (
	"io"
	"net/http"
	"strconv"

	"bitbucket.org/kleinnic74/photos/embed"
)

func Embedder() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "" || path == "/" {
			path = "/index.html"
		}
		res, err := embed.GetResource(path)
		if err != nil {
			compact.WithError(w, http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", res.Type)
		w.Header().Set("Content-Length", strconv.Itoa(res.Size()))
		w.WriteHeader(http.StatusOK)
		io.Copy(w, res.Open())
	})
}
