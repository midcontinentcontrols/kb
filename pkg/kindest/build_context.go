package kindest

import (
	"crypto/md5"
	"encoding/hex"
	"hash"
)

type BuildContext map[string]interface{}

func hashContext(context map[string]interface{}, h hash.Hash, prefix string) error {
	for k, v := range context {
		k = prefix + k
		if file, ok := v.(*File); ok {
			if _, err := h.Write([]byte(k)); err != nil {
				return err
			}
			if _, err := h.Write([]byte{
				byte((file.Permissions >> 24) & 0xFF),
				byte((file.Permissions >> 16) & 0xFF),
				byte((file.Permissions >> 8) & 0xFF),
				byte((file.Permissions >> 0) & 0xFF),
			}); err != nil {
				return err
			}
			if _, err := h.Write(file.Content); err != nil {
				return err
			}
		} else if dir, ok := v.(*Directory); ok {
			k = k + "?" // impossible to use in most filesystems
			if _, err := h.Write([]byte(k)); err != nil {
				return err
			}
			if _, err := h.Write([]byte{
				byte((dir.Permissions >> 24) & 0xFF),
				byte((dir.Permissions >> 16) & 0xFF),
				byte((dir.Permissions >> 8) & 0xFF),
				byte((dir.Permissions >> 0) & 0xFF),
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

func (c BuildContext) Digest() (string, error) {
	h := md5.New()
	if err := hashContext(map[string]interface{}(c), h, ""); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (c BuildContext) Archive() ([]byte, error) {
	return nil, nil
}

type File struct {
	Permissions int
	Content     []byte
}

type Directory struct {
	Permissions int
	Contents    map[string]interface{}
}
