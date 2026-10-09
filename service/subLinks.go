package service

import (
	"net/url"
	"strings"

	"github.com/Danialrostamani/drnetwork-panel/database"
	"github.com/Danialrostamani/drnetwork-panel/logger"
	"github.com/Danialrostamani/drnetwork-panel/util"
	"github.com/Danialrostamani/drnetwork-panel/util/common"
	"github.com/Danialrostamani/drnetwork-panel/util/hosts"
)

// The subscription can be served on several domains at once: the first one in
// subDomain is what new links use, the others keep answering, so a domain can
// be rolled over without breaking the links people already have. A wildcard
// entry ("*.sub.example.com") gives each client a host of its own, with a
// label only the panel can work out.

func init() {
	// Inbound addresses written as "*.cdn.example.com" get the same kind of
	// per-client label; the link generator lives below the settings.
	util.WildcardLabel = func(base, name string) string {
		return hosts.Label((&SettingService{}).GetLabelSecret(), base, name)
	}
}

// GetSubDomains is the subscription domain list, in order.
func (s *SettingService) GetSubDomains() []string {
	v, _ := s.getString("subDomain")
	return hosts.Parse(v)
}

// GetLabelSecret is the key of the per-client wildcard labels. It is made on
// first use and kept: a new one would move every client to a new host.
func (s *SettingService) GetLabelSecret() []byte {
	if database.GetDB() == nil {
		return []byte(defaultValueMap["labelSecret"])
	}
	v, err := s.getString("labelSecret")
	if err != nil || v == "" {
		return []byte(defaultValueMap["labelSecret"])
	}
	if v == defaultValueMap["labelSecret"] {
		if err := s.saveSetting("labelSecret", v); err != nil {
			logger.Warning("save label secret failed:", err)
		}
	}
	return []byte(v)
}

// SubHostEntries lists the domain entries a subscription request may come on:
// the subDomain list and the host of an explicit subURI.
func (s *SettingService) SubHostEntries() []string {
	entries := s.GetSubDomains()
	if uri, _ := s.GetSubURI(); uri != "" {
		if u, err := url.Parse(uri); err == nil && u.Hostname() != "" {
			h := strings.ToLower(u.Hostname())
			if hosts.Check(h) == nil {
				entries = append(entries, h)
			}
		}
	}
	return entries
}

// SubLabelOK tells whether the host of a request may serve subID: any host
// that is not under a wildcard entry may, a wildcard one only with the
// client's own label.
func (s *SettingService) SubLabelOK(host, subID string) bool {
	entries := s.SubHostEntries()
	entry, label, ok := hosts.Match(entries, host)
	if ok {
		return !hosts.IsWildcard(entry) || label == hosts.Label(s.GetLabelSecret(), hosts.Base(entry), subID)
	}
	// A deeper name under a wildcard domain -- DNS answers it too -- is no
	// client's host.
	h := hosts.Strip(host)
	for _, e := range entries {
		if hosts.IsWildcard(e) && strings.HasSuffix(h, "."+hosts.Base(e)) {
			return false
		}
	}
	return true
}

// ClientSubURL is the subscription link of one client: the base link of
// GetFinalSubURI with the client's label in place of a wildcard, then the
// client's name.
func (s *SettingService) ClientSubURL(name, host string) (string, error) {
	base, err := s.GetFinalSubURI(host)
	if err != nil {
		return "", err
	}
	if u, perr := url.Parse(base); perr == nil && hosts.IsWildcard(u.Hostname()) {
		concrete := hosts.Concrete(s.GetLabelSecret(), strings.ToLower(u.Hostname()), name)
		if port := u.Port(); port != "" {
			u.Host = concrete + ":" + port
		} else {
			u.Host = concrete
		}
		base = u.String()
	} else if strings.Contains(base, "://*.") {
		return "", common.NewError("bad wildcard in the subscription address: ", base)
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + url.PathEscape(name), nil
}
