package fdd

import "sync"

type checker interface {
	verify(task *task) (checkedTask *task, detected bool)
	review(task *task) (checkedTask *task)
}

type checkList struct {
	mtx  sync.Mutex
	list map[key]info
}

func newCheckList() checker {
	return &checkList{list: make(map[key]info)}
}

func (item *checkList) verify(task *task) (checkedTask *task, detected bool) {
	item.mtx.Lock()
	defer item.mtx.Unlock()

	info, detected := item.list[task.key]
	if detected {
		if info.checked {
			return nil, detected
		}
		checkedTask = newTask(task.key, info)
		info.checked = true
		item.list[task.key] = info
		return checkedTask, detected
	}
	item.list[task.key] = task.info
	return nil, detected
}

func (item *checkList) review(task *task) (checkedTask *task) {
	item.mtx.Lock()
	defer item.mtx.Unlock()

	info, detected := item.list[task.key]
	if detected {
		return newTask(task.key, info)
	}
	item.list[task.key] = task.info
	return nil
}
