package sub

import "fmt"

// Stock sing-box has latency urltest, not a weighted/priority failover outbound.
// Keep automatic tests INSIDE the assigned node; testing every node together
// would undo the capacity split. A separate distributed backup is explicit.
// Panel-detected failure changes the assigned node on the next profile refresh.
func addSmartOutbounds(outbounds *[]map[string]interface{}, tags *[]string, nodeTags map[string]string, primary string, eligible map[string]bool) error {
	members := []string{}
	backup := []string{}
	backupNode := ""
	for _, tag := range *tags {
		node := nodeTags[tag]
		if node == primary {
			members = append(members, tag)
		} else if node != "" && eligible[node] {
			if backupNode == "" {
				backupNode = node
			}
			if node == backupNode {
				backup = append(backup, tag)
			}
		}
	}
	if len(members) == 0 {
		return fmt.Errorf("assigned node has no compatible sing-box outbound")
	}
	taken := map[string]bool{}
	for _, o := range *outbounds {
		if t, ok := o["tag"].(string); ok {
			taken[t] = true
		}
	}
	backupTag := uniqueOutboundTag("backup", taken)
	for _, o := range *outbounds {
		switch o["tag"] {
		case "auto":
			o["outbounds"] = members
			o["interval"] = "1m"
			o["interrupt_exist_connections"] = false
		case "proxy":
			options := []string{"auto"}
			if len(backup) > 0 {
				options = append(options, backupTag)
			}
			options = append(options, "direct")
			options = append(options, (*tags)...)
			o["outbounds"] = options
			o["default"] = "auto"
			o["interrupt_exist_connections"] = false
		}
	}
	if len(backup) > 0 {
		*outbounds = append(*outbounds, map[string]interface{}{
			"type": "urltest", "tag": backupTag, "outbounds": backup,
			"url": "http://www.gstatic.com/generate_204", "interval": "1m", "tolerance": 50,
			"interrupt_exist_connections": false,
		})
	}
	return nil
}
