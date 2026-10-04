package service

import (
	"encoding/json"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
)

type EndpointService struct {
	WarpService
}

func (o *EndpointService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	endpoints := []*model.Endpoint{}
	err := db.Model(model.Endpoint{}).Scan(&endpoints).Error
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, endpoint := range endpoints {
		epData := map[string]interface{}{
			"id":   endpoint.Id,
			"type": endpoint.Type,
			"tag":  endpoint.Tag,
			"ext":  endpoint.Ext,
		}
		if endpoint.Options != nil {
			var restFields map[string]json.RawMessage
			if err := json.Unmarshal(endpoint.Options, &restFields); err != nil {
				return nil, err
			}
			for k, v := range restFields {
				epData[k] = v
			}
		}
		data = append(data, epData)
	}
	return &data, nil
}

func (o *EndpointService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var endpointsJson []json.RawMessage
	var endpoints []*model.Endpoint
	err := db.Model(model.Endpoint{}).Find(&endpoints).Error
	if err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		endpointJson, err := endpoint.MarshalJSON()
		if err != nil {
			return nil, err
		}
		endpointsJson = append(endpointsJson, endpointJson)
	}
	return endpointsJson, nil
}

func (s *EndpointService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	_, err := s.save(tx, act, data)
	return err
}

// save applies one change to the transaction and to the running core, and says
// what the core still needs: see coreSync. The new configuration reaches the
// core first, so one sing-box would reject is refused before it is stored; the
// delete goes to the database first, because it has nothing to validate.
func (s *EndpointService) save(tx *gorm.DB, act string, data json.RawMessage) (coreSync, error) {
	var live coreSync
	var err error

	switch act {
	case "new", "edit":
		var endpoint model.Endpoint
		err = endpoint.UnmarshalJSON(data)
		if err != nil {
			return live, err
		}
		if endpoint.Type == "warp" {
			if act == "new" {
				err = s.WarpService.RegisterWarp(&endpoint)
				if err != nil {
					return live, err
				}
			} else {
				var old_license string
				err = tx.Model(model.Endpoint{}).Select("json_extract(ext, '$.license_key')").Where("id = ?", endpoint.Id).Find(&old_license).Error
				if err != nil {
					return live, err
				}
				err = s.WarpService.SetWarpLicense(old_license, &endpoint)
				if err != nil {
					return live, err
				}
			}
		}

		if corePtr.IsRunning() {
			configData, err := endpoint.MarshalJSON()
			if err != nil {
				return live, err
			}
			if act == "new" {
				live, err = addLive(endpointOps(), configData)
			} else {
				var old model.Endpoint
				err = tx.Where("id = ?", endpoint.Id).Limit(1).Find(&old).Error
				if err != nil {
					return live, err
				}
				var oldConfig []byte
				if old.Id != 0 {
					// Best effort: it only matters if the edit is rejected.
					oldConfig, _ = old.MarshalJSON()
				}
				live, err = replaceLive(endpointOps(), old.Tag, oldConfig, configData)
			}
			if err != nil {
				return live, err
			}
		}

		err = tx.Save(&endpoint).Error
		if err != nil {
			return live, err
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return live, err
		}
		err = tx.Where("tag = ?", tag).Delete(model.Endpoint{}).Error
		if err != nil {
			return live, err
		}
		if corePtr.IsRunning() {
			live, err = removeLive(endpointOps(), tag)
			if err != nil {
				return live, err
			}
		}
	default:
		return live, common.NewErrorf("unknown action: %s", act)
	}
	return live, nil
}
