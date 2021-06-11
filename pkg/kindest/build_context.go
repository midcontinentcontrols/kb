package kindest

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	gogitignore "github.com/sabhiram/go-gitignore"
)

type BuildContext map[string]Entity

func hashContext(
	context map[string]Entity,
	h hash.Hash,
	prefix string,
) error {
	keys := make([]string, len(context))
	i := 0
	for k := range context {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := context[k]
		k = prefix + k
		if file, ok := v.(*File); ok {
			if _, err := h.Write([]byte(k)); err != nil {
				return err
			}
			mode := file.Info().Mode()
			if _, err := h.Write([]byte{
				byte((mode >> 24) & 0xFF),
				byte((mode >> 16) & 0xFF),
				byte((mode >> 8) & 0xFF),
				byte((mode >> 0) & 0xFF),
			}); err != nil {
				return err
			}
			if _, err := h.Write(file.Content); err != nil {
				return err
			}
		} else if dir, ok := v.(*Directory); ok {
			if _, err := h.Write([]byte(k + "?")); err != nil {
				return err
			}
			mode := dir.Info().Mode()
			if _, err := h.Write([]byte{
				byte((mode >> 24) & 0xFF),
				byte((mode >> 16) & 0xFF),
				byte((mode >> 8) & 0xFF),
				byte((mode >> 0) & 0xFF),
			}); err != nil {
				return err
			}
			if err := hashContext(dir.Contents, h, k); err != nil {
				return err
			}
		} else {
			panic("unreachable")
		}
	}
	return nil
}

func (c BuildContext) Digest(include *gogitignore.GitIgnore) (string, error) {
	h := md5.New()
	if err := hashContext(map[string]Entity(c), h, ""); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func addToArchive(
	archive *archivex.TarFile,
	c map[string]Entity,
	prefix string,
) error {
	for k, v := range c {
		header, err := tar.FileInfoHeader(v.Info(), "")
		if err != nil {
			return err
		}
		header.Name = prefix + k
		if d, ok := v.(*Directory); ok {
			header.Name += "/"
			if err := archive.Writer.WriteHeader(header); err != nil {
				return err
			}
			if err := addToArchive(archive, d.Contents, header.Name); err != nil {
				return err
			}
		} else if f, ok := v.(*File); ok {
			if err := archive.Writer.WriteHeader(header); err != nil {
				return err
			}
			_, err := io.Copy(archive.Writer, bytes.NewReader(f.Content))
			if err != nil {
				return err
			}
		} else {
			panic("unreachable branch detected")
		}
	}
	return nil
}

func getTempDir() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	tmpDir := filepath.Join(u.HomeDir, ".kindest", "tmp")
	if err := os.MkdirAll(tmpDir, 0766); err != nil {
		panic(err)
	}
	return tmpDir
}

func (c BuildContext) Archive() ([]byte, error) {
	tarPath := filepath.Join(getTempDir(), fmt.Sprintf("build-context-%s.tar", uuid.New().String()))
	archive := new(archivex.TarFile)
	archive.Create(tarPath)
	defer os.Remove(tarPath)
	if err := addToArchive(archive, map[string]Entity(c), ""); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(tarPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type File struct {
	Content []byte
	info    os.FileInfo
}

func (f *File) Info() os.FileInfo {
	return f.info
}

type Directory struct {
	Contents map[string]Entity
	info     os.FileInfo
}

func (d *Directory) Info() os.FileInfo {
	return d.info
}

type Entity interface {
	Info() os.FileInfo
}
