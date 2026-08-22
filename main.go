package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

const defaultBootVolumeGBs int64 = 50

type instanceConfig struct {
	CompartmentID      string
	AvailabilityDomain string
	SubnetID           string
	DisplayName        string
	HostnameLabel      string
	ImageID            string
	SSHAuthorizedKeys  string
	OCPUs              float64
	MemoryGBs          float64
	BootVolumeGBs      int64
}

type config struct {
	TFFile                string
	OCIConfigFile         string
	OCIProfile            string
	OCIPrivateKeyPassword string
	Shape                 string
	InitialSleep          time.Duration
	MinSleep              time.Duration
	MaxSleep              time.Duration
	PublicIPWait          time.Duration
	PublicIPInterval      time.Duration
	AssignPublicIP        bool
	PVEncryption          bool
	EnableRootPassword    bool
	RootPassword          string
	Telegram              telegramConfig
	Instance              instanceConfig
}

type telegramConfig struct {
	Enabled bool
	Token   string
	ChatID  string
	APIHost string
}

type notifier struct {
	cfg    telegramConfig
	client *http.Client
}

type creator struct {
	cfg      config
	compute  core.ComputeClient
	network  core.VirtualNetworkClient
	notifier notifier
	log      strings.Builder
	password string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	cfg, err := loadConfig(args, getenv)
	if err != nil {
		return err
	}

	provider, err := common.ConfigurationProviderFromFileWithProfile(
		cfg.OCIConfigFile,
		cfg.OCIProfile,
		cfg.OCIPrivateKeyPassword,
	)
	if err != nil {
		return fmt.Errorf("load OCI config: %w", err)
	}

	compute, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return fmt.Errorf("create compute client: %w", err)
	}
	network, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return fmt.Errorf("create network client: %w", err)
	}

	c := creator{
		cfg:     cfg,
		compute: compute,
		network: network,
		notifier: notifier{
			cfg:    cfg.Telegram,
			client: &http.Client{Timeout: 15 * time.Second},
		},
	}
	return c.create(ctx, out)
}

func loadConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("oracle-arm", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	rest := fs.Args()

	cfg := config{
		TFFile:             strings.TrimSpace(getenv("ORACLE_ARM_TF_FILE")),
		OCIConfigFile:      envDefault(getenv, "OCI_CONFIG_FILE", "~/.oci/config"),
		OCIProfile:         envDefault(getenv, "OCI_CONFIG_PROFILE", "DEFAULT"),
		Shape:              envDefault(getenv, "ORACLE_ARM_SHAPE", "VM.Standard.A1.Flex"),
		InitialSleep:       5 * time.Second,
		MinSleep:           5 * time.Second,
		MaxSleep:           60 * time.Second,
		PublicIPWait:       10 * time.Minute,
		PublicIPInterval:   5 * time.Second,
		AssignPublicIP:     true,
		PVEncryption:       true,
		EnableRootPassword: true,
		Telegram: telegramConfig{
			Token:   strings.TrimSpace(getenv("TG_BOT_TOKEN")),
			ChatID:  strings.TrimSpace(firstNonEmpty(getenv("TG_CHAT_ID"), getenv("TG_USER_ID"))),
			APIHost: envDefault(getenv, "TG_API_HOST", "api.telegram.org"),
		},
		Instance: instanceConfig{BootVolumeGBs: defaultBootVolumeGBs},
	}
	if cfg.TFFile == "" && len(rest) > 0 {
		cfg.TFFile = rest[0]
	}
	cfg.OCIPrivateKeyPassword = getenv("OCI_PRIVATE_KEY_PASSWORD")
	cfg.RootPassword = getenv("ORACLE_ARM_ROOT_PASSWORD")

	var err error
	if cfg.InitialSleep, err = envSeconds(getenv, "ORACLE_ARM_INITIAL_SLEEP_SECONDS", cfg.InitialSleep); err != nil {
		return config{}, err
	}
	if cfg.MinSleep, err = envSeconds(getenv, "ORACLE_ARM_MIN_SLEEP_SECONDS", cfg.MinSleep); err != nil {
		return config{}, err
	}
	if cfg.MaxSleep, err = envSeconds(getenv, "ORACLE_ARM_MAX_SLEEP_SECONDS", cfg.MaxSleep); err != nil {
		return config{}, err
	}
	if cfg.PublicIPWait, err = envSeconds(getenv, "ORACLE_ARM_PUBLIC_IP_WAIT_SECONDS", cfg.PublicIPWait); err != nil {
		return config{}, err
	}
	if cfg.PublicIPInterval, err = envSeconds(getenv, "ORACLE_ARM_PUBLIC_IP_INTERVAL_SECONDS", cfg.PublicIPInterval); err != nil {
		return config{}, err
	}
	if cfg.AssignPublicIP, err = envBool(getenv, "ORACLE_ARM_ASSIGN_PUBLIC_IP", cfg.AssignPublicIP); err != nil {
		return config{}, err
	}
	if cfg.PVEncryption, err = envBool(getenv, "ORACLE_ARM_PV_ENCRYPTION", cfg.PVEncryption); err != nil {
		return config{}, err
	}
	if cfg.EnableRootPassword, err = envBool(getenv, "ORACLE_ARM_ENABLE_ROOT_PASSWORD", cfg.EnableRootPassword); err != nil {
		return config{}, err
	}
	cfg.Telegram.Enabled = cfg.Telegram.Token != "" && cfg.Telegram.ChatID != ""
	if cfg.Telegram.Enabled, err = envBool(getenv, "TG_ENABLED", cfg.Telegram.Enabled); err != nil {
		return config{}, err
	}
	if cfg.Telegram.Enabled && (cfg.Telegram.Token == "" || cfg.Telegram.ChatID == "") {
		return config{}, errors.New("TG_ENABLED=true requires TG_BOT_TOKEN and TG_CHAT_ID")
	}

	if cfg.TFFile != "" {
		tf, err := parseTerraformFile(cfg.TFFile)
		if err != nil {
			return config{}, err
		}
		cfg.Instance = mergeInstance(cfg.Instance, tf)
	}
	if err := applyInstanceEnv(getenv, &cfg.Instance); err != nil {
		return config{}, err
	}
	if cfg.Instance.HostnameLabel == "" {
		cfg.Instance.HostnameLabel = hostnameLabel(cfg.Instance.DisplayName)
	}
	return cfg, validateConfig(cfg)
}

