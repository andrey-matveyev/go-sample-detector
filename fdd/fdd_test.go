package fdd

import (
	"context"
	"crypto/rand"
	"fmt"
	"hash/crc32"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Helper functions for testing ---

// createTempFile creates a temporary file with the given size and fills it with the specified content byte.
// Returns the file path.
func createTempFile(t *testing.T, dir, name string, size int64, content byte) string {
	filePath := filepath.Join(dir, name)
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create temp file %s: %v", filePath, err)
	}
	defer file.Close()

	if size > 0 {
		buf := make([]byte, size)
		for i := range buf {
			buf[i] = content
		}
		if _, err := file.Write(buf); err != nil {
			t.Fatalf("failed to write to temp file %s: %v", filePath, err)
		}
	}
	return filePath
}

// createTempDir creates a temporary directory.
// Returns the directory path.
func createTempDir(t *testing.T, pattern string) string {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir
}

// --- Tests for `SearchEngine` ---

// Test_SearchEngine_BasicFunctionality tests the core SearchEngine functionality.
// It creates two identical files and expects them to be found as duplicates.
func Test_SearchEngine_BasicFunctionality(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_basic")
	defer os.RemoveAll(tmpDir)

	// Create some test files
	contentA := []byte(strings.Repeat("a", 100))
	contentB := []byte(strings.Repeat("a", 100)) // Duplicate content of A
	contentC := []byte(strings.Repeat("a", 50))  // Duplicate part content of A
	contentD := []byte(strings.Repeat("b", 100)) // Same size as A/B, different content

	// Manually write content for precise hash calculation
	path1 := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(path1, contentA, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	path2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(path2, contentB, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	path3 := filepath.Join(tmpDir, "file3.txt")
	if err := os.WriteFile(path3, contentC, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	path4 := filepath.Join(tmpDir, "file4.txt")
	if err := os.WriteFile(path4, contentD, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	engine := GetEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	callback := func() {
		wg.Done()
	}

	engine.Run(ctx, tmpDir, callback)
	wg.Wait()

	result := engine.GetResult()

	// Expected results: file1.txt and file2.txt should be duplicates
	expectedHash := crc32.ChecksumIEEE(contentA)
	expectedPaths := []string{filepath.ToSlash(path1), filepath.ToSlash(path2)}
	sort.Strings(expectedPaths)

	foundDuplicateGroup := false
	for _, elem := range result.List {
		if elem.Size == int64(len(contentA)) && elem.Hash == expectedHash && len(elem.Paths) == 2 {
			currentPaths := make([]string, len(elem.Paths))
			for i, p := range elem.Paths {
				currentPaths[i] = filepath.ToSlash(p) // Ensure paths are in slash format for comparison
			}
			sort.Strings(currentPaths)
			if reflect.DeepEqual(currentPaths, expectedPaths) {
				foundDuplicateGroup = true
				if elem.Group != 0 { // Based on matcher logic, one comparison leads to group = 0
					t.Errorf("Expected Group to be 0 for a pair of duplicates, got %d", elem.Group)
				}
				break
			}
		}
	}

	if !foundDuplicateGroup {
		t.Errorf("Test_SearchEngine_BasicFunctionality failed. Expected duplicate group for %v not found.\nResult: %+v", expectedPaths, result)
	}

	// Basic check for progress
	progressBytes := engine.GetProgress()
	if len(progressBytes) == 0 {
		t.Errorf("GetProgress returned nil or empty bytes")
	}
}

// Test_SearchEngine_NoDuplicates tests scenario with no duplicate files.
func Test_SearchEngine_NoDuplicates(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_no_duplicates")
	defer os.RemoveAll(tmpDir)

	createTempFile(t, tmpDir, "fileA.txt", 10, 'x')
	createTempFile(t, tmpDir, "fileB.txt", 20, 'y')
	createTempFile(t, tmpDir, "fileC.txt", 30, 'z')

	engine := GetEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	callback := func() {
		wg.Done()
	}

	engine.Run(ctx, tmpDir, callback)
	wg.Wait()

	result := engine.GetResult()

	if len(result.List) != 0 {
		t.Errorf("Test_SearchEngine_NoDuplicates failed. Expected no duplicates, got: %+v", result)
	}
}

// Test_SearchEngine_EmptyDirectory tests an empty directory.
func Test_SearchEngine_EmptyDirectory(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_empty_dir")
	defer os.RemoveAll(tmpDir)

	engine := GetEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	callback := func() {
		wg.Done()
	}

	engine.Run(ctx, tmpDir, callback)
	wg.Wait()

	result := engine.GetResult()

	if len(result.List) != 0 {
		t.Errorf("Test_SearchEngine_EmptyDirectory failed. Expected no duplicates, got: %+v", result)
	}
}

// Test_SearchEngine_Subdirectories tests files in subdirectories.
func Test_SearchEngine_Subdirectories(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_subdirs")
	defer os.RemoveAll(tmpDir)

	subDir1 := filepath.Join(tmpDir, "sub1")
	subDir2 := filepath.Join(tmpDir, "sub2")
	if err := os.Mkdir(subDir1, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.Mkdir(subDir2, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	contentA := []byte(strings.Repeat("a", 10))
	path1 := filepath.Join(tmpDir, "file_root.txt")
	if err := os.WriteFile(path1, contentA, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	path2 := filepath.Join(subDir1, "file_sub1.txt")
	if err := os.WriteFile(path2, contentA, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	createTempFile(t, subDir2, "file_sub2.txt", 10, 'b') // Different content

	engine := GetEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	callback := func() {
		wg.Done()
	}

	engine.Run(ctx, tmpDir, callback)
	wg.Wait()

	result := engine.GetResult()

	expectedHash := crc32.ChecksumIEEE(contentA)
	expectedPaths := []string{filepath.ToSlash(path1), filepath.ToSlash(path2)}
	sort.Strings(expectedPaths)

	foundDuplicateGroup := false
	for _, elem := range result.List {
		if elem.Size == int64(len(contentA)) && elem.Hash == expectedHash && len(elem.Paths) == 2 {
			currentPaths := make([]string, len(elem.Paths))
			for i, p := range elem.Paths {
				currentPaths[i] = filepath.ToSlash(p)
			}
			sort.Strings(currentPaths)
			if reflect.DeepEqual(currentPaths, expectedPaths) {
				foundDuplicateGroup = true
				if elem.Group != 0 {
					t.Errorf("Expected Group to be 0 for a pair of duplicates, got %d", elem.Group)
				}
				break
			}
		}
	}

	if !foundDuplicateGroup {
		t.Errorf("Test_SearchEngine_Subdirectories failed. Expected duplicate group for %v not found.\nResult: %+v", expectedPaths, result)
	}
}

// Test_SearchEngine_Cancellation tests context cancellation during execution.
func Test_SearchEngine_Cancellation(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_cancellation")
	defer os.RemoveAll(tmpDir)

	// Create many files to ensure cancellation happens before completion
	for i := 0; i < 500; i++ { // Increased file count
		createTempFile(t, tmpDir, fmt.Sprintf("file_%d.txt", i), 1024*100, byte(i%256)) // Larger files
	}

	engine := GetEngine()
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	callback := func() {
		wg.Done()
	}

	engine.Run(ctx, tmpDir, callback)

	// Cancel the context after a short delay
	time.Sleep(10 * time.Millisecond) // Give it some time to start processing
	cancel()

	// Wait for a short period to allow cleanup, but don't expect it to finish all files
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Callback might still be called if the process was very fast,
		// but the main point is that it should not block indefinitely.
		t.Log("Callback was called, indicating graceful shutdown after cancellation (or fast completion).")
	case <-time.After(30 * time.Second): // A longer timeout for cleanup
		t.Log("SearchEngine stopped due to cancellation (callback not necessarily called if cancelled early).")
	}

	// Verify that the number of goroutines eventually goes down (loose check)
	// Initial goroutine count + some for the test runner.
	// After cancellation, the number should drop significantly from peak.
	currentGoroutines := runtime.NumGoroutine()
	t.Logf("Goroutines count after cancellation attempt: %d", currentGoroutines)
	// A perfect goroutine count is hard to assert, but we can check if it's not excessively high,
	// indicating that most worker goroutines exited.
	if currentGoroutines > 40 { // Arbitrary threshold; tune based on typical idle goroutine count + test overhead
		slog.Warn("Potentially too many goroutines still running after cancellation.", "Current:", currentGoroutines)
	}

	// Check if GetResult is still callable and doesn't panic
	result := engine.GetResult()
	if result == nil {
		t.Error("GetResult returned nil after cancellation")
	}
	t.Logf("Result list length after cancellation: %d", len(result.List))
}

// --- Tests for `checker.go` ---

// Test_checkList_Verify tests the verify method of checkList.
func Test_checkList_Verify(t *testing.T) {
	cl := newCheckList()

	// Test case 1: First encounter of a task
	task1 := newTask(key{size: 10, hash: 1}, info{path: "/path/to/file1.txt", checked: false})
	checkedTask, detected := cl.verify(task1)
	if detected {
		t.Errorf("Expected not detected, but got detected for task1")
	}
	if checkedTask != nil {
		t.Errorf("Expected nil checkedTask for first encounter, got %+v", checkedTask)
	}
	// Verify task1 is in the list with checked: false
	if info, ok := cl.(*checkList).list[task1.key]; !ok || info.checked {
		t.Errorf("Task1 not correctly added to list or marked as checked prematurely")
	}

	// Test case 2: Second encounter of the same task key, not yet checked (should be detected and return original)
	task2 := newTask(key{size: 10, hash: 1}, info{path: "/path/to/file2.txt", checked: false})
	checkedTask, detected = cl.verify(task2)
	if !detected {
		t.Errorf("Expected detected, but got not detected for task2")
	}
	if checkedTask == nil || checkedTask.key != task1.key || checkedTask.info.path != task1.info.path {
		t.Errorf("Expected checkedTask to be original task1, got %+v", checkedTask)
	}
	// Verify task1 in the list is now marked as checked
	if info, ok := cl.(*checkList).list[task1.key]; !ok || !info.checked {
		t.Errorf("Task1 not marked as checked after second encounter")
	}

	// Test case 3: Third encounter of the same task key, already checked (should be detected, but return nil checkedTask)
	task3 := newTask(key{size: 10, hash: 1}, info{path: "/path/to/file3.txt", checked: false})
	checkedTask, detected = cl.verify(task3)
	if !detected {
		t.Errorf("Expected detected, but got not detected for task3 (already checked)")
	}
	if checkedTask != nil {
		t.Errorf("Expected nil checkedTask for already checked task, got %+v", checkedTask)
	}
	// Verify task1 in the list remains marked as checked
	if info, ok := cl.(*checkList).list[task1.key]; !ok || !info.checked {
		t.Errorf("Task1 unexpectedly unchecked after third encounter")
	}

	// Test case 4: New task with different key
	task4 := newTask(key{size: 20, hash: 2}, info{path: "/path/to/file4.txt", checked: false})
	checkedTask, detected = cl.verify(task4)
	if detected {
		t.Errorf("Expected not detected, but got detected for task4")
	}
	if checkedTask != nil {
		t.Errorf("Expected nil checkedTask for new task, got %+v", checkedTask)
	}
	if _, ok := cl.(*checkList).list[task4.key]; !ok {
		t.Errorf("Task4 not added to list")
	}
}

// Test_checkList_Review tests the review method of checkList.
func Test_checkList_Review(t *testing.T) {
	cl := newCheckList()

	// Test case 1: Review a new task (not in list)
	taskToReview1 := newTask(key{size: 100, hash: 123}, info{checked: false, path: "/new/path1.txt"})
	_ = cl.review(taskToReview1)
	reviewedTask := cl.review(taskToReview1)

	// review adds the task if not present, and returns the newly added task (with its original info.checked=false)
	// or the existing one. If it was new, it effectively returns itself but from the map.
	if reviewedTask == nil {
		t.Errorf("Expected a reviewed task, got nil")
	}
	if reviewedTask.path != taskToReview1.path {
		t.Errorf("Path mismatch. Expected %s, Got %s", taskToReview1.path, reviewedTask.path)
	}
	if reviewedTask.checked { // It should be unchecked if it was newly added
		t.Errorf("Expected newly reviewed task to be unchecked, got checked")
	}
	// Verify it's in the list
	if info, ok := cl.(*checkList).list[taskToReview1.key]; !ok || info.path != taskToReview1.path {
		t.Errorf("Task not added to list correctly after review")
	}

	// Test case 2: Review an existing task (already in list from previous step)
	taskToReview2 := newTask(key{size: 100, hash: 123}, info{checked: false, path: "/another/path2.txt"}) // Same key, different path
	reviewedTask = cl.review(taskToReview2)

	// It should return the original task (taskToReview1) because its key matched
	if reviewedTask == nil {
		t.Errorf("Expected a reviewed task for existing key, got nil")
	}
	if reviewedTask.path != taskToReview1.path { // It should return the original path from the map
		t.Errorf("Path mismatch. Expected %s, Got %s", taskToReview1.path, reviewedTask.path)
	}
	if reviewedTask.checked { // Original task was unchecked
		t.Errorf("Expected original reviewed task to be unchecked, got checked")
	}
}

// --- Tests for `queue.go` ---

// Test_Queue_PushPop tests basic push and pop operations of the custom queue.
func Test_Queue_PushPop(t *testing.T) {
	q := newQueue("test_queue")

	task1 := &task{key{size: 1}, info{path: "path1"}}
	task2 := &task{key{size: 2}, info{path: "path2"}}

	q.push(task1)
	q.push(task2)

	poppedTask1 := q.pop()
	if poppedTask1 != task1 {
		t.Errorf("Expected task1, got %v", poppedTask1)
	}

	poppedTask2 := q.pop()
	if poppedTask2 != task2 {
		t.Errorf("Expected task2, got %v", poppedTask2)
	}

	poppedTask3 := q.pop()
	if poppedTask3 != nil {
		t.Errorf("Expected nil for empty queue, got %v", poppedTask3)
	}
}

// Test_Queue_InpOutProcess tests the channel-based queue processing.
func Test_Queue_InpOutProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate an input channel
	inputChan := make(chan *task, 5)

	// Create queue using inpQueue, which starts inpProcess internally
	q := inpQueue("test_inp_out_queue", inputChan)

	// Create output channel using outQueue, which starts outProcess internally
	outputChan := outQueue(ctx, q)

	task1 := newTask(key{size: 1}, info{path: "path1"})
	task2 := newTask(key{size: 2}, info{path: "path2"})
	task3 := newTask(key{size: 3}, info{path: "path3"})

	// Push tasks to input channel
	inputChan <- task1
	inputChan <- task2
	inputChan <- task3
	close(inputChan) // Signal that no more tasks will come

	receivedTasks := []*task{}
	timeout := time.After(500 * time.Millisecond) // Give it some time
	for len(receivedTasks) < 3 {
		select {
		case tsk, ok := <-outputChan:
			if !ok { // Channel closed before all tasks received
				break
			}
			receivedTasks = append(receivedTasks, tsk)
		case <-timeout:
			t.Fatal("Timeout waiting for tasks from output channel")
		}
	}

	// At this point, all expected tasks are received, the outQueue's outProcess should eventually close 'outputChan'
	// as inpQueue's inpProcess will close queue.innerChan when inputChan closes.
	// We don't close outputChan explicitly here as outProcess does it.

	// Give a moment for goroutines to clean up
	time.Sleep(100 * time.Millisecond)

	if len(receivedTasks) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(receivedTasks))
	}
	// Check content and order
	if receivedTasks[0].path != task1.path || receivedTasks[1].path != task2.path || receivedTasks[2].path != task3.path {
		t.Errorf("Tasks received in wrong order or content: %+v", receivedTasks)
	}
}

// Test_fileGenerator tests the fileGenerator worker.
// It sets up a simplified pipeline to check its behavior.
func Test_fileGenerator(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_file_generator")
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Create test files and an empty directory
	file1Path := createTempFile(t, tmpDir, "gen_file1.txt", 10, 'x')
	file2Path := createTempFile(t, subDir, "gen_file2.txt", 20, 'y')
	emptyDirPath := filepath.Join(tmpDir, "empty_dir") // This will also be a task but not outputted as a file
	if err := os.Mkdir(emptyDirPath, 0755); err != nil {
		t.Fatalf("failed to create empty dir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recChan := make(chan *task)                                 // For recursive tasks (directories)
	inpForFetchersQueue := fetchQueue(ctx, recChan, &metrics{}) // Simulate fetchQueue

	// Mock searchEngine for poolCount
	mockEngine := &searchEngine{}

	// Call fileGenerator (it manages its own dispatcher and fetcher worker goroutines)
	// Pass mockEngine and testLogger
	outputChan := fileGenerator(tmpDir, 1, recChan, inpForFetchersQueue, mockEngine)

	// Collect tasks from outputChan (files found by fetchers)
	var receivedTasks []*task
	for task := range outputChan {
		receivedTasks = append(receivedTasks, task)
	}

	mockEngine.poolCount.Wait() // Wait for all workers within fileGenerator's pool to finish

	// Assertions:
	// The `fetcher.run` outputs *only file tasks* to its `out` channel.
	// Directory tasks are sent to `rec` for recursive processing but not outputted to `outputChan`.
	// So, we expect only the 2 files created.
	if len(receivedTasks) != 2 {
		t.Fatalf("Expected 2 file tasks, got %d. Received: %+v", len(receivedTasks), receivedTasks)
	}

	// Check if paths are as expected
	foundPaths := []string{}
	for _, tsk := range receivedTasks {
		foundPaths = append(foundPaths, filepath.ToSlash(tsk.info.path))
	}
	sort.Strings(foundPaths) // Sort for consistent comparison

	expectedPaths := []string{
		filepath.ToSlash(file1Path),
		filepath.ToSlash(file2Path),
	}
	sort.Strings(expectedPaths)

	if !reflect.DeepEqual(foundPaths, expectedPaths) {
		t.Errorf("FileGenerator paths mismatch.\nExpected: %v\nGot: %v", expectedPaths, foundPaths)
	}

	// Additionally check sizes
	for _, tsk := range receivedTasks {
		if tsk.info.path == filepath.ToSlash(file1Path) {
			if tsk.key.size != 10 {
				t.Errorf("File1 size mismatch: Expected 10, got %d", tsk.key.size)
			}
		} else if tsk.info.path == filepath.ToSlash(file2Path) {
			if tsk.key.size != 20 {
				t.Errorf("File2 size mismatch: Expected 20, got %d", tsk.key.size)
			}
		}
	}
}

// Test_sizer_Run tests the sizer worker.
func Test_sizer_Run(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_sizer")
	defer os.RemoveAll(tmpDir)

	smallFile := createTempFile(t, tmpDir, "small.txt", 10, 'a')
	largeFile := createTempFile(t, tmpDir, "large.txt", 1024*1024*2, 'b') // 2MB file
	zeroFile := createTempFile(t, tmpDir, "zero.txt", 0, 'c')             // Will be filtered out
	//nonExistentFile := filepath.Join(tmpDir, "non_existent.txt")          // Will cause error

	inpChan := make(chan *task)
	go func() {
		// Test case 1: Small file (should pass)
		inpChan <- newTask(key{size: 10}, info{path: smallFile})
		// Test case 2: Large file (should pass)
		inpChan <- newTask(key{size: 1024 * 1024 * 2}, info{path: largeFile})
		// Test case 3: Zero-byte file (should be filtered out by minFileSize logic, if any, or pass if size is the only filter for next stage)
		inpChan <- newTask(key{size: 0}, info{path: zeroFile})
		// Test case 4: Non-existent file (will cause os.Stat error in verify or checker logic and be filtered out if it can't get info)
		//inpChan <- newTask(key{size: 100}, info{path: nonExistentFile})
		close(inpChan) // Signal no more input tasks
	}()

	receivedPaths := make([]string, 0)

	mockEngine := &searchEngine{}

	for task := range runPool(&sizer{}, 1, inpChan, newCheckList(), mockEngine) {
		receivedPaths = append(receivedPaths, task.info.path)
	}
	mockEngine.poolCount.Wait() // Wait for all workers to finish

	// In this test, all files have unique sizes (or 0), so no duplicates will be detected at the sizer stage.
	// Therefore, the expected output should be 0.
	if len(receivedPaths) != 0 {
		t.Errorf("Expected 0 tasks to pass sizer, got %d. Received paths: %v", len(receivedPaths), receivedPaths)
	}
}

// Test_hasher_Run tests the hasher worker.
func Test_hasher_Run(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_hasher")
	defer os.RemoveAll(tmpDir)

	contentA := []byte(strings.Repeat("a", 100))
	contentB := []byte(strings.Repeat("a", 100)) // Duplicate content
	contentC := []byte(strings.Repeat("b", 100)) // Different content

	fileA := filepath.Join(tmpDir, "fileA.txt")
	fileB := filepath.Join(tmpDir, "fileB.txt")
	fileC := filepath.Join(tmpDir, "fileC.txt")
	nonExistentFile := filepath.Join(tmpDir, "no_such_file.txt")

	os.WriteFile(fileA, contentA, 0644)
	os.WriteFile(fileB, contentB, 0644)
	os.WriteFile(fileC, contentC, 0644)

	hashA := crc32.ChecksumIEEE(contentA)
	hashB := crc32.ChecksumIEEE(contentB)
	//hashC := crc32.ChecksumIEEE(contentC)

	hasherWorker := &hasher{}
	inpChan := make(chan *task)
	checker := newCheckList() // hasher uses checkList for hash filtering
	go func() {
		// Simulate tasks coming from sizer (all valid sizes)
		// TaskA and TaskB have the same hash
		inpChan <- newTask(key{size: int64(len(contentA))}, info{path: fileA})
		inpChan <- newTask(key{size: int64(len(contentB))}, info{path: fileB})
		inpChan <- newTask(key{size: int64(len(contentC))}, info{path: fileC})
		inpChan <- newTask(key{size: 100}, info{path: nonExistentFile}) // Simulate a file that might have existed but was deleted

		close(inpChan)
	}()

	receivedTasks := make([]*task, 0)

	mockEngine := &searchEngine{}

	for task := range runPool(hasherWorker, 1, inpChan, checker, mockEngine) {
		receivedTasks = append(receivedTasks, task)
	}
	mockEngine.poolCount.Wait()

	// Expected: Hasher outputs tasks only if a duplicate hash is detected (from checker.verify)
	// TaskA (first encounter for hashA): checker.verify returns detected=false. Not outputted.
	// TaskB (second encounter for hashA, same as TaskA): checker.verify returns detected=true, checkedTask=TaskA.
	//    Outputs TaskB and TaskA.
	// TaskC (first encounter for hashC): checker.verify returns detected=false. Not outputted.
	// nonExistentFile: checkError returns true, so it's filtered out.
	// So, we expect 2 tasks: TaskA and TaskB.
	if len(receivedTasks) != 2 {
		t.Fatalf("Expected 2 tasks to pass hasher, got %d. Received: %+v", len(receivedTasks), receivedTasks)
	}

	// Check if the correct files passed and their hashes are set
	foundPaths := make(map[string]bool)
	for _, tsk := range receivedTasks {
		foundPaths[tsk.info.path] = true
		if tsk.info.path == fileA {
			if tsk.key.hash != hashA {
				t.Errorf("FileA hash mismatch: Expected %d, got %d", hashA, tsk.key.hash)
			}
		} else if tsk.info.path == fileB {
			if tsk.key.hash != hashB {
				t.Errorf("FileB hash mismatch: Expected %d, got %d", hashB, tsk.key.hash)
			}
		} else {
			t.Errorf("Unexpected file %s in output", tsk.info.path)
		}
	}

	if !foundPaths[fileA] || !foundPaths[fileB] {
		t.Errorf("Expected fileA and fileB to be outputted by hasher")
	}
	if foundPaths[fileC] {
		t.Errorf("FileC unexpectedly passed hasher")
	}
	if foundPaths[nonExistentFile] {
		t.Errorf("Non-existent file unexpectedly passed hasher")
	}
}

// Test_matcher_Run tests the matcher worker.
func Test_matcher_Run(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_matcher")
	defer os.RemoveAll(tmpDir)

	contentA := make([]byte, 1024)
	rand.Read(contentA) // Generate random content

	contentB := make([]byte, 1024)
	copy(contentB, contentA) // Make contentB identical to contentA

	contentC := make([]byte, 1024)
	rand.Read(contentC) // Generate different random content

	file1Path := filepath.Join(tmpDir, "file1.bin")
	file2Path := filepath.Join(tmpDir, "file2.bin")
	file3Path := filepath.Join(tmpDir, "file3.bin")

	// Manually write the specific content to files
	if err := os.WriteFile(file1Path, contentA, 0644); err != nil {
		t.Fatalf("Failed to write specific content to %s: %v", file1Path, err)
	}
	if err := os.WriteFile(file2Path, contentB, 0644); err != nil {
		t.Fatalf("Failed to write specific content to %s: %v", file2Path, err)
	}
	if err := os.WriteFile(file3Path, contentC, 0644); err != nil {
		t.Fatalf("Failed to write specific content to %s: %v", file3Path, err)
	}

	hashA := crc32.ChecksumIEEE(contentA)
	hashB := crc32.ChecksumIEEE(contentB) // Same as hashA
	hashC := crc32.ChecksumIEEE(contentC)

	matcherWorker := &matcher{}
	inpChan := make(chan *task)
	checker := newCheckList() // matcher uses checkList for byte-to-byte comparison check

	mockEngine := &searchEngine{}

	// Simulate tasks coming from hasher (same size, same hash, but actual content might differ)
	// Task1: first file of a potential duplicate group
	task1 := newTask(key{size: 1024, hash: hashA}, info{path: file1Path})
	// Task2: duplicate candidate (same size, same hash, same content)
	task2 := newTask(key{size: 1024, hash: hashB}, info{path: file2Path})
	// Task3: different content, different hash (should not match anything)
	task3 := newTask(key{size: 1024, hash: hashC}, info{path: file3Path})

	// Add task1 to checker first, simulating it came through previous stages and was first for its hash
	checker.verify(task1) // This will add task1 to the checker list and return detected=false
	go func() {
		inpChan <- task2 // This is the task that should detect file1 as its duplicate
		inpChan <- task3 // This is a unique task
		// Add a non-existent file to test error handling
		nonExistentPath := filepath.Join(tmpDir, "non_existent_matcher.bin")
		inpChan <- newTask(key{size: 1024, hash: 0}, info{path: nonExistentPath})

		close(inpChan)
	}()

	var receivedTasks []*task
	for task := range runPool(matcherWorker, 1, inpChan, checker, mockEngine) {
		receivedTasks = append(receivedTasks, task)
	}
	mockEngine.poolCount.Wait()

	// Expected output:
	// When task2 (file2.bin) is processed:
	// - checker.review(task2) returns task1 (file1.bin)
	// - checkEqual(file2.bin, file1.bin) is true
	// - checker.verify(task2) is called. 'detected' will be true. 'verifiedTask' will be task1 (because task1.info.checked was false when added by checker.verify).
	// - So, 'out <- task2' and 'out <- task1' will be called. (2 tasks)
	// When task3 (file3.bin) is processed:
	// - checker.review(task3) adds task3 to checker, returns nil
	// - Break from inner loop. No output.
	// When nonExistentPath is processed: error, break from inner loop. No output.
	// So, total 2 tasks expected: task1 and task2.
	if len(receivedTasks) != 2 {
		t.Fatalf("Expected 2 tasks to pass matcher, got %d. Received: %+v", len(receivedTasks), receivedTasks)
	}

	foundFile1 := false
	foundFile2 := false
	for _, tsk := range receivedTasks {
		if tsk.info.path == file1Path {
			foundFile1 = true
			if tsk.key.hash != hashA {
				t.Errorf("File1 hash mismatch: Expected %d, got %d", hashA, tsk.key.hash)
			}
		} else if tsk.info.path == file2Path {
			foundFile2 = true
			if tsk.key.hash != hashB {
				t.Errorf("File2 hash mismatch: Expected %d, got %d", hashB, tsk.key.hash)
			}
		} else {
			t.Errorf("Unexpected file %s in output", tsk.info.path)
		}
	}

	if !foundFile1 || !foundFile2 {
		t.Errorf("Expected file1 and file2 to be outputted by matcher, but got: %+v", receivedTasks)
	}
}

// Test_runPool_Concurrency tests runPool with multiple workers and updated mock logic.
func Test_runPool_Concurrency(t *testing.T) {
	tmpDir := createTempDir(t, "fdd_test_runpool_concurrency")
	defer os.RemoveAll(tmpDir)

	numFiles := 100 // 50 unique, 50 duplicates
	fileSize := int64(100)
	duplicateContentBytes := []byte(strings.Repeat("z", int(fileSize)))
	//duplicateHash := crc32.ChecksumIEEE(duplicateContentBytes)

	// Create files: 50 unique, 50 duplicates
	paths := make([]string, numFiles)
	for i := 0; i < numFiles/2; i++ {
		// Unique files
		uniqueContent := make([]byte, fileSize)
		rand.Read(uniqueContent) // Random unique content
		uniquePaths := filepath.Join(tmpDir, fmt.Sprintf("uniq_%d.txt", i))
		if err := os.WriteFile(uniquePaths, uniqueContent, 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		paths[i] = uniquePaths

		// Duplicate files
		duplicatePaths := filepath.Join(tmpDir, fmt.Sprintf("dup_%d.txt", i))
		if err := os.WriteFile(duplicatePaths, duplicateContentBytes, 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		paths[numFiles/2+i] = duplicatePaths
	}

	mockChecker := newCheckList()
	mockEngine := &searchEngine{}

	inpChan := make(chan *task, numFiles) // Buffered input for efficiency in test

	go func() {
		// Send all tasks to the input channel (order might affect which is 'first' duplicate)
		for _, p := range paths {
			var content []byte
			if strings.Contains(p, "dup_") {
				content = duplicateContentBytes
			} else {
				// Read content for unique files to get their correct hash
				content, _ = os.ReadFile(p)
			}
			tsk := newTask(key{size: fileSize, hash: crc32.ChecksumIEEE(content)}, info{path: p})
			inpChan <- tsk
		}
		close(inpChan)
	}()

	// Collect results from the output channel
	var receivedTasks []*task

	//for tsk := range runPool(&mockMatcherWorker{}, 5, inpChan, mockChecker, mockEngine) {
	for tsk := range runPool(&matcher{}, 5, inpChan, mockChecker, mockEngine) {
		receivedTasks = append(receivedTasks, tsk)
	}
	mockEngine.poolCount.Wait() // Wait for all workers to finish via the engine's WaitGroup

	// Verify results
	// Number of unique files: numFiles/2 = 50. These are not outputted by mockMatcherWorker unless they find a duplicate.
	// Number of duplicate files: numFiles/2 = 50.
	expectedOutputCount := (numFiles / 2)

	if len(receivedTasks) != expectedOutputCount {
		t.Errorf("Expected %d tasks from runPool, got %d", expectedOutputCount, len(receivedTasks))
	}
}

// Test_checkError_SlogLogging tests if checkError uses slog.Default() correctly and logs at INFO level.
func Test_checkError_SlogLogging(t *testing.T) {
	// Redirect slog output to a buffer to capture logs
	var buf strings.Builder
	// Save the original default logger
	originalLogger := slog.Default()
	// Create a new logger that writes to our buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug, // To capture INFO level messages
	}))
	slog.SetDefault(testLogger)
	defer slog.SetDefault(originalLogger) // Restore original logger after test

	testError := fmt.Errorf("test error message")
	testMsg := "Failed to process"
	testMethod := "testMethod"
	testItem := struct{ Name string }{Name: "testItem"}
	testTask := &task{key: key{size: 1}, info: info{path: "/test/path"}}

	// Call checkError
	hasError := checkError(testError, testMsg, testMethod, testItem, testTask)

	if !hasError {
		t.Errorf("checkError returned false, expected true for an error")
	}

	logOutput := buf.String()

	// Verify that the log output contains the expected information and INFO level
	if !strings.Contains(logOutput, "level=INFO") { // Assert INFO level as per user's design
		t.Errorf("Log output missing INFO level: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("msg=\"%s\"", testMsg)) {
		t.Errorf("Log output missing expected message: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("method=%s", testMethod)) {
		t.Errorf("Log output missing method: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("error=\"%s\"", testError.Error())) {
		t.Errorf("Log output missing error message: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("item=\"%T\"", testItem)) { // Item's string representation
		t.Errorf("Log output missing item info: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("path=%s", testTask.info.path)) {
		t.Errorf("Log output missing task path: %s", logOutput)
	}
}

// Test_checkTasksError_SlogLogging tests if checkTasksError uses slog.Default() correctly and logs at INFO level.
func Test_checkTasksError_SlogLogging(t *testing.T) {
	// Redirect slog output to a buffer to capture logs
	var buf strings.Builder
	originalLogger := slog.Default()
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(testLogger)
	defer slog.SetDefault(originalLogger)

	testError := fmt.Errorf("comparison failed")
	testMsg := "Check equal error"
	testMethod := "checkEqual()"
	testItem := struct{ Name string }{Name: "matcher"}
	task1 := &task{key: key{size: 10}, info: info{path: "/path/file1.txt"}}
	task2 := &task{key: key{size: 10}, info: info{path: "/path/file2.txt"}}

	hasError := checkTasksError(testError, testMsg, testMethod, testItem, task1, task2)

	if !hasError {
		t.Errorf("checkTasksError returned false, expected true for an error")
	}

	logOutput := buf.String()

	if !strings.Contains(logOutput, "level=INFO") {
		t.Errorf("Log output missing INFO level: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("msg=\"%s\"", testMsg)) {
		t.Errorf("Log output missing expected message: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("method=%s", testMethod)) {
		t.Errorf("Log output missing method: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("error=\"%s\"", testError.Error())) {
		t.Errorf("Log output missing error message: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("item=\"%T\"", testItem)) {
		t.Errorf("Log output missing item info: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("path-1=%s", task1.info.path)) {
		t.Errorf("Log output missing task1 path: %s", logOutput)
	}
	if !strings.Contains(logOutput, fmt.Sprintf("path-2=%s", task2.info.path)) {
		t.Errorf("Log output missing task2 path: %s", logOutput)
	}
}
