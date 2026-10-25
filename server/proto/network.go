package proto

type WakeOnLANReq struct {
	Mac string `form:"mac" validate:"required"`
}

type GetMacRsp struct {
	Macs []string `json:"macs"`
}

type DeleteMacReq struct {
	Mac string `form:"mac" validate:"required"`
}

type SetMacNameReq struct {
	Mac  string `form:"mac" validate:"required"`
	Name string `form:"name" validate:"required"`
}

type GetWifiRsp struct {
	Supported bool   `json:"supported"`
	ApMode    bool   `json:"apMode"`
	Connected bool   `json:"connected"`
	Ssid      string `json:"ssid"`
}

type ConnectWifiReq struct {
	Ssid     string `validate:"required"`
	Password string `validate:"required"`
}

type GetDNSRsp struct {
	Mode      string   `json:"mode"`
	Servers   []string `json:"servers"`
	Effective []string `json:"effective"`
	DHCP      []string `json:"dhcp"`
	Info      DNSInfo  `json:"info"`
}

type SetDNSReq struct {
	Mode    string   `json:"mode" validate:"required,oneof=manual dhcp"`
	Servers []string `json:"servers"`
}

type DNSInfo struct {
	Interface     string   `json:"interface"`
	Type          string   `json:"type"`
	Address       string   `json:"address"`
	SubnetMask    string   `json:"subnetMask"`
	Gateway       string   `json:"gateway"`
	SearchDomains []string `json:"searchDomains"`
}

type SetInterfaceReq struct {
	Mode    string `json:"mode"`
	Address string `json:"address"`
	Gateway string `json:"gateway"`
}

type InterfaceCurrent struct {
	Addresses []string `json:"addresses"`
	Gateway   string   `json:"gateway"`
	Mac       string   `json:"mac"`
}

type InterfacePending struct {
	DeadlineUnix int64  `json:"deadline_unix"`
	NowUnix      int64  `json:"now_unix"` // the device clock when this was sent, so the page need not trust its own
	Mode         string `json:"mode"`
	Address      string `json:"address"`
	Gateway      string `json:"gateway"`
}

type InterfaceInfo struct {
	Name    string            `json:"name"`
	Kind    string            `json:"kind"`
	Present bool              `json:"present"`
	Mode    string            `json:"mode"`
	Address string            `json:"address"`
	Gateway string            `json:"gateway"`
	Current InterfaceCurrent  `json:"current"`
	Ssid    string            `json:"ssid,omitempty"`
	Pending *InterfacePending `json:"pending"`
}

type GetInterfacesRsp struct {
	Interfaces []InterfaceInfo `json:"interfaces"`
}
