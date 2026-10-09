package database

import (
	"github.com/Danialrostamani/drnetwork-panel/database/model"

	"gorm.io/gorm"
)

// table ties a model to the code that copies its rows. The list used to be
// written out twice, in InitDB and in GetDb, and drifted: Service and Tokens
// were migrated but never backed up.
type table struct {
	// name is what the backup endpoint's exclude parameter matches on.
	name  string
	model any
	// copyRows moves every row of this table from src to dst.
	copyRows func(src, dst *gorm.DB) error
}

// schema is the whole database: adding a model here migrates it and backs it up.
func schema() []table {
	return []table{
		{"settings", &model.Setting{}, copyRows[model.Setting]},
		{"tls", &model.Tls{}, copyRows[model.Tls]},
		{"inbounds", &model.Inbound{}, copyRows[model.Inbound]},
		{"outbounds", &model.Outbound{}, copyRows[model.Outbound]},
		{"services", &model.Service{}, copyRows[model.Service]},
		{"endpoints", &model.Endpoint{}, copyRows[model.Endpoint]},
		{"users", &model.User{}, copyRows[model.User]},
		{"tokens", &model.Tokens{}, copyRows[model.Tokens]},
		{"nodes", &model.Node{}, copyRows[model.Node]},
		{"stats", &model.Stats{}, copyRows[model.Stats]},
		{"clients", &model.Client{}, copyRows[model.Client]},
		{"changes", &model.Changes{}, copyRows[model.Changes]},
		{"bot_quotas", &model.BotQuota{}, copyRows[model.BotQuota]},
		{"bot_quota_clients", &model.BotQuotaClient{}, copyRows[model.BotQuotaClient]},
		{"node_metrics", &model.NodeMetric{}, copyRows[model.NodeMetric]},
		{"node_outages", &model.NodeOutage{}, copyRows[model.NodeOutage]},
		{"node_traffic", &model.NodeTraffic{}, copyRows[model.NodeTraffic]},
		{"shop_plans", &model.ShopPlan{}, copyRows[model.ShopPlan]},
		{"shop_orders", &model.ShopOrder{}, copyRows[model.ShopOrder]},
		{"shop_wallets", &model.ShopWallet{}, copyRows[model.ShopWallet]},
		{"shop_wallet_txs", &model.ShopWalletTx{}, copyRows[model.ShopWalletTx]},
		{"shop_discounts", &model.ShopDiscount{}, copyRows[model.ShopDiscount]},
		{"shop_users", &model.ShopUser{}, copyRows[model.ShopUser]},
		{"shop_code_uses", &model.ShopCodeUse{}, copyRows[model.ShopCodeUse]},
		{"shop_sms", &model.ShopSms{}, copyRows[model.ShopSms]},
		{"shop_auto_renews", &model.ShopAutoRenew{}, copyRows[model.ShopAutoRenew]},
	}
}

// excludedWith names the tables a backup leaves out along with another: the
// per-minute node history is traffic history just like stats.
var excludedWith = map[string][]string{
	"stats": {"node_metrics"},
}

// schemaModels returns the models in schema order, for AutoMigrate.
func schemaModels() []any {
	tables := schema()
	models := make([]any, 0, len(tables))
	for _, t := range tables {
		models = append(models, t.model)
	}
	return models
}

// copyRowsBatch is how many rows one statement writes. A single INSERT of a
// big table runs into SQLite's limit on bound variables.
const copyRowsBatch = 200

func copyRows[T any](src, dst *gorm.DB) error {
	var rows []T
	if err := src.Model(new(T)).Scan(&rows).Error; err != nil {
		return err
	}
	for start := 0; start < len(rows); start += copyRowsBatch {
		end := min(start+copyRowsBatch, len(rows))
		if err := dst.Save(rows[start:end]).Error; err != nil {
			return err
		}
	}
	return nil
}
