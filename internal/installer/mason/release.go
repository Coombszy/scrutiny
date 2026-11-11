package mason

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// fetchRelease downloads the package's GitHub release asset into dir,
// unpacking archives. An asset file of the form "name:dest/" unpacks into
// dest within dir.
func (i *Installer) fetchRelease(ctx context.Context, p plan, dir string) error {
	file, dest, _ := strings.Cut(p.asset.File, ":")
	u := fmt.Sprintf("%s/%s/%s/releases/download/%s/%s", i.opts.DownloadBase, p.purl.Namespace, p.purl.Name, p.purl.Version, file)
	body, err := i.get(ctx, u)
	if err != nil {
		return err
	}
	into := filepath.Join(dir, filepath.FromSlash(dest))
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	switch name := path.Base(file); {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		return untar(zr, into)
	case strings.HasSuffix(name, ".tar.xz"), strings.HasSuffix(name, ".tar.bz2"), strings.HasSuffix(name, ".tar"):
		// No xz or bzip2 in the standard library: let tar decompress.
		archive := filepath.Join(dir, ".download-"+name)
		if err := os.WriteFile(archive, body, 0o644); err != nil {
			return err
		}
		defer os.Remove(archive)
		return i.opts.Run(ctx, into, commandEnv(), "tar", "-xf", archive)
	case strings.HasSuffix(name, ".zip"):
		return unzip(body, into)
	case strings.HasSuffix(name, ".gz"):
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(into, strings.TrimSuffix(name, ".gz")), zr, 0o755)
	default:
		return writeFile(filepath.Join(into, name), bytes.NewReader(body), 0o755)
	}
}

// within resolves name inside dir, refusing entries that would escape it,
// directly or through a symlink already extracted on the way there.
func within(dir, name string) (string, error) {
	p := filepath.Join(dir, filepath.FromSlash(name))
	if !inside(dir, p) {
		return "", fmt.Errorf("archive entry %q escapes the package", name)
	}
	for q := p; q != dir; q = filepath.Dir(q) {
		if fi, err := os.Lstat(q); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("archive entry %q goes through a symlink", name)
		}
	}
	return p, nil
}

func inside(dir, p string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// maxDownload bounds a release asset or registry download.
const maxDownload = 1 << 30

func untar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := within(dir, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(p, tr, os.FileMode(h.Mode)&0o777|0o600); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if path.IsAbs(h.Linkname) || filepath.IsAbs(h.Linkname) ||
				!inside(dir, filepath.Join(filepath.Dir(p), filepath.FromSlash(h.Linkname))) {
				return fmt.Errorf("archive link %q -> %q escapes the package", h.Name, h.Linkname)
			}
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

func unzip(body []byte, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		p, err := within(dir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(p, rc, f.Mode()&0o777|0o600)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(p string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
