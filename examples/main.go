package main

import (
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	gitfs "github.com/ear7h/go-git-fs"
	"github.com/go-git/go-git/v5"
)

func main() {
	dir := os.Args[1]

	repo, err := git.PlainOpen(dir)
	if err != nil {
		panic(err)
	}

	mime.AddExtensionType(".md", "text/plain")
	mime.AddExtensionType(".go", "text/plain")

	http.HandleFunc("/tree/", func(w http.ResponseWriter, r *http.Request) {
		// /tree/{rev}/{path}

		p := r.URL.Path[len("/tree/"):]
		arr := strings.SplitN(p, "/", 2)

		log.Println(arr)

		fs, err := gitfs.NewFS(repo, arr[0])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		r.URL.Path = arr[1]

		http.FileServer(http.FS(fs)).ServeHTTP(w, r)
	})

	err = http.ListenAndServe(":8080", nil)

	fmt.Println(err)
}
