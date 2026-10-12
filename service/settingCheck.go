package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"

	"gorm.io/gorm"
)

// checkNewSetting validates the subscription domain list and the decoy site.
// all is the whole form being saved, for the checks that depend on another setting.
func checkNewSetting(tx *gorm.DB, key, value string, all map[string]string) error {
	switch key {
	case "subDomain":
		if bad, err := hosts.CheckList(value); err != nil {
			return common.NewError("subscription domain <", bad, ">: ", err)
		}
	case "webDecoyDir":
		if value == "" {
			return nil
		}
		if !filepath.IsAbs(value) {
			return common.NewError("the decoy site folder must be an absolute path")
		}
		clean := filepath.Clean(value)
		if clean == "/" || clean == filepath.VolumeName(clean)+string(filepath.Separator) {
			return common.NewError("the decoy site folder cannot be the root of the disk")
		}
		if st, err := os.Stat(clean); err != nil || !st.IsDir() {
			return common.NewError("the decoy site folder <", value, "> does not exist")
		}
		webPath, ok := all["webPath"]
		if !ok {
			var row model.Setting
			if tx.Where("key = ?", "webPath").Limit(1).Find(&row).Error == nil && row.Key != "" {
				webPath = row.Value
			} else {
				webPath = defaultValueMap["webPath"]
			}
		}
		if p := strings.Trim(strings.TrimSpace(webPath), "/"); p == "" {
			return common.NewError("a decoy site needs a panel path other than /: the panel itself answers every path")
		}
	}
	return nil
}
