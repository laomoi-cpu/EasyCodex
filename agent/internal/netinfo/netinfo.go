package netinfo

import (
	"net"
	"strconv"
	"strings"
)

type Info struct {
	Listen     string   `json:"listen"`
	Host       string   `json:"host"`
	Port       int      `json:"port"`
	LocalURL   string   `json:"localUrl"`
	LANEnabled bool     `json:"lanEnabled"`
	LANURLs    []string `json:"lanUrls"`
}

func Inspect(listen string) Info {
	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return Info{Listen: listen, Host: listen, LocalURL: "http://" + listen}
	}
	port, _ := strconv.Atoi(portText)
	info := Info{
		Listen:     listen,
		Host:       host,
		Port:       port,
		LocalURL:   "http://" + net.JoinHostPort(localHost(host), portText),
		LANEnabled: isLANEnabledHost(host),
	}
	if info.LANEnabled {
		info.LANURLs = LANURLs(portText)
	}
	return info
}

func LANURLs(port string) []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	urls := []string{}
	for _, iface := range interfaces {
		if !isUsableInterface(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || !isUsableLANIPv4(ip) {
				continue
			}
			urls = append(urls, "http://"+net.JoinHostPort(ip.String(), port))
		}
	}
	return urls
}

func isUsableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	blocked := []string{"virtualbox", "vmware", "vethernet", "hyper-v", "docker", "openvpn", "tap", "tailscale", "zerotier", "bluetooth", "loopback"}
	for _, item := range blocked {
		if strings.Contains(name, item) {
			return false
		}
	}
	return true
}

func isUsableLANIPv4(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
}

func localHost(host string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func isLANEnabledHost(host string) bool {
	return host == "" || host == "0.0.0.0" || host == "::"
}
