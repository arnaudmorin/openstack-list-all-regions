// ola (openstack list all) lists the servers of the current project in
// every region found in the compute service catalog.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	osclient "github.com/gophercloud/utils/v2/client"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/term"
)

const maxWorkers = 40

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type row struct {
	region, id, name, status string
	ips                      []string
}

func main() {
	ipv6 := flag.Bool("ipv6", false, "also show IPv6 addresses")
	debug := flag.Bool("debug", false, "log HTTP requests and responses to stderr")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	log.SetFlags(0)

	ctx := context.Background()

	// Honours OS_CLOUD / clouds.yaml as well as plain OS_* variables.
	provider, err := clientconfig.AuthenticatedClient(ctx, &clientconfig.ClientOpts{})
	if err != nil {
		log.Fatalf("authentication failed: %v", err)
	}
	if *debug {
		provider.HTTPClient.Transport = &osclient.RoundTripper{
			Rt:     provider.HTTPClient.Transport,
			Logger: &osclient.DefaultLogger{},
		}
	}

	regions, err := computeRegions(provider)
	if err != nil {
		log.Fatalf("cannot read service catalog: %v", err)
	}
	if *debug {
		fmt.Fprintf(os.Stderr, "Regions found in service catalog: %v\n", regions)
	}

	versions := map[int]bool{4: true, 6: *ipv6}

	var (
		mu   sync.Mutex
		rows []row
		wg   sync.WaitGroup
		sem  = make(chan struct{}, maxWorkers)
	)
	for _, region := range regions {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if *debug {
				fmt.Fprintf(os.Stderr, "Working on %s\n", region)
			}
			r, err := listServers(ctx, provider, region, versions)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: %s failed: %v\n", region, err)
				return
			}
			mu.Lock()
			rows = append(rows, r...)
			mu.Unlock()
		}(region)
	}
	wg.Wait()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].region != rows[j].region {
			return rows[i].region < rows[j].region
		}
		return rows[i].name < rows[j].name
	})

	render(rows, term.IsTerminal(int(os.Stdout.Fd())))
}

// computeRegions returns the sorted list of regions exposing a public compute endpoint.
func computeRegions(provider *gophercloud.ProviderClient) ([]string, error) {
	res, ok := provider.GetAuthResult().(tokens.CreateResult)
	if !ok {
		return nil, fmt.Errorf("only Keystone v3 authentication is supported")
	}
	catalog, err := res.ExtractServiceCatalog()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, svc := range catalog.Entries {
		if svc.Type != "compute" {
			continue
		}
		for _, ep := range svc.Endpoints {
			if ep.Interface == "public" {
				set[ep.Region] = true
			}
		}
	}
	regions := make([]string, 0, len(set))
	for r := range set {
		regions = append(regions, r)
	}
	sort.Strings(regions)
	return regions, nil
}

func listServers(ctx context.Context, provider *gophercloud.ProviderClient, region string, versions map[int]bool) ([]row, error) {
	compute, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region:       region,
		Availability: gophercloud.AvailabilityPublic,
	})
	if err != nil {
		return nil, err
	}
	pages, err := servers.List(compute, servers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	all, err := servers.ExtractServers(pages)
	if err != nil {
		return nil, err
	}

	rows := make([]row, 0, len(all))
	for _, s := range all {
		rows = append(rows, row{
			region: region,
			id:     s.ID,
			name:   s.Name,
			status: s.Status,
			ips:    extractIPs(s.Addresses, versions),
		})
	}
	return rows, nil
}

func extractIPs(addresses map[string]any, versions map[int]bool) []string {
	networks := make([]string, 0, len(addresses))
	for n := range addresses {
		networks = append(networks, n)
	}
	sort.Strings(networks)

	var ips []string
	for _, network := range networks {
		list, _ := addresses[network].([]any)
		for _, a := range list {
			addr, _ := a.(map[string]any)
			version, _ := addr["version"].(float64)
			if !versions[int(version)] {
				continue
			}
			ip, _ := addr["addr"].(string)
			ips = append(ips, fmt.Sprintf("%s: %s", network, ip))
		}
	}
	return ips
}

var statusColors = map[string]text.Colors{
	"ACTIVE":  {text.FgGreen},
	"BUILD":   {text.FgCyan},
	"SHUTOFF": {text.FgYellow},
	"PAUSED":  {text.FgYellow},
	"ERROR":   {text.Bold, text.FgRed},
}

func render(rows []row, tty bool) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Region", "UUID", "Name", "IPs", "Status"})

	ipSep := ", "
	if tty {
		ipSep = "\n" // one address per line reads better in a terminal
	}

	counts := map[string]int{}
	for _, r := range rows {
		counts[r.status]++
		status := r.status
		if c, ok := statusColors[status]; ok && tty {
			status = c.Sprint(status)
		}
		t.AppendRow(table.Row{r.region, r.id, r.name, strings.Join(r.ips, ipSep), status})
	}

	t.AppendFooter(table.Row{fmt.Sprintf("%d servers", len(rows)), "", "", "", summary(counts)})

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, WidthMax: 30, WidthMaxEnforcer: text.WrapSoft},
		{Number: 2, WidthMax: 36},
		{Number: 3, WidthMax: 64, WidthMaxEnforcer: text.WrapSoft},
		{Number: 4, WidthMax: 80, WidthMaxEnforcer: text.WrapSoft},
		{Number: 5, WidthMax: 30, Align: text.AlignCenter, AlignFooter: text.AlignRight},
	})

	style := table.StyleRounded
	if tty {
		style.Color.Header = text.Colors{text.Bold, text.FgHiCyan}
		style.Color.Footer = text.Colors{text.FgHiBlack}
		style.Color.Border = text.Colors{text.FgHiBlack}
		style.Color.Separator = text.Colors{text.FgHiBlack}
		style.Color.RowAlternate = text.Colors{text.Bg256Color(236)}
	} else {
		style.Color = table.ColorOptions{}
	}
	style.Format.Footer = text.FormatDefault
	style.Format.FooterAlign = text.AlignLeft
	t.SetStyle(style)

	t.Render()
}

// summary renders e.g. "12 ACTIVE · 1 ERROR".
func summary(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	return strings.Join(parts, " · ")
}
