package api

import (
	"strconv"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/sub"
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

// GetSubNamePreview renders a link name template for the links of a client:
// the one asked for, or the first one.
func (a *ApiService) GetSubNamePreview(c *gin.Context) {
	tpl := c.Query("tpl")
	q := database.GetDB().Model(model.Client{})
	if id := c.Query("id"); id != "" {
		q = q.Where("id = ?", id)
	}
	var clients []model.Client
	if err := q.Order("id").Limit(1).Find(&clients).Error; err != nil {
		jsonMsg(c, "", err)
		return
	}
	if len(clients) == 0 {
		jsonMsg(c, "", common.NewError("there is no client to preview the names with"))
		return
	}
	names, err := sub.PreviewNames(&clients[0], tpl)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"client": clients[0].Name, "names": names}, nil)
}
