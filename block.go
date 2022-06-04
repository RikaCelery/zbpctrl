package control

import (
	"strconv"
)

func (manager *Manager[CTX]) InitBlock() error {
	return manager.D.Create("__block", &block{})
}

var blockCache = make(map[int64]bool)

func (manager *Manager[CTX]) DoBlock(uid int64) error {
	manager.Lock()
	defer manager.Unlock()
	blockCache[uid] = true
	return manager.D.Insert("__block", &block{UserID: uid})
}

func (manager *Manager[CTX]) DoUnblock(uid int64) error {
	manager.Lock()
	defer manager.Unlock()
	blockCache[uid] = false
	return manager.D.Del("__block", "where uid = "+strconv.FormatInt(uid, 10))
}

func (manager *Manager[CTX]) IsBlocked(uid int64) bool {
	manager.RLock()
	isbl, ok := blockCache[uid]
	manager.RUnlock()
	if ok {
		return isbl
	}
	manager.Lock()
	defer manager.Unlock()
	isbl = manager.D.CanFind("__block", "where uid = "+strconv.FormatInt(uid, 10))
	blockCache[uid] = isbl
	return isbl
}
