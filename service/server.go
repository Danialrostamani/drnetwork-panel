package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/logger"

	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ServerService struct{}

func (s *ServerService) GetStatus(request string) *map[string]interface{} {
	status := make(map[string]interface{}, 0)
	requests := strings.Split(request, ",")
	for _, req := range requests {
		switch req {
		case "cpu":
			status["cpu"] = s.GetCpuPercent()
		case "mem":
			status["mem"] = s.GetMemInfo()
		case "dsk":
			status["dsk"] = s.GetDiskInfo()
		case "dio":
			status["dio"] = s.GetDiskIO()
		case "swp":
			status["swp"] = s.GetSwapInfo()
		case "net":
			status["net"] = s.GetNetInfo()
		case "nic":
			status["nic"] = s.GetNicInfo()
		case "sys":
			status["sys"] = s.GetSystemInfo()
		case "sbd":
			status["sbd"] = s.GetSingboxInfo()
		case "db":
			status["db"] = s.GetDatabaseInfo()
		}
	}
	return &status
}

func (s *ServerService) GetCpuPercent() float64 {
	percents, err := cpu.Percent(0, false)
	if err != nil {
		logger.Warning("get cpu percent failed:", err)
		return 0
	} else {
		return percents[0]
	}
}

func (s *ServerService) GetMemInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		logger.Warning("get virtual memory failed:", err)
	} else {
		info["current"] = memInfo.Used
		info["total"] = memInfo.Total
	}
	return info
}

func (s *ServerService) GetDiskInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	diskInfo, err := disk.Usage("/")
	if err != nil {
		logger.Warning("get disk usage failed:", err)
	} else {
		info["current"] = diskInfo.Used
		info["total"] = diskInfo.Total
	}
	return info
}

func (s *ServerService) GetDiskIO() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	ioStats, err := disk.IOCounters()
	if err != nil {
		logger.Warning("get disk io counters failed:", err)
	} else if len(ioStats) > 0 {
		infoR, infoW := uint64(0), uint64(0)
		for _, ioStat := range ioStats {
			infoR += ioStat.ReadBytes
			infoW += ioStat.WriteBytes
		}
		info["read"] = infoR
		info["write"] = infoW
	} else {
		logger.Warning("can not find disk io counters")
	}
	return info
}

func (s *ServerService) GetSwapInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	swapInfo, err := mem.SwapMemory()
	if err != nil {
		logger.Warning("get swap memory failed:", err)
	} else {
		info["current"] = swapInfo.Used
		info["total"] = swapInfo.Total
	}
	return info
}

func (s *ServerService) GetNetInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	ioStats, err := net.IOCounters(false)
	if err != nil {
		logger.Warning("get io counters failed:", err)
	} else if len(ioStats) > 0 {
		ioStat := ioStats[0]
		info["sent"] = ioStat.BytesSent
		info["recv"] = ioStat.BytesRecv
		info["psent"] = ioStat.PacketsSent
		info["precv"] = ioStat.PacketsRecv
	} else {
		logger.Warning("can not find io counters")
	}
	return info
}

// GetNicInfo is what the server's own network interfaces sent and received,
// in total and per interface. Loopback and virtual interfaces (containers,
// bridges, tunnels) are left out: their bytes cross a real interface too, and
// would count twice.
func (s *ServerService) GetNicInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	ioStats, err := net.IOCounters(true)
	if err != nil {
		logger.Warning("get io counters failed:", err)
		return info
	}
	loopback := map[string]bool{}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			for _, flag := range iface.Flags {
				if flag == "loopback" {
					loopback[iface.Name] = true
				}
			}
		}
	}
	var sent, recv uint64
	ifs := map[string][2]uint64{}
	for _, st := range ioStats {
		if loopback[st.Name] || isVirtualNic(st.Name) {
			continue
		}
		sent += st.BytesSent
		recv += st.BytesRecv
		ifs[st.Name] = [2]uint64{st.BytesSent, st.BytesRecv}
	}
	info["sent"] = sent
	info["recv"] = recv
	info["ifs"] = ifs
	return info
}

