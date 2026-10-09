package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"

	"gorm.io/gorm"
)

// checkNewSetting validates the settings added with the subscription headers,
// the domain list, the link names and the decoy site. all is the whole form
// being saved, for the checks that depend on another setting.
func checkNewSetting(tx *gorm.DB, key, value string, all map[string]string) error {
	switch key {
	case "subDomain":
		if bad, err := hosts.CheckList(value); err != nil {
			return common.NewError("subscription domain <", bad, ">: ", err)
		}
	case "subAnnounce":
		if len([]rune(value)) > 1000 {
			return common.NewError("the announcement is longer than 1000 characters")
		}
	case "subSupportUrl":
		if value != "" && !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "tg://") {
			return common.NewError("the support link must start with https://, http:// or tg://")
		}
		if len(value) > 500 || strings.ContainsAny(value, "\r\n") {
			return common.NewError("the support link is not a single short link")
		}
	case "subWebPage":
		if _, err := strconv.ParseBool(value); err != nil {
			return common.NewError("subWebPage must be true or false")
		}
	case "subNameTemplate":
		if err := util.CheckNameTemplate(value); err != nil {
			return err
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
