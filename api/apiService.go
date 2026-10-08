package api

import (
	"encoding/json"
	"runtime"
	"strconv"
	"time"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

type ApiService struct {
	service.SettingService
	service.UserService
	service.ConfigService
	service.ClientService
	service.TlsService
	service.InboundService
	service.OutboundService
	service.EndpointService
	service.ServicesService
	service.PanelService
	service.StatsService
	service.ServerService
	service.NodeService
	service.NodeSyncService
}

func (a *ApiService) LoadData(c *gin.Context) {
	data, err := a.getData(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ApiService) getData(c *gin.Context) (interface{}, error) {
	data := make(map[string]interface{}, 0)
	lu := c.Query("lu")
	isUpdated, err := a.ConfigService.CheckChanges(lu)
	if err != nil {
		return "", err
	}
	onlines, err := a.StatsService.GetClusterOnlines()
	if err != nil {
		return "", err
	}
	clientTraffic, err := a.ClientService.GetTrafficSnapshot()
	if err != nil {
		return "", err
	}

	// Carried on every poll so the panel can keep saying the core is down on
	// purpose, wherever the operator happens to be looking.
	maintenance, mErr := a.SettingService.GetMaintenance()
	if mErr != nil {
		logger.Warning("unable to read maintenance setting:", mErr)
	}
	data["maintenance"] = maintenance
	data["nodesStatus"] = a.NodeService.GetStatuses()
	data["ipCounts"] = service.GetIPCounts()
	data["clusterIpActive"] = service.ClusterIPActive()
	data["clientTraffic"] = clientTraffic

	sysInfo := a.ServerService.GetSingboxInfo()
	// A core stopped on purpose is not a failure to report; without this the
	// panel raises the last log as an error on every poll of a quiet system.
	if sysInfo["running"] == false && !maintenance {
		logs := a.ServerService.GetLogs("1", "debug")
		if len(logs) > 0 {
			data["lastLog"] = logs[0]
		}
	}

	if isUpdated {
		config, err := a.SettingService.GetConfig()
		if err != nil {
			return "", err
		}
		clients, err := a.ClientService.GetAll()
		if err != nil {
			return "", err
		}
		tlsConfigs, err := a.TlsService.GetAll()
		if err != nil {
			return "", err
		}
		inbounds, err := a.InboundService.GetAll()
		if err != nil {
			return "", err
		}
		outbounds, err := a.OutboundService.GetAll()
		if err != nil {
			return "", err
		}
		endpoints, err := a.EndpointService.GetAll()
		if err != nil {
			return "", err
		}
		services, err := a.ServicesService.GetAll()
		if err != nil {
			return "", err
		}
		nodes, err := a.NodeService.GetAll()
		if err != nil {
			return "", err
		}
		subURI, err := a.SettingService.GetFinalSubURI(getHostname(c))
		if err != nil {
			return "", err
		}
		trafficAge, err := a.SettingService.GetTrafficAge()
		if err != nil {
			return "", err
		}
		data["config"] = json.RawMessage(config)
		data["clients"] = clients
		data["tls"] = tlsConfigs
		data["inbounds"] = inbounds
		data["outbounds"] = outbounds
		data["endpoints"] = endpoints
		data["services"] = services
		data["nodes"] = nodes
		data["subURI"] = subURI
		data["enableTraffic"] = trafficAge > 0
		data["onlines"] = onlines
		data["os"] = runtime.GOOS
	} else {
		data["onlines"] = onlines
	}

	return data, nil
}

func (a *ApiService) LoadPartialData(c *gin.Context, objs []string) error {
	data := make(map[string]interface{}, 0)
	id := c.Query("id")

	for _, obj := range objs {
		switch obj {
		case "inbounds":
			inbounds, err := a.InboundService.Get(id)
			if err != nil {
				return err
			}
			data[obj] = inbounds
		case "outbounds":
			outbounds, err := a.OutboundService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = outbounds
		case "endpoints":
			endpoints, err := a.EndpointService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = endpoints
		case "services":
			services, err := a.ServicesService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = services
		case "tls":
			tlsConfigs, err := a.TlsService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = tlsConfigs
		case "clients":
			var clients interface{}
			var err error
			if id == "" && c.Query("full") == "1" {
				clients, err = a.ClientService.GetAllWithConfig()
			} else {
				clients, err = a.ClientService.Get(id)
			}
			if err != nil {
				return err
			}
			data[obj] = clients
		case "config":
			config, err := a.SettingService.GetConfig()
			if err != nil {
				return err
			}
			data[obj] = json.RawMessage(config)
		case "nodes":
			nodes, err := a.NodeService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = nodes
		case "settings":
			settings, err := a.SettingService.GetAllSetting()
			if err != nil {
				return err
			}
			data[obj] = settings
		}
	}

	jsonObj(c, data, nil)
	return nil
}

func (a *ApiService) GetUsers(c *gin.Context) {
	users, err := a.UserService.GetUsers()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, *users, nil)
}

func (a *ApiService) GetSettings(c *gin.Context) {
	data, err := a.SettingService.GetAllSetting()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, err)
}

