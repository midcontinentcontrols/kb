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
	"sync"

	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	gogitignore "github.com/sabhiram/go-gitignore"
)

type BuildContext map[string]Entity

type asyncHashResult struct {
	key  string
	hash []byte
	err  error
}

func asyncHashError(key string, err error) *asyncHashResult {
	return &asyncHashResult{
		key: key,
		err: err,
	}
}

// Files over this size will be hashed asynchronously.
const FILE_SYNC_HASH_LIMIT = 100 * 1000

// alphabetizeKeys returns a sorted slice of all keys in the given map.
func alphabetizeKeys(context map[string]Entity) []string {
	keys := make([]string, len(context))
	i := 0
	for k := range context {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

func hashFile(
	h hash.Hash,
	k string,
	file *File,
) error {
	// Append a unique character to the end of the key to ensure
	// that files and directories are hashed differently.
	if _, err := h.Write([]byte(k + "!")); err != nil {
		return err
	}
	// Hash the file's mode.
	mode := file.Info().Mode()
	if _, err := h.Write([]byte{
		byte((mode >> 24) & 0xFF),
		byte((mode >> 16) & 0xFF),
		byte((mode >> 8) & 0xFF),
		byte((mode >> 0) & 0xFF),
	}); err != nil {
		return err
	}
	// Hash the file's length.
	fileSize := len(file.Content)
	if _, err := h.Write([]byte{
		byte((fileSize >> 24) & 0xFF),
		byte((fileSize >> 16) & 0xFF),
		byte((fileSize >> 8) & 0xFF),
		byte((fileSize >> 0) & 0xFF),
	}); err != nil {
		return err
	}
	// Hash the file's content.
	if _, err := h.Write(file.Content); err != nil {
		return err
	}
	return nil
}

func hashDirectory(
	h hash.Hash,
	k string,
	dir *Directory,
) error {
	// Append a unique character to the end of the key to ensure
	// that files and directories are hashed differently.
	if _, err := h.Write([]byte(k + "?")); err != nil {
		return err
	}
	// Write the mode of the directory to the hash.
	mode := dir.Info().Mode()
	if _, err := h.Write([]byte{
		byte((mode >> 24) & 0xFF),
		byte((mode >> 16) & 0xFF),
		byte((mode >> 8) & 0xFF),
		byte((mode >> 0) & 0xFF),
	}); err != nil {
		return err
	}
	// Write the number of child entities to the hash.
	numFiles := len(dir.Contents)
	if _, err := h.Write([]byte{
		byte((numFiles >> 24) & 0xFF),
		byte((numFiles >> 16) & 0xFF),
		byte((numFiles >> 8) & 0xFF),
		byte((numFiles >> 0) & 0xFF),
	}); err != nil {
		return err
	}
	// Hash the directory's contents.
	if err := hashContext(dir.Contents, h, k); err != nil {
		return err
	}
	return nil
}

func hashContext(
	context map[string]Entity,
	h hash.Hash,
	prefix string,
) error {
	// Sort all keys alphabetically. This is important to ensure
	// consistent hashing of the build context.
	keys := alphabetizeKeys(context)

	// Create a wait group to wait for all async hash threads.
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	// Keep track of all asynchronous hash results.
	var asyncHashes []<-chan *asyncHashResult

	// Accumulate all files & directories in the context.
	for _, k := range keys {
		v := context[k]
		k = prefix + k
		if file, ok := v.(*File); ok {
			if len(file.Content) < FILE_SYNC_HASH_LIMIT {
				if err := hashFile(h, k, file); err != nil {
					return fmt.Errorf("failed to hash file %s: %w", k, err)
				}
			} else {
				// Hash the large file asynchronously. This is to
				// improve performance for projects that have many
				// large files on machines with many cores.
				wg.Add(1)
				done := make(chan *asyncHashResult, 1)
				asyncHashes = append(asyncHashes, done)
				go func(k string, file *File, done chan<- *asyncHashResult) {
					defer wg.Done()
					done <- func() *asyncHashResult {
						h := md5.New()
						if err := hashFile(h, k, file); err != nil {
							return asyncHashError(k, err)
						}
						return &asyncHashResult{
							key:  k,
							hash: h.Sum([]byte{'!'}),
						}
					}()
				}(k, file, done)
			}
		} else if dir, ok := v.(*Directory); ok {
			if len(dir.Contents) < 2 {
				// Synchronously hash directories with less than two children.
				if err := hashDirectory(h, k, dir); err != nil {
					return fmt.Errorf("failed to hash directory %s: %w", k, err)
				}
			} else {
				// Hash multiple directory children asynchronously.
				// This is done for maximum performance. Error handling
				// here could be improved, but errors are so uncommon
				// with hashing that it's probably not worth the effort.
				wg.Add(1)
				done := make(chan *asyncHashResult, 1)
				asyncHashes = append(asyncHashes, done)
				go func(k string, dir *Directory, done chan<- *asyncHashResult) {
					defer wg.Done()
					done <- func() *asyncHashResult {
						h := md5.New()
						if err := hashDirectory(h, k, dir); err != nil {
							return asyncHashError(k, err)
						}
						return &asyncHashResult{
							key:  k,
							hash: h.Sum([]byte{'?'}),
						}
					}()
				}(k, dir, done)
			}
		} else {
			panic("unreachable branch: build context entity is neither a File nor a Directory")
		}
	}
	// Accumulate the hashes of the directories last.
	for _, dirHash := range asyncHashes {
		result := <-dirHash
		if result.err != nil {
			return fmt.Errorf("failed to hash directory %s: %w", result.key, result.err)
		}
		if _, err := h.Write(result.hash); err != nil {
			return err
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
