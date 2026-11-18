//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"syscall"
)

// ensureWindowsFirewallPort checks and creates an Inbound TCP firewall rule for the active LearnChain port.
func ensureWindowsFirewallPort(port string) {
	if port == "" {
		return
	}
	ruleName := fmt.Sprintf("LearnChain Port %s", port)
	checkCmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+ruleName)
	checkCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := checkCmd.Run(); err == nil {
		return
	}

	addCmd := exec.Command(
		"netsh", "advfirewall", "firewall", "add", "rule",
		"name="+ruleName,
		"dir=in",
		"action=allow",
		"protocol=TCP",
		"localport="+port,
		"profile=any",
	)
	addCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := addCmd.Run(); err != nil {
		log.Printf("ℹ️ ไม่สามารถเปิด Windows Firewall พอร์ต %s อัตโนมัติได้ (อาจต้องรันแบบ Administrator): %v", port, err)
		return
	}
	log.Printf("🛡️ เปิด Windows Firewall Inbound TCP พอร์ต %s อัตโนมัติเรียบร้อยแล้ว", port)
}