func (a *ApiService) GetStats(c *gin.Context) {
	resource := c.Query("resource")
	tag := c.Query("tag")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 100
	}
	start, _ := strconv.ParseInt(c.Query("start"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end"), 10, 64)
	data, err := a.StatsService.GetStats(resource, tag, limit, start, end)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, err)
}

// GetStatsSummary answers what the master asks every node for its Telegram
// bot's Stats screen: what the stats table counted since a moment, per user and
// inbound, and per bucket seconds for the inbounds.
func (a *ApiService) GetStatsSummary(c *gin.Context) {
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	bucket, _ := strconv.ParseInt(c.Query("bucket"), 10, 64)
	data, err := a.StatsService.GetSummary(since, bucket)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ApiService) GetStatus(c *gin.Context) {
	request := c.Query("r")
	result := a.ServerService.GetStatus(request)
	// A core that is down on purpose looks exactly like a core that crashed,
	// and the panel has to tell the operator which one it is looking at.
	if sbd, ok := (*result)["sbd"].(map[string]interface{}); ok {
		maintenance, err := a.SettingService.GetMaintenance()
		if err != nil {
			logger.Warning("unable to read maintenance setting:", err)
		}
		sbd["maintenance"] = maintenance
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetOnlines(c *gin.Context) {
	onlines, err := a.StatsService.GetOnlines()
	jsonObj(c, onlines, err)
}

func (a *ApiService) GetClusterOnlines(c *gin.Context) {
	onlines, err := a.StatsService.GetClusterOnlines()
	jsonObj(c, onlines, err)
}

func (a *ApiService) GetOnlineIps(c *gin.Context) {
	name := c.Query("name")
	problems := service.ClusterIPProblems()
	if ips, ok := service.ClusterOnlineIPsOf(name); ok {
		jsonObj(c, gin.H{"ips": ips, "problems": problems}, nil)
		return
	}
	jsonObj(c, gin.H{"ips": service.OnlineIPsOf(name), "problems": problems}, nil)
}

func (a *ApiService) GetClusterIps(c *gin.Context) {
	ips, err := service.ClusterIPSnapshot()
	jsonObj(c, ips, err)
}

func (a *ApiService) ApplyClusterBans(c *gin.Context) {
	var bans map[string][]string
	if err := json.Unmarshal([]byte(c.PostForm("data")), &bans); err != nil {
		jsonMsg(c, "clusterBans", err)
		return
	}
	jsonMsg(c, "clusterBans", service.ApplyClusterBans(bans))
}

// ApplyNodeQuotas is the master telling this node how much each of its
// clients may still use here.
func (a *ApiService) ApplyNodeQuotas(c *gin.Context) {
	var totals map[string]int64
	if err := json.Unmarshal([]byte(c.PostForm("data")), &totals); err != nil {
		jsonMsg(c, "quota", err)
		return
	}
	jsonMsg(c, "quota", service.ApplyNodeQuotas(totals))
}

func (a *ApiService) GetSessions(c *gin.Context) {
	resource := c.Query("resource")
	if resource == "" {
		resource = "user"
	}
	sessions, err := a.StatsService.GetSessions(resource, c.Query("tag"))
	jsonObj(c, sessions, err)
}

func (a *ApiService) CloseSessions(c *gin.Context) {
	user := c.PostForm("u")
	if user == "" {
		user = c.Query("u")
	}
	err := a.StatsService.CloseUserSessions(user)
	jsonMsg(c, "closeSessions", err)
}

func (a *ApiService) GetLogs(c *gin.Context) {
	count := c.Query("c")
	level := c.Query("l")
	logs := a.ServerService.GetLogs(count, level)
	jsonObj(c, logs, nil)
}

func (a *ApiService) CheckChanges(c *gin.Context) {
	actor := c.Query("a")
	chngKey := c.Query("k")
	count := c.Query("c")
	changes := a.ConfigService.GetChanges(actor, chngKey, count)
	jsonObj(c, changes, nil)
}

func (a *ApiService) GetKeypairs(c *gin.Context) {
	kType := c.Query("k")
	options := c.Query("o")
	keypair := a.ServerService.GenKeypair(kType, options)
	jsonObj(c, keypair, nil)
}

func (a *ApiService) GetDb(c *gin.Context) {
	exclude := c.Query("exclude")
	db, err := database.GetDb(exclude)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=s-ui_"+time.Now().Format("20060102-150405")+".db")
	c.Writer.Write(db)
}

func (a *ApiService) postActions(c *gin.Context) (string, json.RawMessage, error) {
	var data map[string]json.RawMessage
	err := c.ShouldBind(&data)
	if err != nil {
		return "", nil, err
	}
	return string(data["action"]), data["data"], nil
}

func (a *ApiService) Login(c *gin.Context) {
	remoteIP := getRemoteIp(c)
	loginUser, err := a.UserService.LoginWithCode(c.Request.FormValue("user"), c.Request.FormValue("pass"), c.Request.FormValue("code"), remoteIP)
	if err == service.ErrTotpRequired {
		jsonMsgObj(c, "", map[string]bool{"totp": true}, err)
		return
	}
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	sessionMaxAge, err := a.SettingService.GetSessionMaxAge()
	if err != nil {
		logger.Infof("Unable to get session's max age from DB")
	}

	if err = SetLoginUser(c, loginUser, sessionMaxAge); err != nil {
		// Reported, not logged and swallowed: the old code answered "success"
		// with no cookie set, so the panel bounced straight back to the login
		// form with nothing to explain why.
		logger.Warning("login failed to start a session: ", err)
		jsonMsg(c, "", err)
		return
	}
	logger.Info("user ", loginUser, " login success")

	jsonMsg(c, "", nil)
}

func (a *ApiService) ChangePass(c *gin.Context) {
	loginUser := GetLoginUser(c)
	oldPass := c.Request.FormValue("oldPass")
	newUsername := c.Request.FormValue("newUsername")
	newPass := c.Request.FormValue("newPass")
	err := a.UserService.ChangePass(loginUser, oldPass, newUsername, newPass)
	if err == nil {
		logger.Info("change user credentials success")
		jsonMsg(c, "save", nil)
	} else {
		logger.Warning("change user credentials failed:", err)
		jsonMsg(c, "", err)
	}
}

func (a *ApiService) Save(c *gin.Context, loginUser string, fanout bool) {
	hostname := getHostname(c)
	obj := c.Request.FormValue("object")
	act := c.Request.FormValue("action")
	data := c.Request.FormValue("data")
	initUsers := c.Request.FormValue("initUsers")
	objs, err := a.ConfigService.Save(obj, act, json.RawMessage(data), initUsers, loginUser, hostname)
	if err != nil {
		jsonMsg(c, "save", err)
		return
	}
	if fanout && (obj == "clients" || obj == "inbounds") {
		a.NodeSyncService.MarkAllDirty()
		go a.NodeSyncService.ReconcileDirtyOnline()
	}
	err = a.LoadPartialData(c, objs)
	if err != nil {
		jsonMsg(c, obj, err)
	}
}

func (a *ApiService) RestartApp(c *gin.Context) {
	err := a.PanelService.RestartPanel(3 * time.Second)
	jsonMsg(c, "restartApp", err)
}

func (a *ApiService) RestartSb(c *gin.Context) {
	err := a.ConfigService.RestartCore()
	jsonMsg(c, "restartSb", err)
}

// SetMaintenance stops the core and keeps it stopped, or starts it again. The
// five-second watchdog would undo a plain stop, so this is the only way to hold
// the core down from the panel.
func (a *ApiService) SetMaintenance(c *gin.Context) {
	enabled, err := strconv.ParseBool(c.Request.FormValue("enable"))
	if err != nil {
		jsonMsg(c, "maintenance", common.NewError("missing or invalid parameter: enable"))
		return
	}
	err = a.ConfigService.SetMaintenance(enabled)
	jsonMsg(c, "maintenance", err)
}

func (a *ApiService) ResetTraffic(c *gin.Context) {
	if err := a.ClientService.ResetAllClientsTraffic(); err != nil {
		jsonMsg(c, "resetTraffic", err)
		return
	}
	err := a.ConfigService.RestartCore()
	// Clients the reset re-enabled come back on the nodes too.
	a.NodeSyncService.MarkAllDirty()
	go a.NodeSyncService.ReconcileDirtyOnline()
	jsonMsg(c, "resetTraffic", err)
}

func (a *ApiService) LinkConvert(c *gin.Context) {
	link := c.Request.FormValue("link")
	result, _, err := util.GetOutbound(link, 0)
	jsonObj(c, result, err)
}

func (a *ApiService) SubConvert(c *gin.Context) {
	link := c.Request.FormValue("link")
	result, err := util.GetExternalSub(link)
	jsonObj(c, result, err)
}

func (a *ApiService) ImportDb(c *gin.Context) {
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	defer file.Close()
	err = database.ImportDB(file)
	jsonMsg(c, "", err)
}

func (a *ApiService) Logout(c *gin.Context) {
	loginUser := GetLoginUser(c)
	if loginUser != "" {
		logger.Infof("user %s logout", loginUser)
	}
	ClearSession(c)
	jsonMsg(c, "", nil)
}

func (a *ApiService) LoadTokens() ([]byte, error) {
	return a.UserService.LoadTokens()
}

func (a *ApiService) GetTokens(c *gin.Context) {
	loginUser := GetLoginUser(c)
	tokens, err := a.UserService.GetUserTokens(loginUser)
	jsonObj(c, tokens, err)
}

func (a *ApiService) AddToken(c *gin.Context) {
	loginUser := GetLoginUser(c)
	expiry := c.Request.FormValue("expiry")
	expiryInt, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	desc := c.Request.FormValue("desc")
	token, err := a.UserService.AddToken(loginUser, expiryInt, desc)
	jsonObj(c, token, err)
}

func (a *ApiService) DeleteToken(c *gin.Context) {
	loginUser := GetLoginUser(c)
	tokenId := c.Request.FormValue("id")
	err := a.UserService.DeleteToken(loginUser, tokenId)
	jsonMsg(c, "", err)
}

func (a *ApiService) GetSingboxConfig(c *gin.Context) {
	rawConfig, err := a.ConfigService.GetConfig("")
	if err != nil {
		c.Status(400)
		c.Writer.WriteString(err.Error())
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=config_"+time.Now().Format("20060102-150405")+".json")
	c.Writer.Write(*rawConfig)
}

func (a *ApiService) GetCheckOutbound(c *gin.Context) {
	tag := c.Query("tag")
	link := c.Query("link")
	result := a.ConfigService.CheckOutbound(tag, link)
	jsonObj(c, result, nil)
}

func (a *ApiService) GetCertPing(c *gin.Context) {
	domain := c.PostForm("domain")
	port := c.PostForm("port")
	tlsPing, err := util.GetTlsPing(domain, port)
	jsonObj(c, tlsPing, err)
}

func (a *ApiService) TestNode(c *gin.Context) {
	status, err := a.NodeService.TestNode(json.RawMessage(c.PostForm("data")))
	jsonObj(c, status, err)
}

func (a *ApiService) GetNodeInbounds(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil {
		jsonMsg(c, "nodeInbounds", common.NewError("invalid node id"))
		return
	}
	inbounds, err := a.NodeSyncService.FetchNodeInbounds(uint(id))
	jsonObj(c, inbounds, err)
}

func (a *ApiService) AdoptInbounds(c *gin.Context, actor string) {
	id, err := strconv.ParseUint(c.PostForm("id"), 10, 64)
	if err != nil {
		jsonMsg(c, "adoptInbounds", common.NewError("invalid node id"))
		return
	}
	var tags []string
	if err := json.Unmarshal([]byte(c.PostForm("tags")), &tags); err != nil {
		jsonMsg(c, "adoptInbounds", common.NewError("invalid tags"))
		return
	}
	err = a.NodeSyncService.AdoptInbounds(uint(id), tags, actor)
	if err == nil {
		go func() {
			if syncErr := a.NodeSyncService.ReconcileNow(uint(id)); syncErr != nil {
				logger.Warning("nodes: post-adopt reconcile failed: ", syncErr)
			}
		}()
	}
	jsonMsg(c, "adoptInbounds", err)
}

func (a *ApiService) ReconcileNode(c *gin.Context) {
	id, err := strconv.ParseUint(c.PostForm("id"), 10, 64)
	if err != nil {
		jsonMsg(c, "reconcileNode", common.NewError("invalid node id"))
		return
	}
	err = a.NodeSyncService.ReconcileNow(uint(id))
	jsonMsg(c, "reconcileNode", err)
}

// GetTotp tells whether the logged-in account has two-factor login on, and
// when not, the secret and link to set it up with.
func (a *ApiService) GetTotp(c *gin.Context) {
	enabled, secret, uri, err := a.UserService.TotpState(GetLoginUser(c))
	jsonObj(c, map[string]interface{}{"enabled": enabled, "secret": secret, "uri": uri}, err)
}

// SaveTotp turns two-factor login on or off for the logged-in account.
func (a *ApiService) SaveTotp(c *gin.Context) {
	user, code := GetLoginUser(c), c.Request.FormValue("code")
	var err error
	switch c.Request.FormValue("action") {
	case "enable":
		err = a.UserService.EnableTotp(user, code)
	case "disable":
		err = a.UserService.DisableTotp(user, code)
	default:
		err = common.NewError("unknown action")
	}
	if err == nil {
		logger.Info("two-factor login ", c.Request.FormValue("action"), "d for ", user)
	}
	jsonMsg(c, "save", err)
}

// SendRemoteBackup uploads the database to the backup storage now.
func (a *ApiService) SendRemoteBackup(c *gin.Context) {
	jsonMsg(c, "save", service.SendRemoteBackup(c.Request.Context()))
}

// GetClusterSessions answers the panel: a client's connections on the master
// and on the managed nodes.
func (a *ApiService) GetClusterSessions(c *gin.Context) {
	resource := c.Query("resource")
	if resource == "" {
		resource = "user"
	}
	sessions, err := a.StatsService.GetClusterSessions(resource, c.Query("tag"))
	jsonObj(c, sessions, err)
}

// CloseClusterSessions disconnects a client on the master and the nodes.
func (a *ApiService) CloseClusterSessions(c *gin.Context) {
	user := c.PostForm("u")
	if user == "" {
		user = c.Query("u")
	}
	jsonMsg(c, "closeSessions", a.StatsService.CloseClusterSessions(user))
}