func parseTerraformFile(path string) (instanceConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return instanceConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parseTerraform(string(b))
}

func parseTerraform(data string) (instanceConfig, error) {
	var cfg instanceConfig
	cfg.CompartmentID, _ = tfValue(data, "compartment_id", true)
	cfg.AvailabilityDomain, _ = tfValue(data, "availability_domain", true)
	cfg.SubnetID, _ = tfValue(data, "subnet_id", true)
	cfg.DisplayName, _ = tfValue(data, "display_name", true)
	cfg.ImageID, _ = tfValue(data, "source_id", false)
	cfg.SSHAuthorizedKeys, _ = tfValue(data, "ssh_authorized_keys", true)

	if err := parseOptionalFloat(data, "ocpus", &cfg.OCPUs); err != nil {
		return instanceConfig{}, err
	}
	if err := parseOptionalFloat(data, "memory_in_gbs", &cfg.MemoryGBs); err != nil {
		return instanceConfig{}, err
	}
	if err := parseOptionalInt64(data, "boot_volume_size_in_gbs", &cfg.BootVolumeGBs); err != nil {
		return instanceConfig{}, err
	}
	return cfg, nil
}

func tfValue(data, key string, last bool) (string, bool) {
	re := regexp.MustCompile(`(?m)^\s*"?` + regexp.QuoteMeta(key) + `"?\s*=\s*(?:"([^"]*)"|([^\s#]+))`)
	matches := re.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		return "", false
	}
	i := 0
	if last {
		i = len(matches) - 1
	}
	if matches[i][1] != "" {
		return strings.TrimSpace(matches[i][1]), true
	}
	return strings.TrimSpace(strings.TrimRight(matches[i][2], ",")), true
}

func parseOptionalFloat(data, key string, dst *float64) error {
	raw, ok := tfValue(data, key, true)
	if !ok || raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("%s must be a number: %q", key, raw)
	}
	*dst = v
	return nil
}

func parseOptionalInt64(data, key string, dst *int64) error {
	raw, ok := tfValue(data, key, true)
	if !ok || raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v != float64(int64(v)) {
		return fmt.Errorf("%s must be an integer GB value: %q", key, raw)
	}
	*dst = int64(v)
	return nil
}

