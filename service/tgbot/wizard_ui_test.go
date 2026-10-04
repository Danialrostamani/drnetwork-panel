package tgbot

import (
	"context"
	"strings"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

// rowOf returns the callback data (or labels) of one keyboard row.
func rowOf(kb [][]button, i int, labels bool) string {
	if i >= len(kb) {
		return ""
	}
	var out []string
	for _, btn := range kb[i] {
		if labels {
			out = append(out, btn.Text)
		} else {
			out = append(out, btn.Data)
		}
	}
	return strings.Join(out, " ")
}

// wizardRig drives one admin's chat and reports what each action sent.
type wizardRig struct {
	t   *testing.T
	b   *bot
	got func() []sent
}

func newWizardRig(t *testing.T) *wizardRig {
	b, got := testBot(t)
	return &wizardRig{t: t, b: b, got: got}
}

// do runs an action and returns the messages it caused.
func (r *wizardRig) do(action func()) []sent {
	n := len(r.got())
	action()
	return r.got()[n:]
}

func (r *wizardRig) press(data string) []sent {
	return r.do(func() { r.b.handle(context.Background(), callbackFrom(fullAdmin, data)) })
}

func (r *wizardRig) say(text string) []sent {
	return r.do(func() { r.b.handle(context.Background(), privateMessage(fullAdmin, text)) })
}

func (r *wizardRig) pending() *pending { return r.b.pend.get(fullAdmin) }

// wantStep fails unless the chat waits at the given wizard (or prompt) step.
func (r *wizardRig) wantStep(kind, key string) {
	r.t.Helper()
	p := r.pending()
	if p == nil || p.kind != kind || p.key != key {
		r.t.Fatalf("pending = %+v, want %s/%s", p, kind, key)
	}
}

// makeClient runs the whole wizard for a new ungrouped client and lets the
// caller answer the volume step.
func (r *wizardRig) makeClient(name string, volume func()) model.Client {
	r.t.Helper()
	r.press("c:new")
	r.say(name)
	r.press("w:grp:-")
	volume()
	r.press("w:days:0")
	r.press("w:ip:0")
	return loadByName(r.t, name)
}

func TestWizardVolumeKeyboardLayout(t *testing.T) {
	r := newWizardRig(t)
	r.press("c:new")
	r.say("layout")
	msgs := r.press("w:grp:-")
	kb := keyboardOf(msgs)
	for i, want := range []string{
		"w:vol:10 w:vol:20 w:vol:30",
		"w:vol:50 w:vol:100 w:vol:200",
		"w:vol:300 w:vol:0 w:vol:#c",
	} {
		if got := rowOf(kb, i, false); got != want {
			t.Fatalf("volume row %d = %q, want %q", i, got, want)
		}
	}
	for i, want := range []string{"10 20 30", "50 100 200", "300 ∞ ✏️ Custom"} {
		if got := rowOf(kb, i, true); got != want {
			t.Fatalf("volume row %d labels = %q, want %q", i, got, want)
		}
	}
	if !hasData(kb, "x:cancel") {
		t.Fatalf("the volume step lost its cancel button: %v", callbackData(kb))
	}
	if !strings.Contains(lastText(msgs, "editMessageText", "sendMessage"), "3/5") {
		t.Fatalf("volume step is not numbered 3/5: %q", lastText(msgs, "editMessageText", "sendMessage"))
	}
}

func TestWizardPresetVolumesAreInGigabytes(t *testing.T) {
	r := newWizardRig(t)
	for _, v := range []int64{10, 20, 30, 50, 100, 200, 300, 0} {
		name := "preset" + itoa(v)
		c := r.makeClient(name, func() { r.press("w:vol:" + itoa(v)) })
		if c.Volume != v*gib {
			t.Fatalf("button %d GB made a client with volume %d, want %d", v, c.Volume, v*gib)
		}
	}
}

func TestWizardCustomVolume(t *testing.T) {
	r := newWizardRig(t)
	r.press("c:new")
	r.say("custom")
	r.press("w:grp:-")

	// The custom button asks for a number and stays on the volume step.
	msgs := r.press("w:vol:#c")
	r.wantStep("wiz", "vol")
	if txt := lastText(msgs, "editMessageText", "sendMessage"); !strings.Contains(txt, "GB") {
		t.Fatalf("the custom prompt does not ask for GB: %q", txt)
	}
	kb := keyboardOf(msgs)
	if !hasData(kb, "w:vol:#b") || !hasData(kb, "x:cancel") || hasData(kb, "w:vol:10") {
		t.Fatalf("custom prompt keyboard = %v", callbackData(kb))
	}

	// Back shows the presets again, still on the same step.
	msgs = r.press("w:vol:#b")
	r.wantStep("wiz", "vol")
	if kb = keyboardOf(msgs); !hasData(kb, "w:vol:300") || !hasData(kb, "w:vol:#c") {
		t.Fatalf("presets did not come back: %v", callbackData(kb))
	}

	// A number that is not one keeps the step.
	r.press("w:vol:#c")
	msgs = r.say("lots")
	r.wantStep("wiz", "vol")
	if hasData(keyboardOf(msgs), "w:days:30") {
		t.Fatal("a bad volume moved the wizard on")
	}
	if p := r.pending(); p.data["vol"] != "" {
		t.Fatalf("a bad volume was stored: %q", p.data["vol"])
	}

	// A typed number moves on to the days step and ends up as the volume.
	msgs = r.say("75")
	r.wantStep("wiz", "days")
	if !hasData(keyboardOf(msgs), "w:days:30") {
		t.Fatalf("days step is missing: %v", callbackData(keyboardOf(msgs)))
	}
	r.press("w:days:30")
	r.press("w:ip:2")
	c := loadByName(t, "custom")
	if c.Volume != 75*gib || c.LimitIp != 2 || c.Expiry == 0 {
		t.Fatalf("custom client = volume %d, ip %d, expiry %d", c.Volume, c.LimitIp, c.Expiry)
	}
	if r.pending() != nil {
		t.Fatal("the wizard is still pending after creating the client")
	}

	// Decimals work, and 0 means unlimited.
	c = r.makeClient("fraction", func() { r.press("w:vol:#c"); r.say("2.5") })
	if want := int64(2.5 * float64(gib)); c.Volume != want {
		t.Fatalf("2.5 GB made volume %d, want %d", c.Volume, want)
	}
	c = r.makeClient("unlimited", func() { r.press("w:vol:#c"); r.say("0") })
	if c.Volume != 0 {
		t.Fatalf("0 GB made volume %d, want unlimited", c.Volume)
	}
}

func TestNewGroupButtonInTheWizard(t *testing.T) {
	r := newWizardRig(t)
	seedClient(t, "old1", "Old", 0, 0)

	r.press("c:new")
	msgs := r.say("grouped")
	r.wantStep("wiz", "grp")
	kb := keyboardOf(msgs)
	for _, want := range []string{"w:grp:0", "w:grp:-", "w:grp:#new", "x:cancel"} {
		if !hasData(kb, want) {
			t.Fatalf("group step lacks %s: %v", want, callbackData(kb))
		}
	}

	// The button asks for a name and stays on the group step.
	msgs = r.press("w:grp:#new")
	r.wantStep("wiz", "grp")
	if txt := lastText(msgs, "editMessageText", "sendMessage"); !strings.Contains(txt, "new group") {
		t.Fatalf("new group prompt = %q", txt)
	}
	if kb = keyboardOf(msgs); !hasData(kb, "w:grp:#b") || hasData(kb, "w:grp:0") {
		t.Fatalf("new group prompt keyboard = %v", callbackData(kb))
	}

	// Back shows the chooser again.
	if kb = keyboardOf(r.press("w:grp:#b")); !hasData(kb, "w:grp:#new") || !hasData(kb, "w:grp:0") {
		t.Fatalf("the chooser did not come back: %v", callbackData(kb))
	}

	// Bad names keep the step, a good one is used for the client.
	r.press("w:grp:#new")
	r.say(strings.Repeat("g", maxGroupRunes+1))
	r.wantStep("wiz", "grp")
	r.say("   ")
	r.wantStep("wiz", "grp")
	msgs = r.say("VIP team")
	r.wantStep("wiz", "vol")
	if p := r.pending(); p.data["grp"] != "VIP team" {
		t.Fatalf("group after typing = %q", p.data["grp"])
	}
	if !hasData(keyboardOf(msgs), "w:vol:300") {
		t.Fatal("the volume step did not follow the new group")
	}
	r.press("w:vol:10")
	r.press("w:days:0")
	r.press("w:ip:0")
	if c := loadByName(t, "grouped"); c.Group != "VIP team" || c.Volume != 10*gib {
		t.Fatalf("grouped client = group %q, volume %d", c.Group, c.Volume)
	}
	if groups := r.b.clientGroups(); indexOf(groups, "VIP team") < 0 || indexOf(groups, "Old") < 0 {
		t.Fatalf("groups = %v", groups)
	}

	// The next wizard offers the group that was just made.
	r.press("c:new")
	if kb = keyboardOf(r.say("second")); len(kb) == 0 || !strings.Contains(rowOf(kb, 1, true), "VIP team") && !strings.Contains(rowOf(kb, 0, true), "VIP team") {
		t.Fatalf("the new group is not offered: %v", kb)
	}
}

func TestNewGroupButtonInTheClientEditor(t *testing.T) {
	r := newWizardRig(t)
	c := seedClient(t, "ed1", "Old", 0, 0)
	seedClient(t, "ed2", "Old", 0, 0) // keeps the "Old" group alive when ed1 leaves it
	id := itoa(int64(c.Id))

	kb := keyboardOf(r.press("c:ask:grp:" + id))
	for _, want := range []string{"c:sg:" + id + ":0", "c:sg:" + id + ":-", "c:sg:" + id + ":#new", "x:cancel"} {
		if !hasData(kb, want) {
			t.Fatalf("group chooser lacks %s: %v", want, callbackData(kb))
		}
	}

	// The button asks for a name; the chat then waits for it.
	msgs := r.press("c:sg:" + id + ":#new")
	r.wantStep("cl.grp", "grp")
	if p := r.pending(); p.id != c.Id {
		t.Fatalf("pending client = %d, want %d", p.id, c.Id)
	}
	if txt := lastText(msgs, "editMessageText", "sendMessage"); !strings.Contains(txt, "new group") {
		t.Fatalf("new group prompt = %q", txt)
	}
	if kb = keyboardOf(msgs); !hasData(kb, "c:ask:grp:"+id) {
		t.Fatalf("no way back to the chooser: %v", callbackData(kb))
	}

	// Back to the chooser, and the button still works afterwards.
	if kb = keyboardOf(r.press("c:ask:grp:" + id)); !hasData(kb, "c:sg:"+id+":#new") {
		t.Fatalf("the chooser did not come back: %v", callbackData(kb))
	}
	r.press("c:sg:" + id + ":#new")
	r.wantStep("cl.grp", "grp")

	// A bad name leaves the client alone and keeps waiting.
	r.say(strings.Repeat("g", maxGroupRunes+1))
	r.wantStep("cl.grp", "grp")
	if g := loadByName(t, "ed1").Group; g != "Old" {
		t.Fatalf("a bad group name changed the group to %q", g)
	}

	msgs = r.say("Gold")
	if g := loadByName(t, "ed1").Group; g != "Gold" {
		t.Fatalf("group after typing = %q, want Gold", g)
	}
	if r.pending() != nil {
		t.Fatal("still waiting for a group name after it was set")
	}
	if txt := lastText(msgs, "editMessageText", "sendMessage"); !strings.Contains(txt, "Gold") {
		t.Fatalf("the client card does not show the new group: %q", txt)
	}
	if indexOf(r.b.clientGroups(), "Gold") < 0 {
		t.Fatalf("groups = %v", r.b.clientGroups())
	}

	// Existing groups can still be picked from the buttons.
	r.press("c:ask:grp:" + id)
	groups := r.b.clientGroups()
	r.press("c:sg:" + id + ":" + itoa(int64(indexOf(groups, "Old"))))
	if g := loadByName(t, "ed1").Group; g != "Old" {
		t.Fatalf("picking Old gave %q", g)
	}
}