var virtualNicPrefixes = []string{"loopback", "docker", "veth", "br-", "virbr", "vnet", "tun", "tap", "wg", "tailscale", "zt", "utun", "cni", "flannel", "cali", "vxlan", "kube", "ifb", "dummy", "sing", "warp", "cloudflarewarp"}

// isVirtualNic reports whether an interface name is loopback or one of the
// virtual interfaces containers, VPNs and tunnels create.
func isVirtualNic(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "lo" {
		return true
	}
	if rest, ok := strings.CutPrefix(n, "lo"); ok && rest != "" && strings.Trim(rest, "0123456789") == "" {
		return true
	}
	for _, p := range virtualNicPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

var (
	singboxVersionOnce sync.Once
	singboxVersionText string
)

// singboxVersion is the version of the sing-box the panel is built with. The
// constant is only set by sing-box's own build, so the module's version is
// the fallback.
func singboxVersion() string {
	singboxVersionOnce.Do(func() {
		if v := strings.TrimSpace(C.Version); v != "" && v != "unknown" {
			singboxVersionText = strings.TrimPrefix(v, "v")
			return
		}
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		for _, dep := range info.Deps {
			if dep.Path == "github.com/sagernet/sing-box" {
				v := dep.Version
				if dep.Replace != nil && dep.Replace.Version != "" {
					v = dep.Replace.Version
				}
				singboxVersionText = strings.TrimPrefix(v, "v")
				return
			}
		}
	})
	return singboxVersionText
}

func (s *ServerService) GetSingboxInfo() map[string]interface{} {
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	// One GetInstance, then a nil check. Reading IsRunning and dereferencing
	// GetInstance separately let a stop land in between, and this is the
	// endpoint the dashboard polls every few seconds.
	box := corePtr.GetInstance()
	isRunning := box != nil
	uptime := uint32(0)
	if isRunning {
		uptime = box.Uptime()
	}
	return map[string]interface{}{
		"running": isRunning,
		"version": singboxVersion(),
		"stats": map[string]interface{}{
			"NumGoroutine": uint32(runtime.NumGoroutine()),
			"Alloc":        rtm.Alloc,
			"Uptime":       uptime,
		},
	}
}

func (s *ServerService) GetSystemInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)

	info["appMem"] = rtm.Sys
	info["appThreads"] = uint32(runtime.NumGoroutine())
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		info["cpuType"] = cpuInfo[0].ModelName
	}
	info["cpuCount"] = runtime.NumCPU()
	info["hostName"], _ = os.Hostname()
	info["appVersion"] = config.GetVersion()
	info["appFull"] = config.GetFullVersion()
	ipv4 := make([]string, 0)
	ipv6 := make([]string, 0)
	// get ip address
	netInterfaces, _ := net.Interfaces()
	for i := 0; i < len(netInterfaces); i++ {
		if len(netInterfaces[i].Flags) > 2 && netInterfaces[i].Flags[0] == "up" && netInterfaces[i].Flags[1] != "loopback" {
			addrs := netInterfaces[i].Addrs

			for _, address := range addrs {
				if strings.Contains(address.Addr, ".") {
					ipv4 = append(ipv4, address.Addr)
				} else if !strings.HasPrefix(address.Addr, "fe80::") {
					ipv6 = append(ipv6, address.Addr)
				}
			}
		}
	}
	info["ipv4"] = ipv4
	info["ipv6"] = ipv6
	info["bootTime"], _ = host.BootTime()

	return info
}

func (s *ServerService) GetLogs(count string, level string) []string {
	c, err := strconv.Atoi(count)
	if err != nil {
		c = 10
	}
	return logger.GetLogs(c, level)
}

