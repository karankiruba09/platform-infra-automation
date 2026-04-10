package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommonConfig struct {
	OvfToolPath        string            `json:"ovfToolPath"`
	OvaPath            string            `json:"ovaPath"`
	Username           string            `json:"username"`
	VCenterPassword    string            `json:"vcenterPassword"`
	VCenterPasswordEnv string            `json:"vcenterPasswordEnv"`
	Datastore          string            `json:"datastore"`
	Cluster            string            `json:"cluster"`
	VMFolder           string            `json:"vmFolder"`
	NetworkMappings    map[string]string `json:"networkMappings"`
	DiskMode           string            `json:"diskMode"`
	DeploymentOption   string            `json:"deploymentOption"`
	DNSList            []string          `json:"dnsList"`
	NTPList            []string          `json:"ntpList"`
	MgrCliPassword     string            `json:"mgrCliPassword"`
	MgrCliPasswordEnv  string            `json:"mgrCliPasswordEnv"`
	MgrRootPassword    string            `json:"mgrRootPassword"`
	MgrRootPasswordEnv string            `json:"mgrRootPasswordEnv"`
	TcaUserPassword    string            `json:"tcaUserPassword"`
	TcaUserPasswordEnv string            `json:"tcaUserPasswordEnv"`
	SSHEnabled         bool              `json:"sshEnabled"`
	ServiceWFH         bool              `json:"serviceWFH"`
	ApplianceRole      string            `json:"applianceRole"`
	IPProtocol         string            `json:"ipProtocol"`
	WaitForIPSeconds   int               `json:"waitForIPSeconds"`
	LogDir             string            `json:"logDir"`
	LogLevel           string            `json:"logLevel"`
	WorkerCount        int               `json:"workerCount"`
	TimeoutMinutes     int               `json:"timeoutMinutes"`
	ExtraArgs          []string          `json:"extraArgs"`
}

type SiteConfig struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Hostname           string            `json:"hostname"`
	ExternalAddress    string            `json:"externalAddress"`
	IP                 string            `json:"ip"`
	Prefix             string            `json:"prefix"`
	Gateway            string            `json:"gateway"`
	VCenterHost        string            `json:"vcenterHost"`
	Datacenter         string            `json:"datacenter"`
	Username           string            `json:"username"`
	VCenterPassword    string            `json:"vcenterPassword"`
	VCenterPasswordEnv string            `json:"vcenterPasswordEnv"`
	Datastore          string            `json:"datastore"`
	Cluster            string            `json:"cluster"`
	VMFolder           string            `json:"vmFolder"`
	NetworkMappings    map[string]string `json:"networkMappings"`
	DiskMode           string            `json:"diskMode"`
	DeploymentOption   string            `json:"deploymentOption"`
	DNSList            []string          `json:"dnsList"`
	NTPList            []string          `json:"ntpList"`
	OvfToolPath        string            `json:"ovfToolPath"`
	OvaPath            string            `json:"ovaPath"`
	MgrCliPassword     string            `json:"mgrCliPassword"`
	MgrCliPasswordEnv  string            `json:"mgrCliPasswordEnv"`
	MgrRootPassword    string            `json:"mgrRootPassword"`
	MgrRootPasswordEnv string            `json:"mgrRootPasswordEnv"`
	TcaUserPassword    string            `json:"tcaUserPassword"`
	TcaUserPasswordEnv string            `json:"tcaUserPasswordEnv"`
	SSHEnabled         *bool             `json:"sshEnabled"`
	ServiceWFH         *bool             `json:"serviceWFH"`
	ApplianceRole      string            `json:"applianceRole"`
	IPProtocol         string            `json:"ipProtocol"`
	WaitForIPSeconds   int               `json:"waitForIPSeconds"`
	TimeoutMinutes     int               `json:"timeoutMinutes"`
	ExtraArgs          []string          `json:"extraArgs"`
	LogDir             string            `json:"logDir"`
	LogLevel           string            `json:"logLevel"`
}

type Config struct {
	Common CommonConfig `json:"common"`
	Sites  []SiteConfig `json:"sites"`
}

