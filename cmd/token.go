package cmd

import (
	"fmt"

	"github.com/Danialrostamani/drnetwork-panel/config"
	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/service"
)

// addToken gives the first admin the API token a master will use to manage
// this panel as a node. The panel reads its tokens when it starts, so a
// running panel needs a restart to accept it.
func addToken(token, desc string) error {
	if err := database.InitDB(config.GetDBPath()); err != nil {
		return err
	}
	added, err := (&service.UserService{}).AddTokenValue(token, desc)
	if err != nil {
		return err
	}
	if added {
		fmt.Println("API token added. Restart the panel (systemctl restart s-ui) if it is running.")
	} else {
		fmt.Println("This API token is already there.")
	}
	return nil
}
