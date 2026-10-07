package model

import "strings"

// NodeAlerts are the thresholds a node's Telegram alerts fire at. A nil value
// takes the default; zero turns that alert off.
type NodeAlerts struct {
	// Percent of CPU, memory and disk in use.
	Cpu  *int `json:"cpu"`
	Mem  *int `json:"mem"`
	Disk *int `json:"disk"`
	// Round trip of the probe, in milliseconds.
	Ping *int `json:"ping"`
	// Days left on the certificate of the node panel's HTTPS address.
	CertDays *int `json:"certDays"`
	// Warn when the node runs an older release than the master.
	Version *bool `json:"version"`
}

// Defaults of NodeAlerts.
const (
	DefaultAlertCpu      = 90
	DefaultAlertMem      = 90
	DefaultAlertDisk     = 90
	DefaultAlertPing     = 0
	DefaultAlertCertDays = 7
)

func alertValue(v *int, def int) int {
	if v == nil {
		return def
	}
	if *v < 0 {
		return 0
	}
	return *v
}

func (a NodeAlerts) CpuLimit() int      { return alertValue(a.Cpu, DefaultAlertCpu) }
func (a NodeAlerts) MemLimit() int      { return alertValue(a.Mem, DefaultAlertMem) }
func (a NodeAlerts) DiskLimit() int     { return alertValue(a.Disk, DefaultAlertDisk) }
func (a NodeAlerts) PingLimit() int     { return alertValue(a.Ping, DefaultAlertPing) }
func (a NodeAlerts) CertDaysLimit() int { return alertValue(a.CertDays, DefaultAlertCertDays) }
func (a NodeAlerts) VersionCheck() bool { return a.Version == nil || *a.Version }

// Clone is a copy that shares no pointer with a.
func (a NodeAlerts) Clone() NodeAlerts {
	cp := func(v *int) *int {
		if v == nil {
			return nil
		}
		x := *v
		return &x
	}
	out := NodeAlerts{Cpu: cp(a.Cpu), Mem: cp(a.Mem), Disk: cp(a.Disk), Ping: cp(a.Ping), CertDays: cp(a.CertDays)}
	if a.Version != nil {
		v := *a.Version
		out.Version = &v
	}
	return out
}

// NodeCap is the traffic a node's server may move in one monthly cycle, the
// way a VPS plan counts it.
type NodeCap struct {
	// Bytes per cycle; zero means no cap.
	Limit int64 `json:"limit"`
	// Day of the month the cycle starts on, 1 to 31; a month too short for it
	// starts the cycle on its last day.
	Day int `json:"day"`
	// What counts: "total" (sent and received), "up" (sent) or "down" (received).
	Mode string `json:"mode"`
	// Hide leaves the node's links out of subscriptions once the cap is used up.
	Hide bool `json:"hide"`
}

// Counted is the part of a server's traffic the cap counts.
func (c NodeCap) Counted(up, down int64) int64 {
	switch c.Mode {
	case "up":
		return up
	case "down":
		return down
	default:
		return up + down
	}
}

// ResetDay is Day kept between 1 and 31.
func (c NodeCap) ResetDay() int {
	if c.Day < 1 {
		return 1
	}
	if c.Day > 31 {
		return 31
	}
	return c.Day
}

// NodeAccess limits a node to some clients. Empty, the node serves every client
// that has one of its inbounds; otherwise only the clients of these groups and
// these client IDs.
type NodeAccess struct {
	Groups  []string `json:"groups"`
	Clients []uint   `json:"clients"`
}

// Restricted reports whether the node serves only some clients.
func (a NodeAccess) Restricted() bool {
	return len(a.Groups) > 0 || len(a.Clients) > 0
}

// Allows reports whether the node serves the client with this ID and group.
// Group names compare without regard to case or surrounding spaces, as
// everywhere else in the panel.
func (a NodeAccess) Allows(id uint, group string) bool {
	if !a.Restricted() {
		return true
	}
	for _, c := range a.Clients {
		if c == id {
			return true
		}
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for _, g := range a.Groups {
		if strings.EqualFold(strings.TrimSpace(g), group) {
			return true
		}
	}
	return false
}

// NodeMetric is one minute of a node's probes: the source of its history
// charts and of its uptime.
type NodeMetric struct {
	Id       uint64 `json:"-" gorm:"primaryKey;autoIncrement"`
	NodeId   uint   `json:"-" gorm:"uniqueIndex:idx_node_metric,priority:1;not null"`
	DateTime int64  `json:"t" gorm:"uniqueIndex:idx_node_metric,priority:2;index:idx_node_metric_time;not null"`
	// Probes made in the minute, and how many of them found the node online.
	Probes int `json:"probes"`
	Up     int `json:"up"`
	// Averages over the probes that found the node online.
	Latency int64   `json:"latency"`
	Cpu     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Disk    float64 `json:"disk"`
	// Most users online at once.
	Online int `json:"online"`
	// Bytes the server's network interfaces sent and received in the minute.
	Sent int64 `json:"sent"`
	Recv int64 `json:"recv"`
}

// NodeOutage is one stretch of time a node was not online.
type NodeOutage struct {
	Id     uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId uint   `json:"-" gorm:"index;not null"`
	Start  int64  `json:"start" gorm:"column:start_at"`
	// Zero while the outage lasts.
	End int64 `json:"end" gorm:"column:end_at;index"`
	// "offline" or "core-stopped", and the error the probe saw last.
	State  string `json:"state"`
	Reason string `json:"reason"`
	// The last probe that still found the node down. A master that stops
	// during an outage ends it here when it starts again.
	Checked int64 `json:"-"`
}

// NodeTraffic is what a node's network interfaces moved in one hour, local
// time. The row at hour zero holds the hours too old to keep one by one.
type NodeTraffic struct {
	Id       uint64 `json:"-" gorm:"primaryKey;autoIncrement"`
	NodeId   uint   `json:"-" gorm:"uniqueIndex:idx_node_traffic,priority:1;not null"`
	DateTime int64  `json:"t" gorm:"uniqueIndex:idx_node_traffic,priority:2;not null"`
	Up       int64  `json:"up"`
	Down     int64  `json:"down"`
}

// TableName keeps the name the queries written out in SQL use; the default
// would be node_traffics.
func (NodeTraffic) TableName() string { return "node_traffic" }