type DeployResult struct {
	Site string
	Err  error
}

// --- Progress tracker ---

type siteStatus struct {
	phase    string // Queued, Opening OVA, Validating, Connecting, Deploying, Powering On, Waiting IP, Done, Failed
	progress int    // 0-100 disk progress percentage, -1 if not applicable
	err      string
}

type progressTracker struct {
	mu       sync.Mutex
	names    []string // ordered VM names
	statuses map[string]*siteStatus
	lines    int // number of lines last drawn (for redraw)
}

func newProgressTracker(names []string) *progressTracker {
	statuses := make(map[string]*siteStatus, len(names))
	for _, n := range names {
		statuses[n] = &siteStatus{phase: "Queued", progress: -1}
	}
	return &progressTracker{names: names, statuses: statuses}
}

func (pt *progressTracker) update(vmName, phase string, progress int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if s, ok := pt.statuses[vmName]; ok {
		s.phase = phase
		s.progress = progress
	}
	pt.render()
}

func (pt *progressTracker) fail(vmName, errMsg string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if s, ok := pt.statuses[vmName]; ok {
		s.phase = "Failed"
		s.progress = -1
		s.err = errMsg
	}
	pt.render()
}

func (pt *progressTracker) render() {
	// Move cursor up to overwrite previous table
	if pt.lines > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA", pt.lines)
	}

	// Find max VM name length for column width
	maxName := 7 // minimum "VM NAME"
	for _, n := range pt.names {
		if len(n) > maxName {
			maxName = len(n)
		}
	}
	if maxName > 45 {
		maxName = 45
	}

	var buf strings.Builder
	headerFmt := fmt.Sprintf("  %%-%ds  %%-14s  %%s", maxName)
	rowFmt := fmt.Sprintf("  %%-%ds  %%-14s  %%s", maxName)

	buf.WriteString(fmt.Sprintf(headerFmt, "VM NAME", "STATUS", "PROGRESS"))
	buf.WriteString("\n")
	sep := fmt.Sprintf("  %s  %s  %s", strings.Repeat("─", maxName), strings.Repeat("─", 14), strings.Repeat("─", 30))
	buf.WriteString(sep)
	buf.WriteString("\n")

	lines := 2
	for _, name := range pt.names {
		s := pt.statuses[name]
		displayName := name
		if len(displayName) > maxName {
			displayName = displayName[:maxName-2] + ".."
		}

		var bar string
		if s.progress >= 0 {
			bar = progressBar(s.progress, 20)
		} else if s.phase == "Failed" {
			short := s.err
			if len(short) > 28 {
				short = short[:28] + ".."
			}
			bar = short
		} else if s.phase == "Done" {
			bar = progressBar(100, 20)
		} else {
			bar = "—"
		}

		// Color the status
		status := s.phase
		switch {
		case s.phase == "Failed":
			status = "\033[31m" + s.phase + "\033[0m" // red
		case s.phase == "Done":
			status = "\033[32m" + s.phase + "\033[0m" // green
		case s.phase == "Deploying":
			status = "\033[33m" + s.phase + "\033[0m" // yellow
		case s.phase == "Queued":
			status = "\033[90m" + s.phase + "\033[0m" // gray
		}

		buf.WriteString(fmt.Sprintf(rowFmt, displayName, status, bar))
		buf.WriteString("\033[K\n") // clear rest of line
		lines++
	}

	pt.lines = lines
	fmt.Fprint(os.Stderr, buf.String())
}

func progressBar(pct, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := width * pct / 100
	empty := width - filled
	return fmt.Sprintf("%s%s %3d%%", strings.Repeat("█", filled), strings.Repeat("░", empty), pct)
}

