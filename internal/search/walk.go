package search

import (
	"context"
	"io/fs"
	"path/filepath"
)

// ListFiles returns every non-ignored file under root as a slash-separated path
// relative to root. It applies the same ignore rules as Search, so the fuzzy
// finder and the content search see a consistent view of the tree. The result
// is capped by maxFiles to keep the finder responsive in giant repositories.
func ListFiles(ctx context.Context, root string, maxFiles int) ([]string, error) {
	ig := loadIgnore(root)
	files := make([]string, 0, 256)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if ig.match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ig.match(rel, false) {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if maxFiles > 0 && len(files) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && ctx.Err() != nil {
		return files, ctx.Err()
	}
	return files, nil
}
