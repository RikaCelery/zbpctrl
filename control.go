package control

import (
	"math/bits"
	"strconv"

	log "github.com/sirupsen/logrus"
)

type status struct {
	isenable bool
}

// Control is to control the plugins.
type Control[CTX any] struct {
	Service string
	Cache   map[int64]*status
	Options Options[CTX]
	Manager *Manager[CTX]
}

// NewControl returns Manager with settings.
func (manager *Manager[CTX]) NewControl(service string, o *Options[CTX]) *Control[CTX] {
	var c GroupConfig
	m := &Control[CTX]{
		Service: service,
		Cache:   make(map[int64]*status, 16),
		Options: func() Options[CTX] {
			if o == nil {
				return Options[CTX]{}
			}
			return *o
		}(),
		Manager: manager,
	}
	manager.Lock()
	defer manager.Unlock()
	manager.M[service] = m
	err := manager.D.Create(service, &c)
	if err != nil {
		panic(err)
	}
	err = manager.D.Create(service+"ban", &BanStatus{})
	if err != nil {
		panic(err)
	}
	err = manager.D.Find(m.Service, &c, "WHERE gid=0")
	if err == nil {
		if bits.RotateLeft64(uint64(c.Disable), 1)&1 == 1 {
			m.Options.DisableOnDefault = !m.Options.DisableOnDefault
		}
	}
	return m
}

// Enable enables a group to pass the Manager.
// groupID == 0 (ALL) will operate on all grps.
func (m *Control[CTX]) Enable(groupID int64) {
	var c GroupConfig
	m.Manager.RLock()
	err := m.Manager.D.Find(m.Service, &c, "WHERE gid="+strconv.FormatInt(groupID, 10))
	m.Manager.RUnlock()
	if err != nil {
		c.GroupID = groupID
	}
	c.Disable = int64(uint64(c.Disable) & 0xffffffff_fffffffe)
	m.Manager.Lock()
	m.Cache[groupID] = &status{isenable: true}
	err = m.Manager.D.Insert(m.Service, &c)
	m.Manager.Unlock()
	if err != nil {
		log.Errorf("[control] %v", err)
	}
}

// Disable disables a group to pass the Manager.
// groupID == 0 (ALL) will operate on all grps.
func (m *Control[CTX]) Disable(groupID int64) {
	var c GroupConfig
	m.Manager.RLock()
	err := m.Manager.D.Find(m.Service, &c, "WHERE gid="+strconv.FormatInt(groupID, 10))
	m.Manager.RUnlock()
	if err != nil {
		c.GroupID = groupID
	}
	c.Disable |= 1
	m.Manager.Lock()
	m.Cache[groupID] = &status{isenable: false}
	err = m.Manager.D.Insert(m.Service, &c)
	m.Manager.Unlock()
	if err != nil {
		log.Errorf("[control] %v", err)
	}
}

// Reset resets the default config of a group.
// groupID == 0 (ALL) is not allowed.
func (m *Control[CTX]) Reset(groupID int64) {
	if groupID != 0 {
		m.Manager.Lock()
		m.Cache[groupID] = nil
		err := m.Manager.D.Del(m.Service, "WHERE gid="+strconv.FormatInt(groupID, 10))
		m.Manager.Unlock()
		if err != nil {
			log.Errorf("[control] %v", err)
		}
	}
}

// IsEnabledIn 查询开启群
// 当全局未配置或与默认相同时, 状态取决于单独配置, 后备为默认配置；
// 当全局与默认不同时, 状态取决于全局配置, 单独配置失效。
func (m *Control[CTX]) IsEnabledIn(gid int64) bool {
	var c GroupConfig
	var err error
	m.Manager.RLock()
	yes, ok := m.Cache[0]
	m.Manager.RUnlock()
	if !ok {
		m.Manager.RLock()
		err = m.Manager.D.Find(m.Service, &c, "WHERE gid=0")
		m.Manager.RUnlock()
		m.Manager.Lock()
		if err == nil && c.GroupID == 0 {
			m.Cache[0] = &status{isenable: c.Disable&1 == 0}
		} else {
			m.Cache[0] = nil
		}
		m.Manager.Unlock()
		log.Debugf("[control] cache plugin %s of all : %v", m.Service, yes)
	}

	if (yes != nil) && (yes.isenable == m.Options.DisableOnDefault) { // global enable status is different from default value
		return yes.isenable
	}

	m.Manager.RLock()
	yes, ok = m.Cache[gid]
	m.Manager.RUnlock()
	if ok {
		log.Debugf("[control] read cached %s of grp %d : %v", m.Service, gid, yes)
	} else {
		m.Manager.RLock()
		err = m.Manager.D.Find(m.Service, &c, "WHERE gid="+strconv.FormatInt(gid, 10))
		m.Manager.RUnlock()
		if err == nil && gid == c.GroupID {
			m.Manager.Lock()
			m.Cache[gid] = &status{isenable: c.Disable&1 == 0}
			m.Manager.Unlock()
			log.Debugf("[control] cache plugin %s of grp %d : %v", m.Service, gid, yes)
		}
	}

	if ok {
		return yes.isenable
	}

	m.Manager.Lock()
	m.Cache[gid] = &status{isenable: !m.Options.DisableOnDefault}
	m.Manager.Unlock()
	log.Debugf("[control] cache plugin %s of grp %d (default) : %v", m.Service, gid, !m.Options.DisableOnDefault)

	return !m.Options.DisableOnDefault
}

// Handler 返回 预处理器
func (m *Control[CTX]) Handler(gid, uid int64) bool {
	if m.Manager.IsBlocked(uid) {
		return false
	}
	grp := gid
	if grp == 0 {
		// 个人用户
		grp = -uid
	}
	if !m.Manager.CanResponse(grp) || m.IsBannedIn(uid, grp) {
		return false
	}
	return m.IsEnabledIn(grp)
}

// String 打印帮助
func (m *Control[CTX]) String() string {
	return m.Options.Help
}