// parseLine reads an ovftool output line and returns (phase, progress)
func parseLine(line string) (string, int) {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "disk progress:"):
		// extract percentage: "Disk progress: 45%"
		if idx := strings.Index(lower, "disk progress:"); idx >= 0 {
			after := strings.TrimSpace(line[idx+14:])
			after = strings.TrimSuffix(after, "%")
			if n, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				return "Deploying", n
			}
		}
		return "Deploying", -1
	case strings.Contains(lower, "opening ova source"):
		return "Opening OVA", -1
	case strings.Contains(lower, "manifest validates"):
		return "Validating", -1
	case strings.Contains(lower, "opening vi target"):
		return "Connecting", -1
	case strings.Contains(lower, "deploying to vi"):
		return "Deploying", 0
	case strings.Contains(lower, "transfer completed"):
		return "Finalizing", -1
	case strings.Contains(lower, "powering on"):
		return "Powering On", -1
	case strings.Contains(lower, "waiting for ip"):
		return "Waiting IP", -1
	case strings.Contains(lower, "completed successfully"):
		return "Done", 100
	case strings.Contains(lower, "error:") || strings.Contains(lower, "completed with errors"):
		return "Failed", -1
	}
	return "", -1
}

// trackWriter wraps a file writer and also parses output to update the progress tracker
type trackWriter struct {
	vmName  string
	tracker *progressTracker
	file    *os.File
	buf     []byte
}

func (tw *trackWriter) Write(p []byte) (int, error) {
	n, err := tw.file.Write(p)

	// Buffer and parse lines
	tw.buf = append(tw.buf, p...)
	for {
		idx := -1
		for i, b := range tw.buf {
			if b == '\n' || b == '\r' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(tw.buf[:idx])
		tw.buf = tw.buf[idx+1:]

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		phase, progress := parseLine(line)
		if phase == "" {
			continue
		}
		if phase == "Failed" {
			tw.tracker.fail(tw.vmName, line)
		} else {
			tw.tracker.update(tw.vmName, phase, progress)
		}
	}

	return n, err
}

func (tw *trackWriter) Close() error {
	return tw.file.Close()
}

// --- Main ---

func main() {
	configPath := flag.String("config", "inputs/deploy.json", "Path to JSON config file")
	sitesFlag := flag.String("sites", "", "Comma separated site IDs to deploy; default all")
	dryRun := flag.Bool("dry-run", false, "Print commands without running ovftool")
	workerOverride := flag.Int("workers", 0, "Override worker count from config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		exitWithErr(err)
	}

	if *workerOverride > 0 {
		cfg.Common.WorkerCount = *workerOverride
	}

	requestedSites := parseList(*sitesFlag)
	sites, err := filterSites(cfg.Sites, requestedSites)
	if err != nil {
		exitWithErr(err)
	}

	if cfg.Common.WorkerCount <= 0 {
		if len(sites) < 4 {
			cfg.Common.WorkerCount = len(sites)
		} else {
			cfg.Common.WorkerCount = 4
		}
	}

	if cfg.Common.WorkerCount <= 0 {
		cfg.Common.WorkerCount = 1
	}

	logDir := cfg.Common.LogDir
	if logDir == "" {
		logDir = "output/logs"
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		exitWithErr(fmt.Errorf("failed to create log directory %s: %w", logDir, err))
	}

	fmt.Fprintf(os.Stderr, "Deploying %d site(s) with %d worker(s)\n\n", len(sites), cfg.Common.WorkerCount)

	// Build ordered VM name list and create tracker
	vmNames := make([]string, len(sites))
	for i, s := range sites {
		merged := mergeSite(cfg.Common, s)
		vmNames[i] = merged.Name
	}

	tracker := newProgressTracker(vmNames)

	if !*dryRun {
		tracker.render()
	}

	jobs := make(chan SiteConfig)
	results := make(chan DeployResult, len(sites))

	var wg sync.WaitGroup
	for i := 0; i < cfg.Common.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range jobs {
				err := deploySite(cfg.Common, site, logDir, *dryRun, tracker)
				results <- DeployResult{Site: site.ID + " (" + site.Name + ")", Err: err}
			}
		}()
	}

	for _, s := range sites {
		jobs <- s
	}
	close(jobs)

	wg.Wait()
	close(results)

	// Print final summary below the table
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Deployment summary")
	fmt.Fprintln(os.Stderr, strings.Repeat("─", 60))
	failed := false
	for r := range results {
		if r.Err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "  \033[31mFAILED\033[0m   %s\n", r.Site)
			fmt.Fprintf(os.Stderr, "           %v\n", r.Err)
		} else {
			fmt.Fprintf(os.Stderr, "  \033[32mSUCCESS\033[0m  %s\n", r.Site)
		}
	}

	if failed {
		os.Exit(1)
	}
}

