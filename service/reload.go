package service

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
)

// coreSync tells ConfigService.Save what the running core needs once a save is
// over. The zero value means nothing.
type coreSync struct {
	// restart: the running core cannot take this change in place, so it is
	// restarted once the change is committed.
	restart bool
	// diverged: the running core was changed in place. If the save is rolled
	// back instead of committed, it no longer matches the database and has to
	// be restarted to get back in step.
	diverged bool
}

// liveOps is what a hot swap needs from the core, for outbounds and endpoints
// alike.
type liveOps struct {
	remove func(tag string) error
	add    func(config []byte) error
	// validate checks a configuration without touching the running core.
	// replacing is the tag of the object the configuration will take the place
	// of, which is about to disappear.
	validate func(config []byte, replacing string) error
}

func endpointOps() liveOps {
	return liveOps{
		remove:   func(tag string) error { return corePtr.RemoveEndpoint(tag) },
		add:      func(config []byte) error { return corePtr.AddEndpoint(config) },
		validate: func(config []byte, replacing string) error { return corePtr.ValidateEndpoint(config, replacing) },
	}
}

func outboundOps() liveOps {
	return liveOps{
		remove:   func(tag string) error { return corePtr.RemoveOutbound(tag) },
		add:      func(config []byte) error { return corePtr.AddOutbound(config) },
		validate: func(config []byte, replacing string) error { return corePtr.ValidateOutbound(config, replacing) },
	}
}

// reference finds where the generated sing-box config refers to tag in a way
// the core resolved once, when it started, and says where.
//
// sing-box looks an outbound or endpoint up by tag in a few places only -- the
// route and DNS rules, at the moment a connection is matched. Everything else
// keeps the object it found at start: route.final, the members of a selector
// or urltest, the detour of another outbound, endpoint or DNS server, HTTP
// clients and so on. Replacing an object in the running core therefore leaves
// those pointing at the old, closed instance, and every connection through
// them fails with "WireGuard is not ready yet" (or the like) until the core is
// restarted. Anything that matches here has to be handled by a restart, and
// removing it altogether leaves a config that sing-box will not start from.
//
// The test is deliberately wide: any string equal to the tag counts, except an
// object's own type, tag and panel metadata, and the two rule lists. A false
// positive costs a restart; a miss leaves traffic broken.
func reference(config []byte, tag string) (path string, found bool) {
	var root map[string]interface{}
	if err := json.Unmarshal(config, &root); err != nil {
		return "the stored configuration, which cannot be read", true
	}
	for _, section := range []string{"route", "dns"} {
		if obj, ok := root[section].(map[string]interface{}); ok {
			delete(obj, "rules")
		}
	}
	return mentions(root, tag, "")
}

