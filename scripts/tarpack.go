package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "", "archive root")
	output := flag.String("output", "", "archive path")
	flag.Parse()
	if *root == "" || *output == "" || len(flag.Args()) == 0 {
		panic("root, output, and at least one entry are required")
	}

	file, err := os.Create(*output)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for _, entry := range flag.Args() {
		path := filepath.Join(*root, filepath.FromSlash(entry))
		if err := addPath(tarWriter, *root, path); err != nil {
			panic(err)
		}
	}
}

func addPath(writer *tar.Writer, root, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	name := filepath.ToSlash(relative)
	if info.IsDir() {
		name += "/"
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	header.ModTime = time.Unix(0, 0).UTC()
	if info.IsDir() {
		header.Mode = 0o755
	} else if executable(name) {
		header.Mode = 0o755
	} else {
		header.Mode = 0o644
	}
	if err := writer.WriteHeader(header); err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := addPath(writer, root, filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	_, err = io.Copy(writer, input)
	return err
}

func executable(name string) bool {
	base := filepath.Base(filepath.FromSlash(name))
	return base == "prometheus" || base == "promtool" || base == "prometheus-manager" || strings.Contains(name, "/cmd/") || strings.HasPrefix(name, "cmd/")
}