func loadConfig(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validateConfig(cfg Config) error {
	if cfg.Common.OvfToolPath == "" {
		return errors.New("common.ovfToolPath is required")
	}
	if cfg.Common.OvaPath == "" {
		return errors.New("common.ovaPath is required")
	}
	if cfg.Common.Username == "" {
		return errors.New("common.username is required")
	}
	if cfg.Common.Datastore == "" {
		return errors.New("common.datastore is required")
	}
	if cfg.Common.Cluster == "" {
		return errors.New("common.cluster is required")
	}
	if cfg.Common.VMFolder == "" {
		return errors.New("common.vmFolder is required")
	}
	if len(cfg.Common.NetworkMappings) == 0 {
		return errors.New("common.networkMappings must include at least one entry (e.g. VSMgmt)")
	}
	if cfg.Common.DiskMode == "" {
		return errors.New("common.diskMode is required")
	}
	if cfg.Common.DeploymentOption == "" {
		return errors.New("common.deploymentOption is required")
	}
	if len(cfg.Common.DNSList) == 0 {
		return errors.New("common.dnsList is required")
	}
	if len(cfg.Common.NTPList) == 0 {
		return errors.New("common.ntpList is required")
	}
	if len(cfg.Sites) == 0 {
		return errors.New("at least one site entry is required")
	}

	for _, s := range cfg.Sites {
		if s.ID == "" {
			return errors.New("site.id is required")
		}
		if s.Name == "" {
			return fmt.Errorf("site %s missing name", s.ID)
		}
		if s.Hostname == "" {
			return fmt.Errorf("site %s missing hostname", s.ID)
		}
		if s.ExternalAddress == "" {
			return fmt.Errorf("site %s missing externalAddress", s.ID)
		}
		if s.IP == "" {
			return fmt.Errorf("site %s missing ip", s.ID)
		}
		if s.Prefix == "" {
			return fmt.Errorf("site %s missing prefix", s.ID)
		}
		if s.Gateway == "" {
			return fmt.Errorf("site %s missing gateway", s.ID)
		}
		if s.VCenterHost == "" {
			return fmt.Errorf("site %s missing vcenterHost", s.ID)
		}
		if s.Datacenter == "" {
			return fmt.Errorf("site %s missing datacenter", s.ID)
		}
	}
	return nil
}

func filterSites(all []SiteConfig, requested []string) ([]SiteConfig, error) {
	if len(requested) == 0 {
		return all, nil
	}

	index := make(map[string]SiteConfig, len(all))
	for _, s := range all {
		index[s.ID] = s
	}

	selected := make([]SiteConfig, 0, len(requested))
	for _, id := range requested {
		site, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("requested site not found: %s", id)
		}
		selected = append(selected, site)
	}
	return selected, nil
}