func mentions(v interface{}, tag, path string) (string, bool) {
	switch t := v.(type) {
	case string:
		return path, t == tag
	case []interface{}:
		for i, item := range t {
			at := path
			if obj, ok := item.(map[string]interface{}); ok {
				label := strconv.Itoa(i)
				if name, ok := obj["tag"].(string); ok && name != "" {
					label = name
				}
				at = path + "[" + label + "]"
			}
			if found, ok := mentions(item, tag, at); ok {
				return found, true
			}
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for key := range t {
			switch key {
			case "type", "tag", "ext":
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			at := key
			if path != "" {
				at = path + "." + key
			}
			if found, ok := mentions(t[key], tag, at); ok {
				return found, true
			}
		}
	}
	return "", false
}

// tagReferenced reports whether the generated config refers to tag; see
// reference.
func tagReferenced(config []byte, tag string) bool {
	_, found := reference(config, tag)
	return found
}

// liveReference is reference applied to the stored configuration. When that
// cannot even be built it says yes: a restart is always safe, a hot swap might
// not be.
//
// The stored configuration is read through the shared connection while the
// save's own transaction is open. SQLite runs in WAL mode, so that read is not
// blocked and sees the state from before the save, which is the one the
// running core was started from.
func liveReference(tag string) (path string, found bool) {
	raw, err := (&ConfigService{}).GetConfig("")
	if err != nil {
		logger.Warning("cannot check what refers to ", tag, ": ", err)
		return "the stored configuration, which cannot be built", true
	}
	return reference(*raw, tag)
}

func liveReferences(tag string) bool {
	_, found := liveReference(tag)
	return found
}

// tagOf reads the tag out of an object's configuration.
func tagOf(config []byte) string {
	var fields struct {
		Tag string `json:"tag"`
	}
	_ = json.Unmarshal(config, &fields)
	return fields.Tag
}

// addLive adds a new object to the running core.
func addLive(ops liveOps, config []byte) (coreSync, error) {
	// sing-box only insists that what an object depends on exists when it
	// starts, so a dangling detour would be accepted here and then stop the
	// next start.
	if err := ops.validate(config, ""); err != nil {
		return coreSync{}, err
	}
	if err := ops.add(config); err != nil {
		return coreSync{}, err
	}
	return coreSync{diverged: true}, nil
}

// replaceLive swaps the object oldTag for the one described by config.
//
// A plain remove and add is only right when nothing holds on to the old
// object (see reference); otherwise the swap is left to a restart. Either way
// the new configuration is checked first, so one sing-box would reject is
// refused with the running core untouched. When the add fails regardless, the
// old object is put back: it is already gone from the running core, and the
// failed save leaves it in the database.
func replaceLive(ops liveOps, oldTag string, oldConfig, config []byte) (coreSync, error) {
	newTag := tagOf(config)
	if err := ops.validate(config, oldTag); err != nil {
		return coreSync{}, err
	}
	removed := false
	if oldTag != "" {
		if path, found := liveReference(oldTag); found {
			if newTag != oldTag {
				return coreSync{}, common.NewErrorf("%s is still used by %s, so it cannot be renamed: change that first", oldTag, path)
			}
			return coreSync{restart: true}, nil
		}
		err := ops.remove(oldTag)
		switch {
		case err == nil:
			removed = true
		case err != os.ErrInvalid:
			// How far the core got is unknown.
			return coreSync{diverged: true}, err
		}
	}
	err := ops.add(config)
	if err == nil {
		return coreSync{diverged: true}, nil
	}
	if !removed || oldConfig == nil {
		return coreSync{}, err
	}
	if restoreErr := ops.add(oldConfig); restoreErr != nil {
		logger.Error("could not put ", oldTag, " back after a rejected edit, the core will be restarted: ", restoreErr)
		return coreSync{diverged: true}, err
	}
	return coreSync{}, err
}

// removeLive takes the object tag out of the running core, unless something
// still depends on it: sing-box would not start from what is left, so the
// delete is refused instead.
func removeLive(ops liveOps, tag string) (coreSync, error) {
	if path, found := liveReference(tag); found {
		return coreSync{}, common.NewErrorf("%s is still used by %s: change that first", tag, path)
	}
	if err := ops.remove(tag); err != nil && err != os.ErrInvalid {
		return coreSync{diverged: true}, err
	}
	return coreSync{diverged: true}, nil
}

// restartGate runs core restarts so that none is lost and none is repeated for
// nothing.
//
// A restart reads the configuration from the database when it runs, so one
// started after a request was made covers that request. The gate relies on
// that: a request that finds a restart already underway waits for it and then
// runs once more, because the running one may have read the database before
// the request's change was committed; requests that queue up behind the same
// restart share a single follow-up.
type restartGate struct {
	requested atomic.Uint64
	mu        sync.Mutex
	applied   uint64 // guarded by mu
}

// run executes do unless a run that began after this call already covered it.
func (g *restartGate) run(do func() error) error {
	ticket := g.requested.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.applied >= ticket {
		return nil
	}
	// Read before do, so every request counted here is committed and visible
	// to the configuration do reads. Marked applied whatever the outcome:
	// repeating a restart that failed for the same configuration would only
	// fail again, and the next request tries afresh.
	covered := g.requested.Load()
	err := do()
	g.applied = covered
	return err
}

var coreRestarts restartGate
