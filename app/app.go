package app

import (
	"log"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/core"
	"github.com/Danialrostamani/drnetwork-panel/cronjob"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/service"
	"github.com/Danialrostamani/drnetwork-panel/service/tgbot"
	"github.com/Danialrostamani/drnetwork-panel/sub"
	"github.com/Danialrostamani/drnetwork-panel/web"

	"github.com/op/go-logging"
)

type APP struct {
	service.SettingService
	configService *service.ConfigService
	webServer     *web.Server
	subServer     *sub.Server
	cronJob       *cronjob.CronJob
	logger        *logging.Logger
	core          *core.Core
}

func NewApp() *APP {
	return &APP{}
}

func (a *APP) Init() error {
	log.Printf("%v %v", config.GetName(), config.GetVersion())

	a.initLog()

	err := database.InitDB(config.GetDBPath())
	if err != nil {
		return err
	}

	// Init Setting
	a.SettingService.GetAllSetting()

	a.core = core.NewCore()

	a.cronJob = cronjob.NewCronJob()
	a.webServer = web.NewServer()
	a.subServer = sub.NewServer()

	a.configService = service.NewConfigService(a.core)

	return nil
}

func (a *APP) Start() error {
	loc, err := a.SettingService.GetTimeLocation()
	if err != nil {
		return err
	}

	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		return err
	}

	statsBucketSeconds, err := a.SettingService.GetStatsBucketSeconds()
	if err != nil {
		return err
	}

	err = a.cronJob.Start(loc, trafficAge, statsBucketSeconds)
	if err != nil {
		return err
	}

	err = a.webServer.Start()
	if err != nil {
		return err
	}

	err = a.subServer.Start()
	if err != nil {
		return err
	}

	// Idle unless the Telegram bot is enabled in the settings.
	tgbot.Start(a.configService)

	// StartCore does nothing while maintenance is on, so say why rather than
	// leave the operator looking at a panel that came up with no core.
	if maintenance, mErr := a.SettingService.GetMaintenance(); mErr == nil && maintenance {
		logger.Warning("maintenance mode is on: core not started, clients cannot connect")
	} else {
		err = a.configService.StartCore()
		if err != nil {
			logger.Error(err)
		}
	}

	return nil
}

func (a *APP) Stop() {
	tgbot.Stop()
	a.cronJob.Stop()
	err := a.subServer.Stop()
	if err != nil {
		logger.Warning("stop Sub Server err:", err)
	}
	err = a.webServer.Stop()
	if err != nil {
		logger.Warning("stop Web Server err:", err)
	}
	err = a.configService.StopCore()
	if err != nil {
		logger.Warning("stop Core err:", err)
	}
}

func (a *APP) initLog() {
	switch config.GetLogLevel() {
	case config.Debug:
		logger.InitLogger(logging.DEBUG)
	case config.Info:
		logger.InitLogger(logging.INFO)
	case config.Warn:
		logger.InitLogger(logging.WARNING)
	case config.Error:
		logger.InitLogger(logging.ERROR)
	default:
		log.Fatal("unknown log level:", config.GetLogLevel())
	}
}

func (a *APP) RestartApp() {
	a.Stop()
	a.Start()
}

func (a *APP) GetCore() *core.Core {
	return a.core
}