func deploySite(common CommonConfig, site SiteConfig, baseLogDir string, dryRun bool, tracker *progressTracker) error {
	merged := mergeSite(common, site)

	tracker.update(merged.Name, "Starting", -1)

	ovftool := merged.OvfToolPath
	if ovftool == "" {
		tracker.fail(merged.Name, "ovfToolPath not resolved")
		return fmt.Errorf("site %s: ovfToolPath not resolved", site.ID)
	}
	if _, err := os.Stat(ovftool); err != nil {
		tracker.fail(merged.Name, "ovftool not found")
		return fmt.Errorf("site %s: ovftool not found at %s", site.ID, ovftool)
	}

	ovaPath := merged.OvaPath
	if ovaPath == "" {
		tracker.fail(merged.Name, "ovaPath not resolved")
		return fmt.Errorf("site %s: ovaPath not resolved", site.ID)
	}
	if !filepath.IsAbs(ovaPath) {
		if abs, err := filepath.Abs(ovaPath); err == nil {
			ovaPath = abs
		}
	}
	if _, err := os.Stat(ovaPath); err != nil {
		tracker.fail(merged.Name, "OVA not found")
		return fmt.Errorf("site %s: OVA not found at %s", site.ID, ovaPath)
	}

	vcPassword, err := secret("vCenter", merged.VCenterPasswordEnv, merged.VCenterPassword)
	if err != nil {
		tracker.fail(merged.Name, "missing vCenter password")
		return fmt.Errorf("site %s: %w", site.ID, err)
	}
	mgrCliPass, err := secret("mgrCliPassword", merged.MgrCliPasswordEnv, merged.MgrCliPassword)
	if err != nil {
		tracker.fail(merged.Name, "missing CLI password")
		return fmt.Errorf("site %s: %w", site.ID, err)
	}
	mgrRootPass, err := secret("mgrRootPassword", merged.MgrRootPasswordEnv, merged.MgrRootPassword)
	if err != nil {
		tracker.fail(merged.Name, "missing root password")
		return fmt.Errorf("site %s: %w", site.ID, err)
	}
	tcaUserPass, err := secret("tcaUserPassword", merged.TcaUserPasswordEnv, merged.TcaUserPassword)
	if err != nil {
		tracker.fail(merged.Name, "missing TCA user password")
		return fmt.Errorf("site %s: %w", site.ID, err)
	}

	targetURL := buildVCenterURL(merged.Username, vcPassword, merged.VCenterHost, merged.Datacenter, merged.Cluster)

	logRoot := baseLogDir
	if merged.LogDir != "" {
		logRoot = merged.LogDir
	}
	if logRoot == "" {
		logRoot = "output/logs"
	}

	siteLogDir := filepath.Join(logRoot, sanitize(merged.Name))
	if err := os.MkdirAll(siteLogDir, 0o755); err != nil {
		tracker.fail(merged.Name, "create log dir failed")
		return fmt.Errorf("site %s (%s): create log dir: %w", site.ID, merged.Name, err)
	}

	logFile := filepath.Join(siteLogDir, "ovf_deployment.log")
	stdoutPath := filepath.Join(siteLogDir, "ovf_stdout.log")
	stderrPath := filepath.Join(siteLogDir, "ovf_stderr.log")

	args := buildArgs(merged, ovaPath, targetURL, logFile, mgrCliPass, mgrRootPass, tcaUserPass)

	if dryRun {
		fmt.Printf("[DRY RUN] %s (%s) -> %s\n", site.ID, merged.Name, maskSecrets(ovftool, args, vcPassword, mgrCliPass, mgrRootPass, tcaUserPass))
		return nil
	}

	if err := runOvftool(merged.Name, ovftool, args, stdoutPath, stderrPath, merged.TimeoutMinutes, tracker); err != nil {
		tracker.fail(merged.Name, shortError(err))
		return fmt.Errorf("site %s (%s): %w", site.ID, merged.Name, err)
	}

	tracker.update(merged.Name, "Done", 100)
	return nil
}

