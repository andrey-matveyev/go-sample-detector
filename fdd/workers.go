package fdd

import (
	"bytes"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type worker interface {
	run(inp, out chan *task, checker checker)
}

type fetcher struct{
	outFolder chan *task
}

func (item fetcher) run(inp, out chan *task, checker checker) {
	for currentTask := range inp {
		func() {
			defer foldersCount.Done()

			objects, err := readDir(currentTask.info.path) // custom's changed os.ReadDir
			if err != nil {
				logger.Info("Objects read error.",
					slog.String("worker", "fetcher"),
					slog.String("method", "readDir()"),
					slog.String("error", err.Error()),
					slog.String("path", currentTask.path))
				return
			}

			for _, object := range objects {
				objectPath := filepath.Join(currentTask.path, object.Name())

				if object.IsDir() {
					foldersCount.Add(1)
					item.outFolder <- newTask(key{}, info{path: objectPath})
					continue
				}

				objectInfo, err := object.Info()
				if err != nil {
					logger.Info("Object-info read error.",
						slog.String("worker", "fetcher"),
						slog.String("method", "object.Info()"),
						slog.String("error", err.Error()),
						slog.String("path", currentTask.path))
					continue
				}
				out <- newTask(key{size: objectInfo.Size()}, info{path: objectPath})
			}
		}()
	}
}

type sizer struct{}

func (item sizer) run(inp, out chan *task, checker checker) {
	for currentTask := range inp {
/*
		if currentTask.size == 0 {
			continue
		}
*/
		checkedTask, detected := checker.verify(currentTask)
		if detected {
			out <- currentTask
			if checkedTask != nil {
				out <- checkedTask
			}
		}
	}
}

type hasher struct{}

func (item hasher) run(inp, out chan *task, checker checker) {
	buf := make([]byte, 512)

	for inpTask := range inp {
		func(currentTask *task) {
			file, err := os.Open(currentTask.path)
			if err != nil {
				logger.Info("File open error.",
					slog.String("worker", "hasher"),
					slog.String("method", "os.Open()"),
					slog.String("error", err.Error()),
					slog.String("path", currentTask.path))
				return
			}
			defer func(f *os.File) {
				closeErr := f.Close()
				if closeErr != nil {
					logger.Info("File close error.",
						slog.String("worker", "hasher"),
						slog.String("method", "file.Close()"),
						slog.String("error", closeErr.Error()),
						slog.String("path", currentTask.path))
				}
			}(file)

			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				logger.Info("File read error.", // TODO: check - path="C:\\Users\\All Users"
					slog.String("worker", "hasher"),
					slog.String("method", "file.Read()"),
					slog.String("error", err.Error()),
					slog.String("path", currentTask.path))
				return
			}

			currentTask.key.hash = crc32.ChecksumIEEE(buf[:n])
			checkedTask, detected := checker.verify(currentTask)
			if detected {
				out <- currentTask
				if checkedTask != nil {
					out <- checkedTask
				}
			}
		}(inpTask)
	}
}

type matcher struct{}

func (item matcher) run(inp, out chan *task, checker checker) {
	buf1 := make([]byte, 2*1024)
	buf2 := make([]byte, 2*1024)

	for inpTask := range inp {
		func(currentTask *task) {
			for {
				reviewedTask := checker.review(currentTask)

				if reviewedTask == nil {
					break
				}

				file1, err := os.Open(currentTask.path)
				if err != nil {
					logger.Info("File1 open error.",
						slog.String("worker", "matcher"),
						slog.String("method", "os.Open()"),
						slog.String("error", err.Error()),
						slog.String("path", currentTask.path))
					break
				}
				defer func() {
					if file1 != nil {
						closeErr := file1.Close()
						if closeErr != nil {
							logger.Info("File close error.",
								slog.String("worker", "matcher"),
								slog.String("method", "file1.Close()"),
								slog.String("error", closeErr.Error()),
								slog.String("path", currentTask.path))
						}
					}
				}()

				file2, err := os.Open(reviewedTask.path)
				if err != nil {
					logger.Info("File2 open error.",
						slog.String("worker", "matcher"),
						slog.String("method", "os.Open()"),
						slog.String("error", err.Error()),
						slog.String("path", reviewedTask.path))

					currentTask.key.equal++
					file1.Seek(0, io.SeekStart)
					continue
				}
				defer func() {
					if file2 != nil {
						closeErr := file2.Close()
						if closeErr != nil {
							logger.Info("File close error.",
								slog.String("worker", "matcher"),
								slog.String("method", "file2.Close()"),
								slog.String("error", closeErr.Error()),
								slog.String("path", reviewedTask.path))
						}
					}
				}()

				filesEqual, checkErr := checkEqual(file1, file2, buf1, buf2)
				if checkErr != nil {
					logger.Info("File close error.",
								slog.String("worker", "matcher"),
								slog.String("method", "checkEqual"),
								slog.String("error", checkErr.Error()),
								slog.String("file1", currentTask.path),
								slog.String("file2", reviewedTask.path))
				}

				if filesEqual {
					verifiedTask, detected := checker.verify(currentTask)
					if detected {
						out <- currentTask
						if verifiedTask != nil {
							out <- verifiedTask
						}
						break
					}
					currentTask.key.equal++
					file1.Seek(0, io.SeekStart)
					continue
				} else {
					currentTask.key.equal++
					file1.Seek(0, io.SeekStart)
					continue
				}
			}
		}(inpTask)
	}
}

// Byte-to-byte compare two files
func checkEqual(file1, file2 io.Reader, buf1, buf2 []byte) (bool, error) {
	for {
		n1, err1 := file1.Read(buf1)
		n2, err2 := file2.Read(buf2)

		if err1 == io.EOF && err2 == io.EOF {
			return true, nil
		}

		if err1 == io.EOF || err2 == io.EOF {
			return false, nil
		}

		if err1 != nil {
			return false, err1
		}
		if err2 != nil {
			return false, err2
		}

		if n1 != n2 {
			return false, nil
		}

		if !bytes.Equal(buf1[:n1], buf2[:n2]) {
			return false, nil
		}
	}
}

// Implementation os.ReadDir withot slices.SortFunc of entries
func readDir(name string) ([]os.DirEntry, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dirs, err := file.ReadDir(-1)
	return dirs, err
}
