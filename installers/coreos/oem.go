package coreos

import (
	"archive/tar"
	"fmt"
	"net/http"
	"strings"

	"github.com/tinkerbell/boots/conf"
	"github.com/tinkerbell/boots/files/tarball"
	"github.com/tinkerbell/boots/installers"
	"github.com/tinkerbell/boots/job"
)

func ServeOEM() func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		var isARM bool
		if j, err := job.CreateFromRemoteAddr(req.Context(), req.RemoteAddr); err == nil {
			isARM = j.IsARM()
		} else {
			installers.Logger("coreos").With("client", req.RemoteAddr).Info(err, "retrieved job is empty")
		}

		tw := tarball.New(w)
		defer tw.Close()

		args := []string{
			"bonding.max_bonds=0",
			"systemd.setenv=phone_home_url=http://" + conf.PublicIPv4.String() + "/phone-home",
			"coreos.autologin",
		}

		var console string
		if isARM {
			console = "console=ttyAMA0,115200"
		} else {
			args = append(args, "vga=773")
			console = "console=tty0 console=ttyS1,115200n8"
		}

		// grub.cfg
		f := tw.NewFile("grub.cfg", 0o644, tar.TypeReg)
		f.Writef("set linux_append=%q\n", strings.Join(args, " "))
		f.Writef("set linux_console=%q\n", console)
		f.Writef("set oem_id=packet\n")
		f.Close()

		if err := addTarFile(tw, "cloud-config.yml", 0o644, cloudConfig); err != nil {
			installers.Logger("coreos").With("client", req.RemoteAddr).Error(err, "cloud-config")
		}

		if err := addTarFile(tw, "bin/phone-home.sh", 0o755, phoneHome); err != nil {
			installers.Logger("coreos").With("client", req.RemoteAddr).Error(err, "phone-home")
		}
		if err := addTarFile(tw, "phone-home.service", 0o644, phoneHomeService); err != nil {
			installers.Logger("coreos").With("client", req.RemoteAddr).Error(err, "phone-home.service")
		}
	}
}

func addTarFile(tw *tarball.Tarball, name string, mode int64, content string) error {
	f := tw.NewFile(name, mode, tar.TypeReg)
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write string: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

const cloudConfig = `#cloud-config
coreos:
  units:
    - name: oem-cloudinit.service
      command: restart
      runtime: yes
      content: |
        [Unit]
        Description=Cloudinit from Packet metadata

        [Service]
        Type=oneshot
        ExecStart=/usr/bin/coreos-cloudinit --oem=packet
  oem:
    id: packet
    name: Packet
    version-id: 0.0.5
    home-url: https://packet.net
    bug-report-url: https://github.com/coreos/bugs/issues
`

const phoneHome = `#!/bin/bash
set -e
while ! curl -H "Content-Type: application/json" -X POST ${phone_home_url}; do
	echo "$0: phone-home unavailable: $phone_home_url" >&2
	sleep 2
done
`

const phoneHomeService = `
[Unit]
Description=Phone home to packet to confirm installation completion
Wants=sys-devices-virtual-net-bond0.device
After=sys-devices-virtual-net-bond0.device

[Service]
Type=oneshot
ExecStart=/usr/share/oem/bin/phone-home.sh

[Install]
WantedBy=multi-user.target
`