func mergeSite(common CommonConfig, site SiteConfig) SiteConfig {
	merged := site

	merged.OvfToolPath = firstNonEmpty(site.OvfToolPath, common.OvfToolPath)
	merged.OvaPath = firstNonEmpty(site.OvaPath, common.OvaPath)
	merged.Username = firstNonEmpty(site.Username, common.Username)
	merged.Datastore = firstNonEmpty(site.Datastore, common.Datastore)
	merged.Cluster = firstNonEmpty(site.Cluster, common.Cluster)
	merged.VMFolder = firstNonEmpty(site.VMFolder, common.VMFolder)
	merged.DiskMode = firstNonEmpty(site.DiskMode, common.DiskMode)
	merged.DeploymentOption = firstNonEmpty(site.DeploymentOption, common.DeploymentOption)

	if len(merged.DNSList) == 0 {
		merged.DNSList = common.DNSList
	}
	if len(merged.NTPList) == 0 {
		merged.NTPList = common.NTPList
	}
	if len(merged.NetworkMappings) == 0 {
		merged.NetworkMappings = common.NetworkMappings
	}
	if merged.SSHEnabled == nil {
		merged.SSHEnabled = ptrBool(common.SSHEnabled)
	}
	if merged.ServiceWFH == nil {
		merged.ServiceWFH = ptrBool(common.ServiceWFH)
	}
	merged.ApplianceRole = firstNonEmpty(site.ApplianceRole, common.ApplianceRole)
	merged.IPProtocol = firstNonEmpty(site.IPProtocol, common.IPProtocol)
	merged.WaitForIPSeconds = fallbackInt(site.WaitForIPSeconds, common.WaitForIPSeconds, 0)
	merged.TimeoutMinutes = fallbackInt(site.TimeoutMinutes, common.TimeoutMinutes, 60)
	merged.ExtraArgs = append(common.ExtraArgs, site.ExtraArgs...)

	merged.MgrCliPassword = firstNonEmpty(site.MgrCliPassword, common.MgrCliPassword)
	merged.MgrCliPasswordEnv = firstNonEmpty(site.MgrCliPasswordEnv, common.MgrCliPasswordEnv)
	merged.MgrRootPassword = firstNonEmpty(site.MgrRootPassword, common.MgrRootPassword)
	merged.MgrRootPasswordEnv = firstNonEmpty(site.MgrRootPasswordEnv, common.MgrRootPasswordEnv)
	merged.TcaUserPassword = firstNonEmpty(site.TcaUserPassword, common.TcaUserPassword)
	merged.TcaUserPasswordEnv = firstNonEmpty(site.TcaUserPasswordEnv, common.TcaUserPasswordEnv)

	merged.VCenterPassword = firstNonEmpty(site.VCenterPassword, common.VCenterPassword)
	merged.VCenterPasswordEnv = firstNonEmpty(site.VCenterPasswordEnv, common.VCenterPasswordEnv)

	if merged.IPProtocol == "" {
		merged.IPProtocol = "IPv4"
	}
	if merged.ApplianceRole == "" {
		merged.ApplianceRole = "ControlPlane"
	}
	if merged.WaitForIPSeconds == 0 {
		merged.WaitForIPSeconds = 1800
	}
	if merged.LogDir == "" {
		merged.LogDir = common.LogDir
	}
	if merged.LogLevel == "" {
		merged.LogLevel = common.LogLevel
	}

	return merged
}

func buildArgs(cfg SiteConfig, ovaPath, targetURL, logFile, mgrCliPass, mgrRootPass, tcaUserPass string) []string {
	args := []string{
		"--acceptAllEulas",
		"--noSSLVerify",
		"--disableVerification",
		"--allowExtraConfig",
		"--X:enableHiddenProperties",
		"--X:injectOvfEnv",
		"--X:waitForIp",
		"--X:logFile=" + logFile,
		"--X:logLevel=" + firstNonEmpty(cfg.LogLevel, "verbose"),
		"--name=" + cfg.Name,
		"--datastore=" + cfg.Datastore,
		"--vmFolder=" + cfg.VMFolder,
		"--diskMode=" + cfg.DiskMode,
		"--deploymentOption=" + cfg.DeploymentOption,
		"--prop:mgr_ip_protocol=" + cfg.IPProtocol,
		"--prop:mgr_ip_0=" + cfg.IP,
		"--prop:mgr_prefix_ip_0=" + cfg.Prefix,
		"--prop:mgr_gateway_0=" + cfg.Gateway,
		"--prop:mgr_dns_list=" + strings.Join(cfg.DNSList, ","),
		"--prop:mgr_ntp_list=" + strings.Join(cfg.NTPList, ","),
		"--prop:mgr_cli_passwd=" + mgrCliPass,
		"--prop:mgr_root_passwd=" + mgrRootPass,
		"--prop:tca_user_passwd=" + tcaUserPass,
		"--prop:externalAddress=" + cfg.ExternalAddress,
		"--prop:mgr_isSSHEnabled=" + boolToString(cfg.SSHEnabled),
		"--prop:hostname=" + cfg.Hostname,
		"--prop:service_wfh=" + boolToString(cfg.ServiceWFH),
		"--prop:applianceRole=" + cfg.ApplianceRole,
	}

	for key, netName := range cfg.NetworkMappings {
		args = append(args, fmt.Sprintf("--net:%s=%s", key, netName))
	}

	args = append(args, cfg.ExtraArgs...)
	args = append(args, ovaPath, targetURL)
	return args
}

