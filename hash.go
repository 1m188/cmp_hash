package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var errDiff = errors.New("content differs")

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func walkFiles(root string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = path
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

// CompareFiles returns true if two files have identical content.
func CompareFiles(path1, path2 string) (bool, error) {
	h1, err := fileHash(path1)
	if err != nil {
		return false, err
	}
	h2, err := fileHash(path2)
	if err != nil {
		return false, err
	}
	return h1 == h2, nil
}

// CompareDirs returns true if two directories have identical content —
// same file tree, same file contents.
func CompareDirs(dir1, dir2 string) (bool, error) {
	files1, err := walkFiles(dir1)
	if err != nil {
		return false, err
	}
	files2, err := walkFiles(dir2)
	if err != nil {
		return false, err
	}
	if len(files1) != len(files2) {
		return false, nil
	}

	type pair struct{ a, b string }
	pairs := make([]pair, 0, len(files1))
	for rel, abs1 := range files1 {
		abs2, ok := files2[rel]
		if !ok {
			return false, nil
		}
		pairs = append(pairs, pair{abs1, abs2})
	}
	if len(pairs) == 0 {
		return true, nil
	}

	n := runtime.NumCPU()
	if n > len(pairs) {
		n = len(pairs)
	}
	work := make(chan pair, len(pairs))
	result := make(chan error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			for p := range work {
				h1, err := fileHash(p.a)
				if err != nil {
					result <- err
					return
				}
				h2, err := fileHash(p.b)
				if err != nil {
					result <- err
					return
				}
				if h1 != h2 {
					result <- errDiff
					return
				}
			}
		}()
	}

	for _, p := range pairs {
		work <- p
	}
	close(work)
	wg.Wait()
	close(result)

	if err, ok := <-result; ok {
		if errors.Is(err, errDiff) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
