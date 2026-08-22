# oracle-arm-go

Go rewrite of the original Oracle ARM instance registration script.

It reads the same `main.tf` values when present, then lets environment
variables override them. No source edit is needed for Telegram, OCI profile,
shape, retry timing, or instance parameters.

## Build

```sh
go build -o oracle-arm .
```

## Configure

Copy the example env file and fill the values marked as required:

```sh
cp .env.example .env
```

Manual config required in most cases:

- `OCI_CONFIG_FILE`: OCI CLI config path, usually `$HOME/.oci/config`.
- `OCI_CONFIG_PROFILE`: OCI CLI profile, usually `DEFAULT`.
- `ORACLE_ARM_TF_FILE`: path to `main.tf` if you use the old Terraform export flow.
- `ORACLE_ARM_COMPARTMENT_ID`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_AVAILABILITY_DOMAIN`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_SUBNET_ID`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_DISPLAY_NAME`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_IMAGE_ID`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_OCPUS`: required only when not supplied by `main.tf`.
- `ORACLE_ARM_MEMORY_GBS`: required only when not supplied by `main.tf`.
- `TG_BOT_TOKEN` and `TG_CHAT_ID`: required only when `TG_ENABLED=true`.

Load config before running:

```sh
set -a
source .env
set +a

./oracle-arm
```

The old style still works:

```sh
./oracle-arm ./main.tf
```

## Environment Variables

Required from `main.tf` or env:

```sh
# Required when not supplied by main.tf.
ORACLE_ARM_COMPARTMENT_ID=
ORACLE_ARM_AVAILABILITY_DOMAIN=
ORACLE_ARM_SUBNET_ID=
ORACLE_ARM_DISPLAY_NAME=
ORACLE_ARM_IMAGE_ID=
ORACLE_ARM_OCPUS=1
ORACLE_ARM_MEMORY_GBS=6
```

Optional:

```sh
# OCI config.
OCI_CONFIG_FILE="$HOME/.oci/config"
OCI_CONFIG_PROFILE=DEFAULT
OCI_PRIVATE_KEY_PASSWORD=

# Instance defaults.
ORACLE_ARM_BOOT_VOLUME_GBS=50
ORACLE_ARM_SHAPE=VM.Standard.A1.Flex
ORACLE_ARM_ASSIGN_PUBLIC_IP=true
ORACLE_ARM_PV_ENCRYPTION=true

# Login.
ORACLE_ARM_ENABLE_ROOT_PASSWORD=true
ORACLE_ARM_ROOT_PASSWORD=
ORACLE_ARM_SSH_AUTHORIZED_KEYS=

# Retry and public IP lookup.
ORACLE_ARM_INITIAL_SLEEP_SECONDS=5
ORACLE_ARM_MIN_SLEEP_SECONDS=5
ORACLE_ARM_MAX_SLEEP_SECONDS=60
ORACLE_ARM_PUBLIC_IP_WAIT_SECONDS=600
ORACLE_ARM_PUBLIC_IP_INTERVAL_SECONDS=5
```

Telegram is disabled unless token and chat id are present, or `TG_ENABLED=true`
is set:

```sh
TG_ENABLED=true
TG_BOT_TOKEN=
TG_CHAT_ID=
TG_API_HOST=api.telegram.org
```

## Fixed from the Python version

- Telegram no longer sends when disabled.
- Missing config now returns clear errors instead of panics.
- OCI config path/profile are environment-configurable.
- VNIC lookup uses the instance compartment, not always the tenancy.
- SSH key is optional when root password cloud-init is enabled.
- Generated root password uses cryptographic randomness.
