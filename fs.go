package gitfs
import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	_ fs.FS          = &Tree{}
	_ fs.FileInfo    = &Object{}
	_ fs.DirEntry    = &Object{}
	_ fs.File        = &Object{}
	_ fs.ReadDirFile = &Object{}
)

func NewFS(repo *git.Repository, rev string) (fs.FS, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, err
	}

	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	return &Tree{
		tree: *tree,
		repo: repo,
		hash: *hash,
	}, nil
}

type Tree struct {
	hash plumbing.Hash
	repo *git.Repository
	tree object.Tree
}

func (tree *Tree) Open(name string) (ret fs.File, err error) {
	defer func() {
		if err != nil {
			log.Printf("Open(%s): %v", name, err)
		}
	}()


	var (
		mode filemode.FileMode
		hash plumbing.Hash
	)

	if path.Clean(name) == "." {
		mode = filemode.Dir
		hash = tree.hash
	} else {
		f, err := tree.tree.FindEntry(name)
		if err != nil {
			if errors.Is(err, object.ErrEntryNotFound) ||
				errors.Is(err, object.ErrFileNotFound) {
				return nil, fs.ErrNotExist
			}

			return nil, err
		}
		mode = f.Mode
		hash = f.Hash
	}

	return NewFile(tree.hash,
		hash,
		tree.repo,
		path.Clean(name),
		mode)
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

// monolithic object, works as a File, DirEntry, and FileInfo
// it can be constructed as FileInfo or as File, where the
// former uses less resources
type Object struct {
	commit   plumbing.Hash
	hash     plumbing.Hash
	repo     *git.Repository
	fullName string
	mode     fs.FileMode

	size    int64
	isDir   bool
	modTime time.Time

	r  io.ReadCloser
	te []object.TreeEntry
}

func NewFileInfo(
	commit plumbing.Hash,
	hash plumbing.Hash,
	repo *git.Repository,
	fullName string,
	mode filemode.FileMode) (*Object, error) {

	var (
		size  int64
		isDir bool
	)

	osMode, err := mode.ToOSFileMode()
	if err != nil {
		return nil, err
	}

	obj, err := object.GetObject(repo.Storer, hash)
	if err != nil {
		return nil, err
	}

	switch v := obj.(type) {
	case *object.Tree:
		size = 0
		isDir = true
	case *object.Commit:
		size = 0
		isDir = true
	case *object.Blob:
		size = v.Size
		isDir = false
	default:
		return nil, fmt.Errorf("cannot get file info from %T", obj)
	}

	logOpt := git.LogOptions{
		From: commit,
	}

	if isDir {
		if fullName != "." {
			logOpt.PathFilter = func(s string) bool {
				return strings.HasPrefix(s, fullName)
			}
		}
	} else {
		logOpt.FileName = &fullName
	}

	it, err := repo.Log(&logOpt)
	if err != nil {
		return nil, err
	}

	c, err := it.Next()
	it.Close()
	if err != nil {
		return nil, err
	}

	return &Object{
		commit:   commit,
		hash:     hash,
		repo:     repo,
		fullName: fullName,

		mode:    osMode,
		modTime: c.Author.When,
		size:    size,
		isDir:   isDir,

		r:  nil,
		te: nil,
	}, nil
}

func NewFile(
	commit plumbing.Hash,
	hash plumbing.Hash,
	repo *git.Repository,
	fullName string,
	mode filemode.FileMode) (*Object, error) {

	var (
		size  int64
		isDir bool
		r     io.ReadCloser
		te    []object.TreeEntry
	)

	osMode, err := mode.ToOSFileMode()
	if err != nil {
		return nil, err
	}

	obj, err := object.GetObject(repo.Storer, hash)
	if err != nil {
		return nil, err
	}

	switch v := obj.(type) {
	case *object.Tree:
		size = 0
		isDir = true
		te = v.Entries

	case *object.Commit:
		size = 0
		isDir = true
		tree, err := v.Tree()
		if err != nil {
			return nil, err
		}
		te = tree.Entries
	case *object.Blob:
		size = v.Size
		isDir = false
		r, err = v.Reader()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("cannot get file info from %T", obj)
	}

	logOpt := git.LogOptions{
		From: commit,
	}

	if isDir {
		if fullName != "." {
			logOpt.PathFilter = func(s string) bool {
				return strings.HasPrefix(s, fullName)
			}
		}
	} else {
		logOpt.FileName = &fullName
	}

	it, err := repo.Log(&logOpt)
	if err != nil {
		return nil, err
	}

	c, err := it.Next()
	it.Close()
	if err != nil {
		return nil, err
	}

	return &Object{
		commit:   commit,
		hash:     hash,
		repo:     repo,
		fullName: fullName,

		mode:    osMode,
		modTime: c.Author.When,
		size:    size,
		isDir:   isDir,

		r:  r,
		te: te,
	}, nil
}

func (o *Object) Name() string {
	return path.Base(o.fullName)
}

func (o *Object) Size() int64 {
	return o.size
}

func (o *Object) Mode() fs.FileMode {
	return o.mode
}

func (o *Object) Type() fs.FileMode {
	return o.mode & fs.ModeType
}

func (o *Object) ModTime() time.Time {
	return o.modTime
}

func (o *Object) IsDir() bool {
	return o.isDir
}

func (o *Object) Sys() interface{} {
	return nil
}

func (o *Object) Info() (fs.FileInfo, error) {
	return o, nil
}

func (o *Object) ReadDir(n int) ([]fs.DirEntry, error) {
	if !o.isDir {
		return nil, fs.ErrPermission
	}

	if len(o.te) == 0 {
		return nil, io.EOF
	}

	if n < 0 || len(o.te) < n {
		n = len(o.te)
	}

	te := o.te[:n]
	o.te = o.te[n:]

	ret := make([]fs.DirEntry, n)

	var err error

	for i, v := range te {
		ret[i], err = NewFileInfo(
			o.commit,
			v.Hash,
			o.repo,
			path.Join(o.fullName, v.Name),
			v.Mode)
		if err != nil {
			return nil, err
		}
	}

	return ret, nil

}

func (o *Object) Read(b []byte) (int, error) {
	if o.isDir {
		return 0, fs.ErrPermission
	}

	return o.r.Read(b)
}

func (o *Object) Close() error {
	if o.isDir {
		o.te = nil
		return nil
	}

	return o.r.Close()
}

func (o *Object) Stat() (fs.FileInfo, error) {
	return o, nil
}
