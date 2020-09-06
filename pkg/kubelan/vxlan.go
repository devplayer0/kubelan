package kubelan

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// ZeroMAC represents an all-zero MAC address
var ZeroMAC = net.HardwareAddr([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

func guessMTU(ip net.IP) (int, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return 0, fmt.Errorf("failed to get link list: %w", err)
	}

	for _, link := range links {
		addrs, err := netlink.AddrList(link, syscall.AF_INET)
		if err != nil {
			return 0, fmt.Errorf("failed to get addresses for interface %v: %w", link.Attrs().Name, err)
		}

		for _, addr := range addrs {
			if addr.IP.Equal(ip) {
				// 50 bytes is a generous amount for the IPv4 + UDP + VXLAN header overhead
				return link.Attrs().MTU - 50, nil
			}
		}
	}

	log.Warnf("Failed to guess MTU from IP %v", ip)
	return 0, nil
}

// VXLAN creates and manages a VXLAN network interface
type VXLAN struct {
	name string

	vxlan *netlink.Vxlan
}

// NewVXLAN creates a new VXLAN interface
func NewVXLAN(name string, mtu int, vni uint32, src net.IP, port uint16) (*VXLAN, error) {
	log.WithField("name", name).Debug("Creating VXLAN interface")

	var err error
	if mtu == -1 {
		mtu, err = guessMTU(src)
		if err != nil {
			return nil, fmt.Errorf("error while trying to guess MTU: %w", err)
		}

		if mtu != 0 {
			log.WithFields(log.Fields{
				"ip":  src,
				"mtu": mtu,
			}).Debug("Guessed MTU from source IP")
		}
	}

	existing, err := netlink.LinkByName(name)
	if err == nil {
		log.WithField("interface", name).Warn("Deleting existing VXLAN interface")
		if err := netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("failed to delete existing interface: %w", err)
		}
	} else if !errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil, fmt.Errorf("failed to get existing interface: %w", err)
	}

	la := netlink.NewLinkAttrs()
	la.Name = name
	la.MTU = mtu
	vxlan := &netlink.Vxlan{
		LinkAttrs: la,
		VxlanId:   int(vni),
		SrcAddr:   src,
		Learning:  true,
		Port:      int(port),
	}
	if err := netlink.LinkAdd(vxlan); err != nil {
		return nil, err
	}
	if err := netlink.LinkSetUp(vxlan); err != nil {
		return nil, fmt.Errorf("failed to set state to up: %w", err)
	}

	return &VXLAN{name, vxlan}, nil
}

// Delete removes the VXLAN interface
func (v *VXLAN) Delete() error {
	return netlink.LinkDel(v.vxlan)
}

// AddPeer adds a peer to the VXLAN interface (`bridge fdb append ...`)
func (v *VXLAN) AddPeer(ip net.IP) error {
	log.WithFields(log.Fields{
		"interface": v.name,
		"ip":        ip,
	}).Trace("Adding peer to VXLAN")

	return netlink.NeighAppend(&netlink.Neigh{
		Family:    syscall.AF_BRIDGE,
		LinkIndex: v.vxlan.Index,

		State:        netlink.NUD_PERMANENT,
		Flags:        netlink.NTF_SELF,
		HardwareAddr: ZeroMAC,
		IP:           ip,
	})
}

// RemovePeer deletes a peer from the VXLAN interface (`bridge fdb delete ...`)
func (v *VXLAN) RemovePeer(ip net.IP) error {
	log.WithFields(log.Fields{
		"interface": v.name,
		"ip":        ip,
	}).Trace("Remove peer from VXLAN")

	return netlink.NeighDel(&netlink.Neigh{
		Family:    syscall.AF_BRIDGE,
		LinkIndex: v.vxlan.Index,

		State:        netlink.NUD_PERMANENT,
		Flags:        netlink.NTF_SELF,
		HardwareAddr: ZeroMAC,
		IP:           ip,
	})
}