func runOvftool(vmName, binary string, args []string, stdoutPath, stderrPath string, timeoutMinutes int, tracker *progressTracker) error {
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return fmt.Errorf("create stdout log: %w", err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return fmt.Errorf("create stderr log: %w", err)
	}
	defer stderrFile.Close()

	ctx := context.Background()
	if timeoutMinutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMinutes)*time.Minute)
		defer cancel()
	}

	// Pipe stdout through a scanner to parse progress, while also writing to log file
	stdoutPipeR, stdoutPipeW := io.Pipe()
	stderrPipeR, stderrPipeW := io.Pipe()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = stdoutPipeW
	cmd.Stderr = stderrPipeW

	var scanWg sync.WaitGroup
	scanWg.Add(2)

	// scanCRLF splits on \n or \r so we catch ovftool's carriage-return progress updates
	scanCRLF := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == '\n' || b == '\r' {
				return i + 1, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	parseFn := func(r io.Reader, logFile *os.File) {
		scanner := bufio.NewScanner(r)
		scanner.Split(scanCRLF)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(logFile, line)

			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			phase, progress := parseLine(trimmed)
			if phase == "" {
				continue
			}
			if phase == "Failed" {
				tracker.fail(vmName, trimmed)
			} else {
				tracker.update(vmName, phase, progress)
			}
		}
	}

	// Scan stdout for progress
	go func() {
		defer scanWg.Done()
		parseFn(stdoutPipeR, stdoutFile)
	}()

	// Scan stderr for errors
	go func() {
		defer scanWg.Done()
		parseFn(stderrPipeR, stderrFile)
	}()

	if err := cmd.Start(); err != nil {
		stdoutPipeW.Close()
		stderrPipeW.Close()
		return fmt.Errorf("ovftool start: %w", err)
	}

	// Close write ends after process exits so scanners finish
	var cmdErr error
	go func() {
		cmdErr = cmd.Wait()
		stdoutPipeW.Close()
		stderrPipeW.Close()
	}()

	scanWg.Wait()

	if cmdErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("ovftool timed out after %d minute(s)", timeoutMinutes)
		}
		return fmt.Errorf("ovftool error: %w", cmdErr)
	}
	return nil
}

func buildVCenterURL(user, password, host, datacenter, cluster string) string {
	u := &url.URL{
		Scheme: "vi",
		User:   url.UserPassword(user, password),
		Host:   host,
		Path:   fmt.Sprintf("/%s/host/%s", url.PathEscape(datacenter), url.PathEscape(cluster)),
	}
	return u.String()
}

func secret(label, envName, inline string) (string, error) {
	if envName != "" {
		if val := os.Getenv(envName); val != "" {
			return val, nil
		}
	}
	if inline != "" {
		return inline, nil
	}
	return "", fmt.Errorf("missing %s (set env %s or provide inline value)", label, envName)
}

func boolToString(b *bool) string {
	if b == nil {
		return "False"
	}
	if *b {
		return "True"
	}
	return "False"
}

func sanitize(s string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		"@", "_",
		"/", "_",
		"\\", "_",
		":", "_",
	)
	return replacer.Replace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseList(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ptrBool(b bool) *bool {
	v := b
	return &v
}

func fallbackInt(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func maskSecrets(bin string, args []string, secrets ...string) string {
	masked := make([]string, len(args))
	for i, a := range args {
		masked[i] = a
		for _, sec := range secrets {
			if sec == "" {
				continue
			}
			if strings.Contains(masked[i], sec) {
				masked[i] = strings.ReplaceAll(masked[i], sec, "********")
			}
		}
	}
	return bin + " " + strings.Join(masked, " ")
}

func shortError(err error) string {
	s := err.Error()
	// Remove the "site ... :" prefix if present
	if idx := strings.LastIndex(s, ": "); idx >= 0 {
		s = s[idx+2:]
	}
	return s
}

func exitWithErr(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
