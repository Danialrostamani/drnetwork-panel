package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// These run a real core, so they are skipped under -race: see race_on_test.go.

// The stored config sends example.com to "c" with a route rule, which sing-box
// looks up when a connection is matched, and everything else to the selector
// "sel", which keeps hold of its members "a" and "b" from start-up on.
func runningCoreService(t *testing.T) *ConfigService {
	t.Helper()
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	s := lifecycleService(t)
	if err := s.SettingService.SetConfig(`{"log":{"level":"error"},"route":{"final":"sel","rules":[{"domain":["example.com"],"action":"route","outbound":"c"}]}}`); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"type":"direct","tag":"a"}`,
		`{"type":"direct","tag":"b"}`,
		`{"type":"direct","tag":"c"}`,
		`{"type":"selector","tag":"sel","outbounds":["a","b"],"default":"a"}`,
	} {
		var o model.Outbound
		if err := o.UnmarshalJSON([]byte(raw)); err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	rawConfig, err := s.GetConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if err := corePtr.Start(*rawConfig); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = corePtr.Stop() })
	return s
}

// An outbound the core does not know answers "outbound not found" without
// dialling anything.
func isLive(tag string) bool {
	return corePtr.CheckOutbound(tag, "http://127.0.0.1:1/").Error != "outbound not found"
}

func outboundID(t *testing.T, tag string) uint {
	t.Helper()
	var o model.Outbound
	if err := database.GetDB().Where("tag = ?", tag).First(&o).Error; err != nil {
		t.Fatal(err)
	}
	return o.Id
}

func outboundOptions(t *testing.T, tag string) string {
	t.Helper()
	var o model.Outbound
	if err := database.GetDB().Where("tag = ?", tag).First(&o).Error; err != nil {
		t.Fatal(err)
	}
	return string(o.Options)
}

func saveOutbound(t *testing.T, s *ConfigService, act string, payload map[string]interface{}) error {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save("outbounds", act, data, "", "tester", "localhost")
	return err
}

// Waits for the core to come back as a different instance than before.
func waitForRestart(t *testing.T, before *core.Box) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if cur := corePtr.GetInstance(); cur != nil && cur != before && corePtr.IsRunning() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the core was not restarted")
}

// Nothing may restart the core within a moment of the call returning.
func assertNoRestart(t *testing.T, before *core.Box) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	if cur := corePtr.GetInstance(); cur != before {
		t.Fatal("the core was restarted for a change it could take in place")
	}
}

// Replacing "b" in the running core would leave "sel" holding the closed old
// instance, the same way route.final does for a WireGuard endpoint. The change
// has to reach the core by a restart instead.
func TestEditingAReferencedOutboundRestartsTheCore(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	err := saveOutbound(t, s, "edit", map[string]interface{}{
		"id": outboundID(t, "b"), "type": "direct", "tag": "b", "connect_timeout": "7s",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	waitForRestart(t, before)

	if !isLive("b") {
		t.Error("b is missing after the restart")
	}
	if !strings.Contains(outboundOptions(t, "b"), "7s") {
		t.Errorf("the edit was not stored: %s", outboundOptions(t, "b"))
	}
}

// sing-box will not start from a config where "sel" lists an outbound that is
// not there, so the delete is refused rather than left to a restart that would
// bring the core down.
func TestDeletingAReferencedOutboundIsRefused(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	_, err := s.Save("outbounds", "del", json.RawMessage(`"b"`), "", "tester", "localhost")
	if err == nil {
		t.Fatal("an outbound a selector lists was deleted")
	}
	if !strings.Contains(err.Error(), "outbounds[sel].outbounds") {
		t.Errorf("the error does not say where b is used: %v", err)
	}
	if !isLive("b") {
		t.Error("b is gone from the running core")
	}
	var stored int64
	database.GetDB().Model(&model.Outbound{}).Where("tag = ?", "b").Count(&stored)
	if stored != 1 {
		t.Errorf("b was removed from the database (%d rows)", stored)
	}
	assertNoRestart(t, before)
}

// Renaming leaves "sel" pointing at a tag that no longer exists.
func TestRenamingAReferencedOutboundIsRefused(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	err := saveOutbound(t, s, "edit", map[string]interface{}{
		"id": outboundID(t, "b"), "type": "direct", "tag": "b2",
	})
	if err == nil {
		t.Fatal("an outbound a selector lists was renamed")
	}
	if !isLive("b") {
		t.Error("b is gone from the running core")
	}
	assertNoRestart(t, before)
}

// The same goes for something new that depends on a tag that does not exist.
func TestADanglingDetourIsRefused(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	err := saveOutbound(t, s, "new", map[string]interface{}{"type": "direct", "tag": "e", "detour": "nowhere"})
	if err == nil {
		t.Fatal("an outbound with a detour that does not exist was accepted")
	}
	if isLive("e") {
		t.Error("the refused outbound is in the running core")
	}

	// Editing one that nothing holds on to is refused before it is touched.
	err = saveOutbound(t, s, "edit", map[string]interface{}{
		"id": outboundID(t, "c"), "type": "direct", "tag": "c", "detour": "nowhere",
	})
	if err == nil {
		t.Fatal("an edit adding a detour that does not exist was accepted")
	}
	if !isLive("c") {
		t.Error("c is gone from the running core")
	}
	assertNoRestart(t, before)
}

// A tag that only route rules use is looked up when a connection is matched, so
// replacing it in place is both enough and cheaper than dropping everyone's
// connections.
func TestEditingAnOutboundOnlyRulesUseIsDoneInPlace(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	err := saveOutbound(t, s, "edit", map[string]interface{}{
		"id": outboundID(t, "c"), "type": "direct", "tag": "c", "connect_timeout": "9s",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !isLive("c") {
		t.Error("c is gone from the running core")
	}
	assertNoRestart(t, before)
	if !strings.Contains(outboundOptions(t, "c"), "9s") {
		t.Errorf("the edit was not stored: %s", outboundOptions(t, "c"))
	}
}

func TestNewAndDeletedOutboundsAreDoneInPlace(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	if err := saveOutbound(t, s, "new", map[string]interface{}{"type": "direct", "tag": "d"}); err != nil {
		t.Fatalf("new: %v", err)
	}
	if !isLive("d") {
		t.Fatal("the new outbound is not in the running core")
	}
	if _, err := s.Save("outbounds", "del", json.RawMessage(`"d"`), "", "tester", "localhost"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if isLive("d") {
		t.Fatal("the deleted outbound is still in the running core")
	}
	assertNoRestart(t, before)
}

// The old code removed the outbound before it tried the replacement, so an edit
// sing-box refused still took the working one out of the running core.
func TestARejectedEditLeavesTheRunningOutboundAlone(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()
	stored := outboundOptions(t, "c")

	err := saveOutbound(t, s, "edit", map[string]interface{}{
		"id": outboundID(t, "c"), "type": "socks", "tag": "c", "server": "127.0.0.1", "server_port": "not-a-port",
	})
	if err == nil {
		t.Fatal("an invalid outbound was accepted")
	}
	if !isLive("c") {
		t.Error("the rejected edit took c out of the running core")
	}
	if got := outboundOptions(t, "c"); got != stored {
		t.Errorf("the rejected edit changed the stored outbound: %s", got)
	}
	assertNoRestart(t, before)
}

// An edit that has to be applied by a restart is checked first; otherwise the
// restart would be handed a config it cannot start from and the core would stay
// down.
func TestAnInvalidEditOfAReferencedOutboundIsRefused(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()
	stored := outboundOptions(t, "b")

	for name, payload := range map[string]map[string]interface{}{
		"unparsable option": {"type": "socks", "tag": "b", "server": "127.0.0.1", "server_port": "not-a-port"},
		"unknown detour":    {"type": "direct", "tag": "b", "detour": "nowhere"},
	} {
		payload["id"] = outboundID(t, "b")
		if err := saveOutbound(t, s, "edit", payload); err == nil {
			t.Errorf("%s: the edit was accepted", name)
		}
	}
	if !isLive("b") {
		t.Error("b is gone from the running core")
	}
	if got := outboundOptions(t, "b"); got != stored {
		t.Errorf("a refused edit changed the stored outbound: %s", got)
	}
	assertNoRestart(t, before)
}

// A base config save used to be dropped when it arrived while the watchdog or
// another restart held the lifecycle lock: the config was stored, never loaded.
func TestConfigSaveIsAppliedWhileALifecycleSequenceRuns(t *testing.T) {
	s := runningCoreService(t)
	before := corePtr.GetInstance()

	lifecycleMu.Lock()
	released := false
	defer func() {
		if !released {
			lifecycleMu.Unlock()
		}
	}()

	cfg := json.RawMessage(`{"log":{"level":"warn"},"route":{"final":"sel"}}`)
	if _, err := s.Save("config", "", cfg, "", "tester", "localhost"); err != nil {
		t.Fatalf("config save: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if corePtr.GetInstance() != before {
		t.Fatal("the core restarted while the lifecycle lock was held")
	}
	released = true
	lifecycleMu.Unlock()

	waitForRestart(t, before)
}
