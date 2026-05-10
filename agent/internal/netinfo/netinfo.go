package netinfo

import (
	"net"
	"strconv"
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
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	urls := []string{}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}
		urls = append(urls, "http://"+net.JoinHostPort(ip.String(), port))
	}
	return urls
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
