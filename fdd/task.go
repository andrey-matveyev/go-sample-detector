package fdd

import (
	"cmp"
	"slices"
)

type key struct {
	size  int64
	hash  uint32
	equal int
}

type info struct {
	checked bool
	path    string
}

type task struct {
	key
	info
}

func newTask(key key, info info) *task {
	return &task{key: key, info: info}
}

type header struct {
	Size  int64
	Hash  uint32
	Group int
}

type body struct {
	Paths []string
}

type element struct {
	header
	body
}

type Result struct {
	List []element
}

func newResult(predResult *predResult) *Result {
	item := &Result{}
	item.List = make([]element, len(predResult.List))
	idx := 0
	for key := range predResult.List {
		slices.Sort(predResult.List[key])
		item.List[idx].Size = key.size
		item.List[idx].Hash = key.hash
		item.List[idx].Group = key.equal
		item.List[idx].Paths = predResult.List[key]
		idx++
	}
	slices.SortFunc(item.List, compareTaskKey)
	return item
}

func compareTaskKey(a, b element) int {
	if a.Size == b.Size {
		if a.Hash == b.Hash {
			return cmp.Compare(a.Group, b.Group)
		}
		return cmp.Compare(a.Hash, b.Hash)
	}
	return cmp.Compare(a.Size, b.Size)
}

type predResult struct {
	List map[key][]string
}

func newPredResult() *predResult {
	return &predResult{List: make(map[key][]string)}
}
