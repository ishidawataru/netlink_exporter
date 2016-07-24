// taken mostly from github.com/vishvananda/netlink/link_linux.go
package main

import (
	"net"
	"syscall"
	"unsafe"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

var native = nl.NativeEndian()

const (
	FlagRunning net.Flags = 1 << 5
)

// copied from pkg/net_linux.go
func linkFlags(rawFlags uint32) net.Flags {
	var f net.Flags
	if rawFlags&syscall.IFF_UP != 0 {
		f |= net.FlagUp
	}
	if rawFlags&syscall.IFF_BROADCAST != 0 {
		f |= net.FlagBroadcast
	}
	if rawFlags&syscall.IFF_LOOPBACK != 0 {
		f |= net.FlagLoopback
	}
	if rawFlags&syscall.IFF_POINTOPOINT != 0 {
		f |= net.FlagPointToPoint
	}
	if rawFlags&syscall.IFF_MULTICAST != 0 {
		f |= net.FlagMulticast
	}
	// hack
	if rawFlags&syscall.IFF_RUNNING != 0 {
		f |= FlagRunning
	}
	return f
}

// LinkDeserialize deserializes a raw message received from netlink into
// a link object.
func LinkDeserialize(m []byte) (netlink.Link, error) {
	msg := nl.DeserializeIfInfomsg(m)

	attrs, err := nl.ParseRouteAttr(m[msg.Len():])
	if err != nil {
		return nil, err
	}

	base := netlink.LinkAttrs{Index: int(msg.Index), Flags: linkFlags(msg.Flags)}
	var link netlink.Link
	linkType := ""
	for _, attr := range attrs {
		switch attr.Attr.Type {
		case syscall.IFLA_LINKINFO:
			infos, err := nl.ParseRouteAttr(attr.Value)
			if err != nil {
				return nil, err
			}
			for _, info := range infos {
				switch info.Attr.Type {
				case nl.IFLA_INFO_KIND:
					linkType = string(info.Value[:len(info.Value)-1])
					switch linkType {
					case "dummy":
						link = &netlink.Dummy{}
					case "ifb":
						link = &netlink.Ifb{}
					case "bridge":
						link = &netlink.Bridge{}
					case "vlan":
						link = &netlink.Vlan{}
					case "veth":
						link = &netlink.Veth{}
					case "vxlan":
						link = &netlink.Vxlan{}
					case "bond":
						link = &netlink.Bond{}
					case "ipvlan":
						link = &netlink.IPVlan{}
					case "macvlan":
						link = &netlink.Macvlan{}
					case "macvtap":
						link = &netlink.Macvtap{}
					case "gretap":
						link = &netlink.Gretap{}
					default:
						link = &netlink.GenericLink{LinkType: linkType}
					}
				case nl.IFLA_INFO_DATA:
					_, err := nl.ParseRouteAttr(info.Value)
					if err != nil {
						return nil, err
					}
					// TODO ...
					//					switch linkType {
					//					case "vlan":
					//						parseVlanData(link, data)
					//					case "vxlan":
					//						parseVxlanData(link, data)
					//					case "bond":
					//						parseBondData(link, data)
					//					case "ipvlan":
					//						parseIPVlanData(link, data)
					//					case "macvlan":
					//						parseMacvlanData(link, data)
					//					case "macvtap":
					//						parseMacvtapData(link, data)
					//					case "gretap":
					//						parseGretapData(link, data)
					//					}
				}
			}
		case syscall.IFLA_ADDRESS:
			var nonzero bool
			for _, b := range attr.Value {
				if b != 0 {
					nonzero = true
				}
			}
			if nonzero {
				base.HardwareAddr = attr.Value[:]
			}
		case syscall.IFLA_IFNAME:
			base.Name = string(attr.Value[:len(attr.Value)-1])
		case syscall.IFLA_MTU:
			base.MTU = int(native.Uint32(attr.Value[0:4]))
		case syscall.IFLA_LINK:
			base.ParentIndex = int(native.Uint32(attr.Value[0:4]))
		case syscall.IFLA_MASTER:
			base.MasterIndex = int(native.Uint32(attr.Value[0:4]))
		case syscall.IFLA_TXQLEN:
			base.TxQLen = int(native.Uint32(attr.Value[0:4]))
		case syscall.IFLA_IFALIAS:
			base.Alias = string(attr.Value[:len(attr.Value)-1])
		case syscall.IFLA_STATS:
			base.Statistics = parseLinkStats(attr.Value[:])
		}
	}
	// Links that don't have IFLA_INFO_KIND are hardware devices
	if link == nil {
		link = &netlink.Device{}
	}
	*link.Attrs() = base

	return link, nil
}

func parseLinkStats(data []byte) *netlink.LinkStatistics {
	return (*netlink.LinkStatistics)(unsafe.Pointer(&data[0:netlink.SizeofLinkStats][0]))
}
