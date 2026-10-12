package api

import (
	"strconv"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"github.com/gin-gonic/gin"
)

// GetSubUrl is the subscription link of one client, with the client's own
// label when the subscription domain is a wildcard.
func (a *ApiService) GetSubUrl(c *gin.Context) {
	id, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || id == 0 {
		jsonMsg(c, "", common.NewError("bad client id"))
		return
	}
	var client model.Client
	if err := database.GetDB().Select("id", "name").Where("id = ?", id).First(&client).Error; err != nil {
		jsonMsg(c, "", err)
		return
	}
	link, err := a.SettingService.ClientSubURL(client.Name, getHostname(c))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"url": link}, nil)
}
