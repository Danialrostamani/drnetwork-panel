package sub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util"
)

type Link struct {
	Type   string `json:"type"`
	Remark string `json:"remark"`
	Uri    string `json:"uri"`
}

type LinkService struct {
}

// GetLinks lists the links of a subscription. With a link name template (n
// active) the links the panel made take their names from it and clientInfo,
// the usage suffix of the old names, is left off: the template has its own
// variables for that.
func (s *LinkService) GetLinks(linkJson *json.RawMessage, types string, clientInfo string, n *namer, clientRemark string) []string {
	links := []Link{}
	var result []string
	err := json.Unmarshal(*linkJson, &links)
	if err != nil {
		return nil
	}
	for _, link := range links {
		switch link.Type {
		case "external":
			result = append(result, s.named(link, n, clientRemark))
		case "sub":
			subLinks := util.GetExternalLink(link.Uri)
			result = append(result, strings.Split(subLinks, "\n")...)
		case "local":
			if types == "all" {
				if n.active() {
					result = append(result, s.named(link, n, clientRemark))
				} else {
					result = append(result, s.addClientInfo(link.Uri, clientInfo))
				}
			}
		}
	}
	return result
}

// named gives a link the panel made its template name; other links pass.
func (s *LinkService) named(link Link, n *namer, clientRemark string) string {
	if !n.active() {
		return link.Uri
	}
	info, def := n.infoForLink(link, clientRemark)
	if info == nil {
		return link.Uri
	}
	return setLinkName(link.Uri, n.name(info, def, true))
}

func (s *LinkService) GetExternalOutbounds(linkJson *json.RawMessage) ([]map[string]interface{}, []string) {
	outbounds, tags, _ := s.externalOutbounds(linkJson, nil, "")
	return outbounds, tags
}

// externalOutbounds turns the external links of a client into outbounds,
// with what the name template knows of each (nil for links it leaves alone).
func (s *LinkService) externalOutbounds(linkJson *json.RawMessage, n *namer, clientRemark string) ([]map[string]interface{}, []string, []*linkInfo) {
	links := []Link{}
	if linkJson == nil || len(*linkJson) == 0 {
		return nil, nil, nil
	}
	err := json.Unmarshal(*linkJson, &links)
	if err != nil {
		return nil, nil, nil
	}

	var outbounds []map[string]interface{}
	var tags []string
	var infos []*linkInfo

	for _, link := range links {
		switch link.Type {
		case "external":
			outbound, tag, err := util.GetOutbound(link.Uri, 0)
			if err == nil && outbound != nil && len(tag) > 0 {
				outbounds = append(outbounds, *outbound)
				tags = append(tags, tag)
				var info *linkInfo
				if n.active() {
					info, _ = n.infoForLink(link, clientRemark)
				}
				infos = append(infos, info)
			}
		case "sub":
			subOutbounds, err := util.GetExternalSub(link.Uri)
			if err != nil {
				logger.Warning("sub: Error getting external sub:", err)
				continue
			}
			for _, outbound := range subOutbounds {
				if tag, _ := outbound["tag"].(string); len(tag) > 0 {
					outbounds = append(outbounds, outbound)
					tags = append(tags, tag)
					infos = append(infos, nil)
				}
			}
		}
	}

	// Make tags unique; sing-box and clash reject duplicate tags/names.
	seen := make(map[string]int)
	for i, tag := range tags {
		if n := seen[tag]; n > 0 {
			newTag := fmt.Sprintf("%s-%d", tag, n)
			seen[tag] = n + 1
			tags[i] = newTag
			outbounds[i]["tag"] = newTag
		} else {
			seen[tag] = 1
		}
	}

	return outbounds, tags, infos
}

func (s *LinkService) addClientInfo(uri string, clientInfo string) string {
	if len(clientInfo) == 0 {
		return uri
	}
	protocol := strings.Split(uri, "://")
	if len(protocol) < 2 {
		return uri
	}
	switch protocol[0] {
	case "vmess":
		var vmessJson map[string]interface{}
		config, err := util.B64StrToByte(protocol[1])
		if err != nil {
			logger.Warning("sub: Error decoding vmess content:", err)
			return uri
		}
		err = json.Unmarshal(config, &vmessJson)
		if err != nil {
			logger.Warning("sub: Error decoding vmess content:", err)
			return uri
		}
		// A vmess link with no "ps", or one whose "ps" is not a string, used
		// to panic here and take down the whole subscription response.
		ps, _ := vmessJson["ps"].(string)
		vmessJson["ps"] = ps + clientInfo
		result, err := json.MarshalIndent(vmessJson, "", "  ")
		if err != nil {
			logger.Warning("sub: Error decoding vmess + clientInfo content:", err)
			return uri
		}
		return "vmess://" + util.ByteToB64Str(result)
	default:
		return uri + clientInfo
	}
}
