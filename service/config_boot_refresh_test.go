package service

import (
	"encoding/json"
	"testing"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
)

func TestConfigNeedsRoutingRebuild(t *testing.T) {
	direct := json.RawMessage(`{"type":"direct","tag":"direct"}`)
	socks := json.RawMessage(`{"type":"socks","tag":"warp-like","server":"127.0.0.1","server_port":1}`)
	endpoint := json.RawMessage(`{"type":"wireguard","tag":"wg"}`)

	tests := []struct {
		name string
		cfg  SingBoxConfig
		want bool
	}{
		{name: "empty", cfg: SingBoxConfig{}, want: false},
		{name: "seeded direct", cfg: SingBoxConfig{Outbounds: []json.RawMessage{direct}}, want: false},
		{name: "direct with bind", cfg: SingBoxConfig{Outbounds: []json.RawMessage{json.RawMessage(`{"type":"direct","tag":"direct","bind_interface":"eth0"}`)}}, want: true},
		{name: "extra outbound", cfg: SingBoxConfig{Outbounds: []json.RawMessage{direct, socks}}, want: true},
		{name: "only socks", cfg: SingBoxConfig{Outbounds: []json.RawMessage{socks}}, want: true},
		{name: "endpoint", cfg: SingBoxConfig{Outbounds: []json.RawMessage{direct}, Endpoints: []json.RawMessage{endpoint}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := configNeedsRoutingRebuild(&test.cfg); got != test.want {
				t.Fatalf("configNeedsRoutingRebuild = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRebuildCoreForRoutingNoopsWhenDown(t *testing.T) {
	s := lifecycleService(t)
	if err := s.RebuildCoreForRouting(); err != nil {
		t.Fatalf("RebuildCoreForRouting on a down core: %v", err)
	}
	if corePtr.IsRunning() {
		t.Fatal("RebuildCoreForRouting started a core that was down")
	}
}

func TestRebuildCoreForRoutingNoopsInMaintenance(t *testing.T) {
	s := lifecycleService(t)
	if err := s.SettingService.SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildCoreForRouting(); err != nil {
		t.Fatalf("RebuildCoreForRouting in maintenance: %v", err)
	}
}

func TestRebuildCoreForRoutingSkipsSeededDirect(t *testing.T) {
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	s := lifecycleService(t)
	if err := s.SettingService.SetConfig(`{"log":{"level":"error"}}`); err != nil {
		t.Fatal(err)
	}
	if err := s.StartCore(); err != nil {
		t.Fatalf("StartCore: %v", err)
	}
	t.Cleanup(func() { _ = corePtr.Stop() })
	before := corePtr.GetInstance()
	if err := s.RebuildCoreForRouting(); err != nil {
		t.Fatalf("RebuildCoreForRouting: %v", err)
	}
	if corePtr.GetInstance() != before {
		t.Fatal("seeded direct outbound rebuilt the core")
	}
}

func TestRebuildCoreForRoutingRebuildsExtraOutbound(t *testing.T) {
	if raceEnabled {
		t.Skip("starting a Box trips sing-box's own race in route.NetworkManager; see race_on_test.go")
	}
	s := lifecycleService(t)
	if err := s.SettingService.SetConfig(`{"log":{"level":"error"}}`); err != nil {
		t.Fatal(err)
	}
	outbound := model.Outbound{Type: "socks", Tag: "proxy", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1}`)}
	if err := database.GetDB().Create(&outbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.StartCore(); err != nil {
		t.Fatalf("StartCore: %v", err)
	}
	t.Cleanup(func() { _ = corePtr.Stop() })
	before := corePtr.GetInstance()
	if err := s.RebuildCoreForRouting(); err != nil {
		t.Fatalf("RebuildCoreForRouting: %v", err)
	}
	if !corePtr.IsRunning() {
		t.Fatal("rebuild left the core down")
	}
	if corePtr.GetInstance() == before {
		t.Fatal("extra outbound did not rebuild the core")
	}
}