func (s *ServerService) GenKeypair(keyType string, options string) []string {
	if len(keyType) == 0 {
		return []string{"No keypair to generate"}
	}

	switch keyType {
	case "ech":
		return s.generateECHKeyPair(options)
	case "tls":
		return s.generateTLSKeyPair(options)
	case "reality":
		return s.generateRealityKeyPair()
	case "wireguard":
		return s.generateWireGuardKey(options)
	case "openvpn":
		return s.generateOpenVPNStaticKey()
	}

	return []string{"Failed to generate keypair"}
}

func (s *ServerService) generateECHKeyPair(serverName string) []string {
	configPem, keyPem, err := tls.ECHKeygenDefault(serverName)
	if err != nil {
		return []string{"Failed to generate ECH keypair: ", err.Error()}
	}
	return append(strings.Split(configPem, "\n"), strings.Split(keyPem, "\n")...)
}

func (s *ServerService) generateTLSKeyPair(serverName string) []string {
	privateKeyPem, publicKeyPem, err := tls.GenerateCertificate(nil, nil, time.Now, serverName, time.Now().AddDate(0, 12, 0))
	if err != nil {
		return []string{"Failed to generate TLS keypair: ", err.Error()}
	}
	return append(strings.Split(string(privateKeyPem), "\n"), strings.Split(string(publicKeyPem), "\n")...)
}

// generateOpenVPNStaticKey produces what `openvpn --genkey secret` writes: 256
// random bytes as hex between OpenVPN's own markers. tls-auth and tls-crypt
// take one of these rather than a PEM, and both ends of a tunnel have to carry
// the same one.
func (s *ServerService) generateOpenVPNStaticKey() []string {
	material := make([]byte, 256)
	if _, err := rand.Read(material); err != nil {
		return []string{"Failed to generate OpenVPN static key: ", err.Error()}
	}
	encoded := hex.EncodeToString(material)

	lines := []string{
		"#",
		"# 2048 bit OpenVPN static key",
		"#",
		"-----BEGIN OpenVPN Static key V1-----",
	}
	for offset := 0; offset < len(encoded); offset += 32 {
		lines = append(lines, encoded[offset:offset+32])
	}
	return append(lines, "-----END OpenVPN Static key V1-----")
}

func (s *ServerService) generateRealityKeyPair() []string {
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return []string{"Failed to generate Reality keypair: ", err.Error()}
	}
	publicKey := privateKey.PublicKey()
	return []string{"PrivateKey: " + base64.RawURLEncoding.EncodeToString(privateKey[:]), "PublicKey: " + base64.RawURLEncoding.EncodeToString(publicKey[:])}
}

func (s *ServerService) generateWireGuardKey(pk string) []string {
	if len(pk) > 0 {
		key, _ := wgtypes.ParseKey(pk)
		return []string{key.PublicKey().String()}
	}
	wgKeys, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return []string{"Failed to generate wireguard keypair: ", err.Error()}
	}
	return []string{"PrivateKey: " + wgKeys.String(), "PublicKey: " + wgKeys.PublicKey().String()}
}

func (s *ServerService) GetDatabaseInfo() map[string]int64 {
	info := make(map[string]int64, 0)
	db := database.GetDB()
	if db == nil {
		return nil
	}

	var clientsCount, inboundsCount, outboundsCount, servicesCount, endpointsCount, clientUp, clientDown int64

	db.Model(&model.Client{}).Count(&clientsCount)
	db.Model(&model.Inbound{}).Count(&inboundsCount)
	db.Model(&model.Outbound{}).Count(&outboundsCount)
	db.Model(&model.Service{}).Count(&servicesCount)
	db.Model(&model.Endpoint{}).Count(&endpointsCount)
	db.Model(&model.Client{}).Select("COALESCE(SUM(up+total_up),0)").Scan(&clientUp)
	db.Model(&model.Client{}).Select("COALESCE(SUM(down+total_down),0)").Scan(&clientDown)

	info["clients"] = clientsCount
	info["inbounds"] = inboundsCount
	info["outbounds"] = outboundsCount
	info["services"] = servicesCount
	info["endpoints"] = endpointsCount
	info["clientUp"] = clientUp
	info["clientDown"] = clientDown

	return info
}
