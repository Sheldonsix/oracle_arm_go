package main

import (
	"encoding/base64"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseTerraform(t *testing.T) {
	tf := `
resource "oci_core_instance" "x" {
  compartment_id = "ocid1.compartment"
  availability_domain = "abc:PHX-AD-1"
  display_name = "arm vm"
  shape_config {
    ocpus = "2"
    memory_in_gbs = 12
  }
  create_vnic_details {
    subnet_id = "ocid1.subnet"
  }
  source_details {
    source_id = "ocid1.image"
    boot_volume_size_in_gbs = "80"
  }
  metadata = {
    "ssh_authorized_keys" = "ssh-rsa AAA test"
  }
}
`
	got, err := parseTerraform(tf)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompartmentID != "ocid1.compartment" || got.SubnetID != "ocid1.subnet" || got.ImageID != "ocid1.image" {
		t.Fatalf("wrong ids: %#v", got)
	}
	if got.DisplayName != "arm vm" || got.OCPUs != 2 || got.MemoryGBs != 12 || got.BootVolumeGBs != 80 {
		t.Fatalf("wrong shape fields: %#v", got)
	}
	if got.SSHAuthorizedKeys != "ssh-rsa AAA test" {
		t.Fatalf("wrong ssh key: %q", got.SSHAuthorizedKeys)
	}
}

func TestLoadConfigEnvOnly(t *testing.T) {
	env := mapEnv(map[string]string{
		"ORACLE_ARM_COMPARTMENT_ID":        "ocid1.compartment",
		"ORACLE_ARM_AVAILABILITY_DOMAIN":   "ad-1",
		"ORACLE_ARM_SUBNET_ID":             "ocid1.subnet",
		"ORACLE_ARM_DISPLAY_NAME":          "my vm",
		"ORACLE_ARM_IMAGE_ID":              "ocid1.image",
		"ORACLE_ARM_OCPUS":                 "1",
		"ORACLE_ARM_MEMORY_GBS":            "6",
		"ORACLE_ARM_INITIAL_SLEEP_SECONDS": "7",
		"ORACLE_ARM_ENABLE_ROOT_PASSWORD":  "false",
		"ORACLE_ARM_SSH_AUTHORIZED_KEYS":   "ssh-rsa AAA",
	})
	cfg, err := loadConfig(nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Instance.DisplayName != "my-vm" || cfg.Instance.HostnameLabel != "my-vm" {
		t.Fatalf("names were not normalized: %#v", cfg.Instance)
	}
	if cfg.Instance.BootVolumeGBs != defaultBootVolumeGBs {
		t.Fatalf("boot default = %d", cfg.Instance.BootVolumeGBs)
	}
	if cfg.InitialSleep != 7*time.Second {
		t.Fatalf("initial sleep = %s", cfg.InitialSleep)
	}
}

func TestCloudInitQuotesPassword(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(cloudInitUserData("a'b"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `'root:a'\''b'`) {
		t.Fatalf("password was not shell-quoted:\n%s", raw)
	}
}

func TestRetryActionRetriesTemporaryNetworkErrors(t *testing.T) {
	errs := []error{
		&url.Error{Op: "Post", URL: "https://iaas.example", Err: timeoutErr{}},
		&url.Error{Op: "Post", URL: "https://iaas.example", Err: io.EOF},
		&url.Error{Op: "Post", URL: "https://iaas.example", Err: io.ErrUnexpectedEOF},
	}
	for _, err := range errs {
		action, msg := retryAction(err)
		if action != slowDown || msg != "temporary network error" {
			t.Fatalf("retryAction(%v) = %v, %q", err, action, msg)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "net/http: TLS handshake timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
