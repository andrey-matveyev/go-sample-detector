package fdd

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
