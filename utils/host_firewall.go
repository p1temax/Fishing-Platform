package utils

import (
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"runtime"
	"strings"
)

const firewallRuleCommentPrefix = "fishing-platform-ip-blacklist:"

type firewallTarget struct {
	command string
	chains  []string
	source  string
}

func ApplyHostIPBlock(ipOrCIDR string) error {
	target, err := buildFirewallTarget(ipOrCIDR)
	if err != nil {
		return err
	}

	var applied []string
	for _, chain := range target.chains {
		exists, err := firewallChainExists(target.command, chain)
		if err != nil {
			rollbackHostIPBlock(target, applied)
			return err
		}
		if !exists {
			continue
		}

		present, err := firewallRuleExists(target.command, chain, target.source)
		if err != nil {
			rollbackHostIPBlock(target, applied)
			return err
		}
		if present {
			continue
		}

		if err := runFirewallCommand(target.command, append([]string{"-I", chain, "1"}, firewallRuleArgs(target.source)...)...); err != nil {
			rollbackHostIPBlock(target, applied)
			return err
		}
		applied = append(applied, chain)
	}

	if len(applied) == 0 {
		chainAvailable := false
		for _, chain := range target.chains {
			exists, _ := firewallChainExists(target.command, chain)
			if exists {
				chainAvailable = true
				break
			}
		}
		if !chainAvailable {
			return fmt.Errorf("no supported firewall chain found for %s", target.command)
		}
	}

	return nil
}

func RemoveHostIPBlock(ipOrCIDR string) error {
	target, err := buildFirewallTarget(ipOrCIDR)
	if err != nil {
		return err
	}

	var firstErr error
	for _, chain := range target.chains {
		exists, err := firewallChainExists(target.command, chain)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !exists {
			continue
		}

		for {
			present, err := firewallRuleExists(target.command, chain, target.source)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break
			}
			if !present {
				break
			}
			if err := runFirewallCommand(target.command, append([]string{"-D", chain}, firewallRuleArgs(target.source)...)...); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break
			}
		}
	}

	return firstErr
}

func buildFirewallTarget(ipOrCIDR string) (firewallTarget, error) {
	if runtime.GOOS != "linux" {
		return firewallTarget{}, fmt.Errorf("host firewall blacklist currently supports linux, current OS is %s", runtime.GOOS)
	}

	source, isIPv4, err := normalizeFirewallSource(ipOrCIDR)
	if err != nil {
		return firewallTarget{}, err
	}

	command := "iptables"
	if !isIPv4 {
		command = "ip6tables"
	}
	if _, err := exec.LookPath(command); err != nil {
		return firewallTarget{}, fmt.Errorf("%s not found on host", command)
	}

	return firewallTarget{
		command: command,
		chains:  []string{"INPUT", "DOCKER-USER"},
		source:  source,
	}, nil
}

func normalizeFirewallSource(value string) (string, bool, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", false, errors.New("IP address is required")
	}

	if strings.Contains(candidate, "/") {
		prefix, err := netip.ParsePrefix(candidate)
		if err != nil {
			return "", false, errors.New("Invalid IP or CIDR")
		}
		masked := prefix.Masked()
		return masked.String(), masked.Addr().Is4(), nil
	}

	addr, err := netip.ParseAddr(candidate)
	if err != nil {
		return "", false, errors.New("Invalid IP or CIDR")
	}
	return addr.String(), addr.Is4(), nil
}

func firewallRuleArgs(source string) []string {
	return []string{
		"-s", source,
		"-m", "comment",
		"--comment", firewallRuleCommentPrefix + source,
		"-j", "DROP",
	}
}

func firewallChainExists(command string, chain string) (bool, error) {
	err := runFirewallCommand(command, "-nL", chain)
	if err == nil {
		return true, nil
	}
	errorText := err.Error()
	if strings.Contains(errorText, "No chain") ||
		strings.Contains(errorText, "does not exist") ||
		strings.Contains(errorText, "No such file or directory") {
		return false, nil
	}
	return false, err
}

func firewallRuleExists(command string, chain string, source string) (bool, error) {
	err := runFirewallCommand(command, append([]string{"-C", chain}, firewallRuleArgs(source)...)...)
	if err == nil {
		return true, nil
	}
	errorText := err.Error()
	if strings.Contains(errorText, "Bad rule") ||
		strings.Contains(errorText, "does a matching rule exist") {
		return false, nil
	}
	return false, err
}

func runFirewallCommand(command string, args ...string) error {
	fullArgs := append([]string{"-w"}, args...)
	cmd := exec.Command(command, fullArgs...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s failed: %w: %s", command, strings.Join(fullArgs, " "), err, strings.TrimSpace(string(output)))
}

func rollbackHostIPBlock(target firewallTarget, chains []string) {
	for _, chain := range chains {
		_ = runFirewallCommand(target.command, append([]string{"-D", chain}, firewallRuleArgs(target.source)...)...)
	}
}
