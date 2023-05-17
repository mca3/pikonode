// Package resolvconf implements a platform to which DNS may be configured.
package resolvconf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mca3/pikonode/net/ifctl"
)

type Config struct {
	Nameservers   []string
	SearchDomains []string
}

// Adds a nameserver to the *top* of the slice.
func (c *Config) AddNameserver(ns string) {
	// TODO: Ensure they're valid
	c.Nameservers = append([]string{ns}, c.Nameservers...)
}

// Adds a search domain to the *top* of the slice.
func (c *Config) AddSearchDomain(ns string) {
	// TODO: Ensure they're valid
	c.SearchDomains = append([]string{ns}, c.SearchDomains...)
}

// SetDNS attempts to set the DNS configuration.
func SetDNS(ifc ifctl.Interface, c Config) error {
	b := strings.Builder{}

	// Build resolv.conf
	b.WriteString("# Generated by Pikonet. DO NOT MODIFY.\n")
	for _, v := range c.SearchDomains {
		b.WriteString(fmt.Sprintf("search %s\n", v))
	}
	for _, v := range c.Nameservers {
		b.WriteString(fmt.Sprintf("nameserver %s\n", v))
	}

	// Ask resolvconf to set our configuration file.
	cmd := exec.Command("resolvconf", "-m", "0", "-x", "-a", ifc.Name())

	var pipe io.WriteCloser
	var err error
	if pipe, err = cmd.StdinPipe(); err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := pipe.Write([]byte(b.String())); err != nil {
		pipe.Close()
		cmd.Wait()
		return err
	}

	pipe.Close()
	return cmd.Wait()
}

// UnsetDNS unsets the DNS configuration set for an interface.
func UnsetDNS(ifc ifctl.Interface) error {
	return exec.Command("resolvconf", "-d", ifc.Name()).Run()
}

func parse(f io.Reader) (c Config, err error) {
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "nameserver ") {
			c.Nameservers = append(c.Nameservers, strings.TrimSpace(strings.TrimPrefix(line, "nameserver ")))
		} else if strings.HasPrefix(line, "search ") {
			c.SearchDomains = append(c.SearchDomains, strings.TrimSpace(strings.TrimPrefix(line, "search ")))
		}
	}

	if s.Err() != nil {
		return c, s.Err()
	}
	return
}

// FetchCurrentConfig returns the current resolv.conf configration.
func FetchCurrentConfig() (c Config, err error) {
	var f *os.File
	f, err = os.Open("/etc/resolv.conf")
	if err != nil {
		return
	}
	defer f.Close()

	c, err = parse(f)
	return
}
