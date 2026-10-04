package service

import (
	"encoding/json"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/database/model"
	"github.com/Danialrostamani/drnetwork-panel/util/common"

	"gorm.io/gorm"
)

type OutboundService struct{}

func (o *OutboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	outbounds := []*model.Outbound{}
	err := db.Model(model.Outbound{}).Scan(&outbounds).Error
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, outbound := range outbounds {
		outData := map[string]interface{}{
			"id":   outbound.Id,
			"type": outbound.Type,
			"tag":  outbound.Tag,
		}
		if outbound.Options != nil {
			var restFields map[string]json.RawMessage
			if err := json.Unmarshal(outbound.Options, &restFields); err != nil {
				return nil, err
			}
			for k, v := range restFields {
				outData[k] = v
			}
		}
		data = append(data, outData)
	}
	return &data, nil
}

func (o *OutboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var outboundsJson []json.RawMessage
	var outbounds []*model.Outbound
	err := db.Model(model.Outbound{}).Scan(&outbounds).Error
	if err != nil {
		return nil, err
	}
	for _, outbound := range outbounds {
		outboundJson, err := outbound.MarshalJSON()
		if err != nil {
			return nil, err
		}
		outboundsJson = append(outboundsJson, outboundJson)
	}
	return outboundsJson, nil
}

func (s *OutboundService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	_, err := s.save(tx, act, data)
	return err
}

// save is EndpointService.save for an outbound.
func (s *OutboundService) save(tx *gorm.DB, act string, data json.RawMessage) (coreSync, error) {
	var live coreSync
	var err error

	switch act {
	case "new", "edit":
		var outbound model.Outbound
		err = outbound.UnmarshalJSON(data)
		if err != nil {
			return live, err
		}

		if corePtr.IsRunning() {
			configData, err := outbound.MarshalJSON()
			if err != nil {
				return live, err
			}
			if act == "new" {
				live, err = addLive(outboundOps(), configData)
			} else {
				var old model.Outbound
				err = tx.Where("id = ?", outbound.Id).Limit(1).Find(&old).Error
				if err != nil {
					return live, err
				}
				var oldConfig []byte
				if old.Id != 0 {
					// Best effort: it only matters if the edit is rejected.
					oldConfig, _ = old.MarshalJSON()
				}
				live, err = replaceLive(outboundOps(), old.Tag, oldConfig, configData)
			}
			if err != nil {
				return live, err
			}
		}

		err = tx.Save(&outbound).Error
		if err != nil {
			return live, err
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return live, err
		}
		err = tx.Where("tag = ?", tag).Delete(model.Outbound{}).Error
		if err != nil {
			return live, err
		}
		if corePtr.IsRunning() {
			live, err = removeLive(outboundOps(), tag)
			if err != nil {
				return live, err
			}
		}
	default:
		return live, common.NewErrorf("unknown action: %s", act)
	}
	return live, nil
}
