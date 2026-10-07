package api

import (
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	ApiService
	apiv2 *APIv2Handler
}

func NewAPIHandler(g *gin.RouterGroup, a2 *APIv2Handler) {
	a := &APIHandler{
		apiv2: a2,
	}
	a.initRouter(g)
}

func (a *APIHandler) initRouter(g *gin.RouterGroup) {
	g.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasSuffix(path, "login") && !strings.HasSuffix(path, "logout") {
			checkLogin(c)
		}
	})
	g.POST("/:postAction", a.postHandler)
	g.GET("/:getAction", a.getHandler)
}

func (a *APIHandler) postHandler(c *gin.Context) {
	loginUser := GetLoginUser(c)
	action := c.Param("postAction")

	switch action {
	case "login":
		a.ApiService.Login(c)
	case "changePass":
		a.ApiService.ChangePass(c)
	case "save":
		a.ApiService.Save(c, loginUser, true)
	case "restartApp":
		a.ApiService.RestartApp(c)
	case "restartSb":
		a.ApiService.RestartSb(c)
	case "maintenance":
		a.ApiService.SetMaintenance(c)
	case "resetTraffic":
		a.ApiService.ResetTraffic(c)
	case "linkConvert":
		a.ApiService.LinkConvert(c)
	case "subConvert":
		a.ApiService.SubConvert(c)
	case "importdb":
		a.ApiService.ImportDb(c)
	case "addToken":
		a.ApiService.AddToken(c)
		a.apiv2.ReloadTokens()
	case "deleteToken":
		a.ApiService.DeleteToken(c)
		a.apiv2.ReloadTokens()
	case "closeSessions":
		a.ApiService.CloseSessions(c)
	case "getCertPing":
		a.ApiService.GetCertPing(c)
	case "testNode":
		a.ApiService.TestNode(c)
	case "adoptInbounds":
		a.ApiService.AdoptInbounds(c, loginUser)
	case "reconcileNode":
		a.ApiService.ReconcileNode(c)
	case "nodeAction":
		a.ApiService.NodeAction(c, loginUser)
	case "shop":
		a.ApiService.SaveShop(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}

func (a *APIHandler) getHandler(c *gin.Context) {
	action := c.Param("getAction")

	switch action {
	case "logout":
		a.ApiService.Logout(c)
	case "load":
		a.ApiService.LoadData(c)
	case "inbounds", "outbounds", "endpoints", "services", "tls", "clients", "config", "nodes":
		err := a.ApiService.LoadPartialData(c, []string{action})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "users":
		a.ApiService.GetUsers(c)
	case "settings":
		a.ApiService.GetSettings(c)
	case "stats":
		a.ApiService.GetStats(c)
	case "status":
		a.ApiService.GetStatus(c)
	case "onlines":
		a.ApiService.GetClusterOnlines(c)
	case "sessions":
		a.ApiService.GetSessions(c)
	case "onlineIps":
		a.ApiService.GetOnlineIps(c)
	case "logs":
		a.ApiService.GetLogs(c)
	case "changes":
		a.ApiService.CheckChanges(c)
	case "keypairs":
		a.ApiService.GetKeypairs(c)
	case "getdb":
		a.ApiService.GetDb(c)
	case "tokens":
		a.ApiService.GetTokens(c)
	case "singbox-config":
		a.ApiService.GetSingboxConfig(c)
	case "checkOutbound":
		a.ApiService.GetCheckOutbound(c)
	case "nodeInbounds":
		a.ApiService.GetNodeInbounds(c)
	case "nodeOnlines":
		a.ApiService.GetNodeOnlines(c)
	case "nodeHistory":
		a.ApiService.GetNodeHistory(c)
	case "nodeOutages":
		a.ApiService.GetNodeOutages(c)
	case "nodeTraffic":
		a.ApiService.GetNodeTraffic(c)
	case "nodeLogs":
		a.ApiService.GetNodeLogs(c)
	case "nodeChanges":
		a.ApiService.GetNodeChanges(c)
	case "nodeOutbounds":
		a.ApiService.GetNodeOutbounds(c)
	case "nodeCheckOutbound":
		a.ApiService.GetNodeCheckOutbound(c)
	case "nodeSyncPreview":
		a.ApiService.GetNodeSyncPreview(c)
	case "nodeSyncReport":
		a.ApiService.GetNodeSyncReport(c)
	case "nodeBackup":
		a.ApiService.GetNodeBackup(c)
	case "nodesBackup":
		a.ApiService.GetNodesBackup(c)
	case "shop":
		a.ApiService.GetShop(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}
