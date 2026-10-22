package core

import (
	"context"
	"testing"

	"github.com/sagernet/sing-box/option"
)

// The Cloudflared inbound's dialers, set from the panel as a detour only,
// are read by sing-box.
func TestCloudflaredDialerDetour(t *testing.T) {
	ctx := Context(context.Background(), InboundRegistry(), OutboundRegistry(),
		EndpointRegistry(), DNSTransportRegistry(), ServiceRegistry(), CertificateProviderRegistry())
	raw := `{"inbounds":[{"type":"cloudflared","tag":"cf","token":"x","control_dialer":{"detour":"warp"},"tunnel_dialer":{"detour":"warp2"}}]}`
	var opts option.Options
	if err := opts.UnmarshalJSONContext(ctx, []byte(raw)); err != nil {
		t.Skip("cloudflared is not built in: ", err)
	}
	cf, ok := opts.Inbounds[0].Options.(*option.CloudflaredInboundOptions)
	if !ok {
		t.Fatalf("options = %T", opts.Inbounds[0].Options)
	}
	if cf.ControlDialer.Detour != "warp" || cf.TunnelDialer.Detour != "warp2" {
		t.Fatalf("dialers = %+v / %+v", cf.ControlDialer, cf.TunnelDialer)
	}
}
