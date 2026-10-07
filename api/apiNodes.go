package api

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

// The Nodes page's own endpoints: what the master knows about a node, and what
// it asks the node on the operator's behalf.

func nodeIDParam(c *gin.Context) (uint, error) {
	raw := c.Query("id")
	if raw == "" {
		raw = c.PostForm("id")
	}
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, common.NewError("invalid node id")
	}
	return uint(id), nil
}

// NodeAction runs an action on one node (id) or several (ids, a JSON list).
func (a *ApiService) NodeAction(c *gin.Context, actor string) {
	var ids []uint
	if raw := strings.TrimSpace(c.PostForm("ids")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			jsonMsg(c, "nodeAction", common.NewError("invalid node ids"))
			return
		}
	} else {
		id, err := nodeIDParam(c)
		if err != nil {
			jsonMsg(c, "nodeAction", err)
			return
		}
		ids = []uint{id}
	}
	results, err := a.NodeSyncService.NodeAction(ids, c.PostForm("action"), actor)
	if err != nil {
		jsonMsg(c, "nodeAction", err)
		return
	}
	jsonObj(c, results, nil)
}

func (a *ApiService) GetNodeOnlines(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeOnlines", err)
		return
	}
	jsonObj(c, a.NodeService.GetNodeOnlines(id), nil)
}

func (a *ApiService) GetNodeHistory(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeHistory", err)
		return
	}
	hours, _ := strconv.Atoi(c.Query("hours"))
	history, err := a.NodeService.GetNodeHistory(id, hours)
	jsonObj(c, history, err)
}

func (a *ApiService) GetNodeOutages(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeOutages", err)
		return
	}
	report, err := a.NodeService.GetNodeOutages(id)
	jsonObj(c, report, err)
}

func (a *ApiService) GetNodeTraffic(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeTraffic", err)
		return
	}
	period := c.Query("period")
	if period == "" {
		period = "24h"
	}
	report, err := a.NodeSyncService.GetTrafficReport(id, period)
	jsonObj(c, report, err)
}

func (a *ApiService) GetNodeLogs(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeLogs", err)
		return
	}
	count, _ := strconv.Atoi(c.Query("c"))
	logs, err := a.NodeSyncService.NodeLogs(id, count, c.Query("l"))
	jsonObj(c, logs, err)
}

func (a *ApiService) GetNodeChanges(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeChanges", err)
		return
	}
	count, _ := strconv.Atoi(c.Query("c"))
	changes, err := a.NodeSyncService.NodeChanges(id, c.Query("a"), c.Query("k"), count)
	jsonObj(c, changes, err)
}

func (a *ApiService) GetNodeOutbounds(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeOutbounds", err)
		return
	}
	outbounds, err := a.NodeSyncService.NodeOutbounds(id)
	jsonObj(c, outbounds, err)
}

func (a *ApiService) GetNodeCheckOutbound(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeCheckOutbound", err)
		return
	}
	result, err := a.NodeSyncService.NodeCheckOutbound(id, c.Query("tag"))
	jsonObj(c, result, err)
}

func (a *ApiService) GetNodeSyncPreview(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeSyncPreview", err)
		return
	}
	preview, err := a.NodeSyncService.PreviewSync(id)
	jsonObj(c, preview, err)
}

func (a *ApiService) GetNodeSyncReport(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeSyncReport", err)
		return
	}
	report, err := a.NodeSyncService.GetSyncReport(id)
	jsonObj(c, report, err)
}

// GetNodeBackup sends the database of one node, as the node made it.
func (a *ApiService) GetNodeBackup(c *gin.Context) {
	id, err := nodeIDParam(c)
	if err != nil {
		jsonMsg(c, "nodeBackup", err)
		return
	}
	node, data, err := a.NodeSyncService.NodeBackup(id, c.Query("exclude"))
	if err != nil {
		jsonMsg(c, "nodeBackup", err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", contentDisposition(service.NodeBackupName(node, time.Now())))
	c.Writer.Write(data)
}

// GetNodesBackup sends a zip with the master's database and those of all
// enabled nodes.
func (a *ApiService) GetNodesBackup(c *gin.Context) {
	write, err := a.NodeSyncService.BackupAll(c.Query("exclude"))
	if err != nil {
		jsonMsg(c, "nodesBackup", err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", contentDisposition("s-ui_cluster_"+time.Now().Format("20060102-150405")+".zip"))
	if err := write(c.Writer); err != nil {
		// The headers are gone: all that is left is to cut the zip short.
		logger.Warning("nodes: cluster backup: ", err)
	}
}

// contentDisposition names a download, with a plain ASCII name for old
// browsers and the real one, which may not be ASCII, for the others.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + url.PathEscape(name)
}
