package fdd

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)
/*
"Truth is not born pure from the earth; it requires refinement from the superfluous to shine in its essence."
— Ancient Wisdom

"Divide tasks to conquer complexity."
— Modern Wisdom
*/
type worker interface {
	run(inp, out chan *task, checker checker)
}

// For coordination works of fetcher-pool in recursive mode
type dispatcher struct {
	done chan struct{} // To send signal about "all folders are processed" - close this chan
	stop chan struct{} // To send signal about "all fetchers are stopped" - close this chan
	inc  chan int      // Chan for count unprocessed folders
	rec  chan *task    // Input chan for recursive tasks (tasks with folders)
}

func newDispatcher(rec chan *task) *dispatcher {
	dd := &dispatcher{}
	dd.inc = make(chan int)
	dd.done = make(chan struct{})
	dd.stop = make(chan struct{})
	dd.rec = rec
	return dd
}

// Sending first task for processing
func (dd *dispatcher) start(rootPath string) {
	dd.inc <- 1
	dd.rec <- newTask(key{}, info{path: rootPath})
}

// Wait until all folders are processed
func (dd *dispatcher) waitQueueDone() {
	var done int64
	for i := range dd.inc {
		done += int64(i)
		if done == 0 {
			close(dd.done)
			return
		}
	}
}

// Wait until it is possible to close the channel "rec"
func (dd *dispatcher) waitFetcherStopped() {
	select {
	case <-dd.done:
	case <-dd.stop:
	}
	close(dd.rec)
}

func fileGenerator(rootPath string, amt int, rec, inp chan *task, item *searchEngine) chan *task {
	out := make(chan *task)

	dispather := newDispatcher(rec)
	go dispather.start(rootPath)
	go dispather.waitQueueDone()
	go dispather.waitFetcherStopped()

	var workers sync.WaitGroup
	item.poolCount.Add(1)
	for range amt {
		workers.Add(1)
		go func() {
			defer workers.Done()
			(&fetcher{}).run(inp, out, dispather.rec, dispather.inc)
		}()
	}
	slog.Debug("Worker-pool - started.", slog.String("workerType", fmt.Sprintf("%T", fetcher{})))

	go func() {
		workers.Wait()
		close(out)
		close(dispather.inc)
		close(dispather.stop)
		item.poolCount.Done()

		slog.Debug("Worker-pool - stoped.", slog.String("workerType", fmt.Sprintf("%T", fetcher{})))
	}()
	return out
}

func runPool(runWorker worker, amt int, inp chan *task, checker checker, item *searchEngine) chan *task {
	out := make(chan *task)

	var workers sync.WaitGroup
	item.poolCount.Add(1)
	for range amt {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runWorker.run(inp, out, checker)
		}()
	}
	slog.Debug("Worker-pool - started.", slog.String("workerType", fmt.Sprintf("%T", runWorker)))

	go func() {
		workers.Wait()
		close(out)
		item.poolCount.Done()

		slog.Debug("Worker-pool - stoped.", slog.String("workerType", fmt.Sprintf("%T", runWorker)))
	}()
	return out
}

type fetcher struct{}

func (item *fetcher) run(inp, out, rec chan *task, inc chan int) {
	for currentTask := range inp {
		func() {
			defer func() { inc <- -1 }()

			objects, err := readDir(currentTask.info.path) // custom's changed os.ReadDir
			if checkError(err, "Objects read error.", "readDir()", item, currentTask, nil) {
				return
			}

			for _, object := range objects {
				objectPath := filepath.Join(currentTask.path, object.Name())

				if object.IsDir() {
					inc <- 1
					rec <- newTask(key{}, info{path: objectPath})
					continue
				}

				objectInfo, err := object.Info()
				if checkError(err, "Object-info read error.", "object.Info()", item, currentTask, nil) {
					continue
				}

				out <- newTask(key{size: objectInfo.Size()}, info{path: objectPath})
			}
		}()
	}
}

type sizer struct{}

func (item *sizer) run(inp, out chan *task, checker checker) {
	for currentTask := range inp {
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

func (item *hasher) run(inp, out chan *task, checker checker) {
	buf := make([]byte, 512)

	for inpTask := range inp {
		func(currentTask *task) {
			file, err := os.Open(currentTask.path)
			if checkError(err, "File open error.", "os.Open()", item, currentTask, nil) {
				return
			}
			defer func(f *os.File) {
				closeErr := f.Close()
				checkError(closeErr, "File close error.", "file.Close()", item, currentTask, nil)
			}(file)

			n, err := file.Read(buf) // TODO: check - path="C:\\Users\\All Users"
			if checkError(err, "File read error.", "file.Read()", item, currentTask, nil) {
				return // file will be ignored, if size=0 or Read returns a non-EOF error
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

func (item *matcher) run(inp, out chan *task, checker checker) {
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
				if checkError(err, "File1 open error.", "os.Open()", item, currentTask, nil) {
					break
				}
				defer func() {
					if file1 != nil {
						closeErr := file1.Close()
						checkError(closeErr, "File1 close error.", "file1.Close()", item, currentTask, nil)
					}
				}()

				file2, err := os.Open(reviewedTask.path)
				if checkError(err, "File2 open error.", "os.Open()", item, reviewedTask, nil) {
					currentTask.key.equal++
					file1.Seek(0, io.SeekStart)
					continue
				}
				defer func() {
					if file2 != nil {
						closeErr := file2.Close()
						checkError(closeErr, "File2 close error.", "file2.Close()", item, reviewedTask, nil)
					}
				}()

				filesEqual, checkErr := checkEqual(file1, file2, buf1, buf2)
				checkError(checkErr, "Check equal error (file1.Read() or file2.Read()).", "checkEqual()", item, currentTask, reviewedTask)

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

func checkError(err error, msg, method string, item any, task1, task2 *task) bool {
	if err != nil && err != io.EOF {
		if task2 == nil {
			slog.Info(msg,
				slog.String("item", fmt.Sprintf("%T", item)),
				slog.String("method", method),
				slog.String("error", err.Error()),
				slog.String("path", task1.path))
			return true
		}
		slog.Info(msg,
			slog.String("item", fmt.Sprintf("%T", item)),
			slog.String("method", method),
			slog.String("error", err.Error()),
			slog.String("path-1", task1.path),
			slog.String("path-2", task2.path))
		return true
	}
	return false
}