func mergeInstance(base, override instanceConfig) instanceConfig {
	if override.CompartmentID != "" {
		base.CompartmentID = override.CompartmentID
	}
	if override.AvailabilityDomain != "" {
		base.AvailabilityDomain = override.AvailabilityDomain
	}
	if override.SubnetID != "" {
		base.SubnetID = override.SubnetID
	}
	if override.DisplayName != "" {
		base.DisplayName = strings.ReplaceAll(strings.TrimSpace(override.DisplayName), " ", "-")
	}
	if override.HostnameLabel != "" {
		base.HostnameLabel = override.HostnameLabel
	}
	if override.ImageID != "" {
		base.ImageID = override.ImageID
	}
	if override.SSHAuthorizedKeys != "" {
		base.SSHAuthorizedKeys = override.SSHAuthorizedKeys
	}
	if override.OCPUs != 0 {
		base.OCPUs = override.OCPUs
	}
	if override.MemoryGBs != 0 {
		base.MemoryGBs = override.MemoryGBs
	}
	if override.BootVolumeGBs != 0 {
		base.BootVolumeGBs = override.BootVolumeGBs
	}
	return base
}

func applyInstanceEnv(getenv func(string) string, cfg *instanceConfig) error {
	applyString(getenv, "ORACLE_ARM_COMPARTMENT_ID", &cfg.CompartmentID)
	applyString(getenv, "ORACLE_ARM_AVAILABILITY_DOMAIN", &cfg.AvailabilityDomain)
	applyString(getenv, "ORACLE_ARM_SUBNET_ID", &cfg.SubnetID)
	applyString(getenv, "ORACLE_ARM_DISPLAY_NAME", &cfg.DisplayName)
	applyString(getenv, "ORACLE_ARM_HOSTNAME_LABEL", &cfg.HostnameLabel)
	applyString(getenv, "ORACLE_ARM_IMAGE_ID", &cfg.ImageID)
	applyString(getenv, "ORACLE_ARM_SSH_AUTHORIZED_KEYS", &cfg.SSHAuthorizedKeys)

	var err error
	if cfg.OCPUs, err = envFloat(getenv, "ORACLE_ARM_OCPUS", cfg.OCPUs); err != nil {
		return err
	}
	if cfg.MemoryGBs, err = envFloat(getenv, "ORACLE_ARM_MEMORY_GBS", cfg.MemoryGBs); err != nil {
		return err
	}
	if cfg.BootVolumeGBs, err = envInt64(getenv, "ORACLE_ARM_BOOT_VOLUME_GBS", cfg.BootVolumeGBs); err != nil {
		return err
	}
	cfg.DisplayName = strings.ReplaceAll(strings.TrimSpace(cfg.DisplayName), " ", "-")
	return nil
}

func validateConfig(cfg config) error {
	missing := []string{}
	addMissing := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	addMissing("compartment_id / ORACLE_ARM_COMPARTMENT_ID", cfg.Instance.CompartmentID)
	addMissing("availability_domain / ORACLE_ARM_AVAILABILITY_DOMAIN", cfg.Instance.AvailabilityDomain)
	addMissing("subnet_id / ORACLE_ARM_SUBNET_ID", cfg.Instance.SubnetID)
	addMissing("display_name / ORACLE_ARM_DISPLAY_NAME", cfg.Instance.DisplayName)
	addMissing("source_id / ORACLE_ARM_IMAGE_ID", cfg.Instance.ImageID)
	if cfg.Instance.OCPUs <= 0 {
		missing = append(missing, "ocpus / ORACLE_ARM_OCPUS")
	}
	if cfg.Instance.MemoryGBs <= 0 {
		missing = append(missing, "memory_in_gbs / ORACLE_ARM_MEMORY_GBS")
	}
	if cfg.Instance.BootVolumeGBs <= 0 {
		missing = append(missing, "boot_volume_size_in_gbs / ORACLE_ARM_BOOT_VOLUME_GBS")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing config: %s", strings.Join(missing, ", "))
	}
	if cfg.MinSleep <= 0 || cfg.InitialSleep <= 0 || cfg.MaxSleep <= 0 || cfg.MinSleep > cfg.MaxSleep {
		return errors.New("sleep seconds must be positive and ORACLE_ARM_MIN_SLEEP_SECONDS <= ORACLE_ARM_MAX_SLEEP_SECONDS")
	}
	if cfg.PublicIPWait <= 0 || cfg.PublicIPInterval <= 0 {
		return errors.New("public IP wait and interval seconds must be positive")
	}
	if !cfg.EnableRootPassword && cfg.Instance.SSHAuthorizedKeys == "" {
		return errors.New("ORACLE_ARM_ENABLE_ROOT_PASSWORD=false requires ORACLE_ARM_SSH_AUTHORIZED_KEYS or ssh_authorized_keys in main.tf")
	}
	if strings.ContainsAny(cfg.RootPassword, "\r\n") {
		return errors.New("ORACLE_ARM_ROOT_PASSWORD cannot contain newlines")
	}
	return nil
}

