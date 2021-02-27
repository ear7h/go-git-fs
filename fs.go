package gitfs

import (
	"log"
	"io"
	"io/fs"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var _ fs.FS = &Tree{}

func NewFS(repo *git.Repository, rev string) (fs.FS, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, err
	}

	log.Println("hash: ", hash)

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	for _, v := range tree.Entries {
		log.Println(v.Name)
	}

	return &Tree{
		tree: *tree,
		repo: repo,
		hash: *hash,
	}, nil
}

type Tree struct {
	tree object.Tree
	repo *git.Repository
	hash plumbing.Hash
}

func (tree *Tree) Open(name string) (ret fs.File, err error) {
	defer func() {
		if err != nil {
			log.Printf("Open(%s): %v", name, err)
		}
	}()

	f, err := tree.tree.File(name)
	if err != nil {
		return nil, err
	}


	var r io.ReadCloser

	if f.Mode.IsFile() {
		r, err = f.Reader()
		if err != nil {
			return nil, err
		}
	}


	var fsMode fs.FileMode
	fsMode, err = f.Mode.ToOSFileMode()
	if err != nil {
		return nil, err
	}

	it, err := tree.repo.Log(&git.LogOptions{
		From:     tree.hash,
		FileName: &name,
	})
	if err != nil {
		return nil, err
	}

	c, err := it.Next()
	it.Close()
	if err != nil {
		return nil, err
	}

	log.Println("got commit ", c)

	return &File{
		fi: FileInfo{
			name:    f.Name,
			size:    f.Size,
			mode:    fsMode,
			modTime: c.Committer.When,
			isDir:   fs.ModeDir&fsMode > 0,
		},
		r: r,
	}, nil
}

type FileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (fi *FileInfo) Name() string {
	return fi.name
}

func (fi *FileInfo) Size() int64 {
	return fi.size
}

func (fi *FileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi *FileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *FileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *FileInfo) Sys() interface{} {
	return nil
}

type File struct {
	fi FileInfo
	r  io.ReadCloser
}

func (f *File) Stat() (fs.FileInfo, error) {
	return &f.fi, nil
}

func (f *File) Read(byt []byte) (int, error) {
	return f.r.Read(byt)
}

func (f *File) Close() error {
	return f.r.Close()
}