func (c *creator) create(ctx context.Context, out io.Writer) error {
	if c.cfg.EnableRootPassword {
		if c.cfg.RootPassword == "" {
			var err error
			c.password, err = randomPassword(16)
			if err != nil {
				return err
			}
		} else {
			c.password = c.cfg.RootPassword
		}
		fmt.Fprintf(out, "root password: %s\n", c.password)
	}

	start := fmt.Sprintf("oracle-arm started: ad=%s name=%s cpu=%.2f memory=%.2fGB boot=%dGB",
		c.cfg.Instance.AvailabilityDomain,
		c.cfg.Instance.DisplayName,
		c.cfg.Instance.OCPUs,
		c.cfg.Instance.MemoryGBs,
		c.cfg.Instance.BootVolumeGBs,
	)
	fmt.Fprintln(out, start)
	if err := c.notifier.send(ctx, start); err != nil {
		fmt.Fprintf(out, "telegram failed: %v\n", err)
	}

	sleep := c.cfg.InitialSleep
	for tries := 1; ; tries++ {
		resp, err := c.compute.LaunchInstance(ctx, c.launchRequest())
		if err != nil {
			action, msg := retryAction(err)
			if action == stopRetry {
				text := fmt.Sprintf("create failed after %d tries: %v", tries, err)
				_ = c.notifier.send(ctx, text)
				return errors.Join(errors.New(msg), err)
			}
			if action == slowDown && sleep < c.cfg.MaxSleep {
				sleep += 10 * time.Second
				if sleep > c.cfg.MaxSleep {
					sleep = c.cfg.MaxSleep
				}
			}
			if action == speedUp && sleep > c.cfg.MinSleep {
				sleep -= 10 * time.Second
				if sleep < c.cfg.MinSleep {
					sleep = c.cfg.MinSleep
				}
			}
			fmt.Fprintf(out, "try %d failed: %v; next try in %s\n", tries, err, sleep)
			if err := sleepContext(ctx, sleep); err != nil {
				return err
			}
			continue
		}

		instanceID := value(resp.Instance.Id)
		c.logf("created after %d tries: name=%s cpu=%.2f memory=%.2fGB",
			tries,
			c.cfg.Instance.DisplayName,
			c.cfg.Instance.OCPUs,
			c.cfg.Instance.MemoryGBs,
		)
		if c.password != "" {
			c.logf("root password: %s", c.password)
		}
		if ip, err := c.waitPublicIP(ctx, instanceID); err != nil {
			c.logf("public ip lookup failed: %v", err)
		} else if ip != "" {
			c.logf("public ip: %s", ip)
		} else {
			c.logf("public ip is empty; check subnet public IP settings")
		}
		if err := c.notifier.send(ctx, strings.TrimSpace(c.log.String())); err != nil {
			fmt.Fprintf(out, "telegram failed: %v\n", err)
		}
		fmt.Fprint(out, c.log.String())
		return nil
	}
}

func (c *creator) launchRequest() core.LaunchInstanceRequest {
	details := core.LaunchInstanceDetails{
		DisplayName:        common.String(c.cfg.Instance.DisplayName),
		CompartmentId:      common.String(c.cfg.Instance.CompartmentID),
		Shape:              common.String(c.cfg.Shape),
		AvailabilityDomain: common.String(c.cfg.Instance.AvailabilityDomain),
		ShapeConfig: &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(float32(c.cfg.Instance.OCPUs)),
			MemoryInGBs: common.Float32(float32(c.cfg.Instance.MemoryGBs)),
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       common.String(c.cfg.Instance.SubnetID),
			HostnameLabel:  common.String(c.cfg.Instance.HostnameLabel),
			AssignPublicIp: common.Bool(c.cfg.AssignPublicIP),
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(c.cfg.Instance.ImageID),
			BootVolumeSizeInGBs: common.Int64(c.cfg.Instance.BootVolumeGBs),
		},
		IsPvEncryptionInTransitEnabled: common.Bool(c.cfg.PVEncryption),
	}
	if c.cfg.EnableRootPassword {
		details.ExtendedMetadata = map[string]interface{}{"user_data": cloudInitUserData(c.password)}
	}
	if c.cfg.Instance.SSHAuthorizedKeys != "" {
		details.Metadata = map[string]string{"ssh_authorized_keys": c.cfg.Instance.SSHAuthorizedKeys}
	}
	return core.LaunchInstanceRequest{LaunchInstanceDetails: details}
}

func (c *creator) waitPublicIP(ctx context.Context, instanceID string) (string, error) {
	deadline := time.Now().Add(c.cfg.PublicIPWait)
	for time.Now().Before(deadline) {
		resp, err := c.compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
			CompartmentId: common.String(c.cfg.Instance.CompartmentID),
			InstanceId:    common.String(instanceID),
		})
		if err != nil {
			return "", err
		}
		if len(resp.Items) > 0 && resp.Items[0].VnicId != nil {
			vnic, err := c.network.GetVnic(ctx, core.GetVnicRequest{VnicId: resp.Items[0].VnicId})
			if err != nil {
				return "", err
			}
			return value(vnic.Vnic.PublicIp), nil
		}
		if err := sleepContext(ctx, c.cfg.PublicIPInterval); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("timed out after %s", c.cfg.PublicIPWait)
}

func (c *creator) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	c.log.WriteString(line)
	c.log.WriteByte('\n')
}

type retryKind int

const (
	stopRetry retryKind = iota
	slowDown
	speedUp
)

func retryAction(err error) (retryKind, string) {
	serviceErr, ok := common.IsServiceError(err)
	if !ok {
		return stopRetry, "non-OCI error"
	}
	status, code, msg := serviceErr.GetHTTPStatusCode(), serviceErr.GetCode(), serviceErr.GetMessage()
	if status == http.StatusTooManyRequests || code == "TooManyRequests" {
		return slowDown, "rate limited"
	}
	if status == http.StatusInternalServerError && code == "InternalError" && strings.Contains(msg, "Out of host capacity") {
		return speedUp, "out of host capacity"
	}
	if status == http.StatusBadRequest && strings.Contains(msg, "Service limit") {
		return stopRetry, "service limit reached"
	}
	return stopRetry, "OCI returned a non-retryable error"
}

func cloudInitUserData(password string) string {
	script := `#!/bin/bash
set -e
printf '%s\n' ` + shellQuote("root:"+password) + ` | chpasswd
if grep -qE '^[#[:space:]]*PermitRootLogin[[:space:]]+' /etc/ssh/sshd_config; then
  sed -i 's|^[#[:space:]]*PermitRootLogin[[:space:]].*|PermitRootLogin yes|' /etc/ssh/sshd_config
else
  printf '%s\n' 'PermitRootLogin yes' >> /etc/ssh/sshd_config
fi
if grep -qE '^[#[:space:]]*PasswordAuthentication[[:space:]]+' /etc/ssh/sshd_config; then
  sed -i 's|^[#[:space:]]*PasswordAuthentication[[:space:]].*|PasswordAuthentication yes|' /etc/ssh/sshd_config
else
  printf '%s\n' 'PasswordAuthentication yes' >> /etc/ssh/sshd_config
fi
systemctl restart sshd || systemctl restart ssh || service sshd restart || service ssh restart || reboot
`
	return base64.StdEncoding.EncodeToString([]byte(script))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func randomPassword(n int) (string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789#@"
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b.WriteByte(chars[x.Int64()])
	}
	return b.String(), nil
}

func (n notifier) send(ctx context.Context, text string) error {
	if !n.cfg.Enabled {
		return nil
	}
	host := strings.TrimRight(n.cfg.APIHost, "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	form := url.Values{"chat_id": {n.cfg.ChatID}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/bot"+n.cfg.Token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func hostnameLabel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.TrimRight(out[:63], "-")
	}
	if out == "" || (out[0] < 'a' || out[0] > 'z') {
		out = "vm-" + out
	}
	return out
}

func envDefault(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(getenv func(string) string, key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return v, nil
}

func envSeconds(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return time.Duration(seconds) * time.Second, nil
}

func envFloat(getenv func(string) string, key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive number", key)
	}
	return v, nil
}

func envInt64(getenv func(string) string, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return v, nil
}

func applyString(getenv func(string) string, key string, dst *string) {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		*dst = v
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
