# oracle-arm-go

[English](#english) | [中文说明](#中文说明)

---

## English

Go rewrite of the original [Oracle ARM instance registration script](https://github.com/Zpipishrimp/oracle_arm).
This tool automates the process of registering and provisioning Oracle Cloud Free Tier ARM instances. It reads the same `main.tf` values when present, then lets environment variables override them. No source code editing is required for configuring Telegram, OCI profiles, shapes, retry timing, or instance parameters.

### 🚀 Quick Start

#### 1. Download the Pre-built Binary
You can install the latest version automatically using our one-click installation script:

```sh
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
cd ~/oracle-arm-go
```
*(This script installs to `~/oracle-arm-go`, auto-detects your OS and Architecture, downloads the binary, and creates `.env` when missing. You can also manually download it from the [Releases page](https://github.com/Sheldonsix/oracle_arm_go/releases).)*

Custom install directory:

```sh
export INSTALL_DIR="$HOME/apps/oracle-arm-go"
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
```

Update:

```sh
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
cd ~/oracle-arm-go
```

The installer overwrites `oracle-arm` and `.env.example`, but keeps your existing
`.env`. If the old program is already running, stop it and start it again so the
new binary is used. If you installed to a custom directory, export the same
`INSTALL_DIR` before running the update command.

#### 2. Prepare OCI API Config
This tool uses the same API config file as OCI CLI. If `~/.oci/config` already
works on this machine, keep the default `.env` values:

```sh
OCI_CONFIG_FILE="$HOME/.oci/config"
OCI_CONFIG_PROFILE="DEFAULT"
```

If OCI CLI is not configured yet:

```sh
bash -c "$(curl -L https://raw.githubusercontent.com/oracle/oci-cli/master/scripts/install/install.sh)"
exec -l "$SHELL"
oci -v
oci setup config
oci iam availability-domain list
```

For a screenshot walkthrough, see [Daniao's OCI setup guide](https://www.daniao.org/14035.html),
steps **3** and **4**: copy the user OCID and tenancy OCID from the Oracle
Console, then configure OCI CLI and upload the generated public key.

During `oci setup config`, fill in your user OCID, tenancy OCID, region, and API
key. Then upload the generated public key:

```sh
cat ~/.oci/oci_api_key_public.pem
```

In Oracle Console, add it under **User Settings → Resources → API Keys → Add API
Key**. The generated `~/.oci/config` should look like:

```ini
[DEFAULT]
user=ocid1.user.oc1...
fingerprint=xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx
tenancy=ocid1.tenancy.oc1...
region=ap-tokyo-1
key_file=/home/your-user/.oci/oci_api_key.pem
```

If the private key has a passphrase, also set `OCI_PRIVATE_KEY_PASSWORD` in
`.env`.

#### 3. Configure
Fill in the required values:

```sh
nano .env
```

*Note: If you have an existing `main.tf` from the old Terraform export flow, you can still use it by passing it as an argument or setting `ORACLE_ARM_TF_FILE`.*

**How to get `main.tf`?**
You can refer to [this tutorial](https://www.daniao.org/14121.html) (Step 1). Go to the Oracle Cloud instance creation page, configure your desired ARM instance, but instead of clicking "Create", click **"Save as stack"** to download the Terraform configuration zip file, which contains the `main.tf` file.

#### 4. Run
Load the environment variables and start the script:

```sh
set -a; source .env; set +a
./oracle-arm
```
*(Old style `./oracle-arm ./main.tf` is still supported).*

> **💡 Pro Tip: Running in the Background**
> Since this script needs to run continuously until an instance is provisioned, consider running it in the background using `nohup` or `tmux`:
> ```sh
> set -a; source .env; set +a
> nohup ./oracle-arm > run.log 2>&1 &
> ```

### ⚙️ Configuration (Environment Variables)

**Manual configuration required in most cases:**
- `OCI_CONFIG_FILE`: OCI CLI config path, usually `$HOME/.oci/config`.
- `OCI_CONFIG_PROFILE`: OCI CLI profile, usually `DEFAULT`.
- `TG_BOT_TOKEN` and `TG_CHAT_ID`: required only when `TG_ENABLED=true`.

**Required from `main.tf` or `.env`:**
```sh
ORACLE_ARM_COMPARTMENT_ID=
ORACLE_ARM_AVAILABILITY_DOMAIN=
ORACLE_ARM_SUBNET_ID=
ORACLE_ARM_DISPLAY_NAME=
ORACLE_ARM_IMAGE_ID=
ORACLE_ARM_OCPUS=1
ORACLE_ARM_MEMORY_GBS=6
```

**Optional Settings (OCI, Instance, Login, Retry, Telegram):**
```sh
# OCI
OCI_CONFIG_FILE="$HOME/.oci/config"
OCI_CONFIG_PROFILE=DEFAULT
OCI_PRIVATE_KEY_PASSWORD=

# Instance defaults
ORACLE_ARM_BOOT_VOLUME_GBS=50
ORACLE_ARM_SHAPE=VM.Standard.A1.Flex
ORACLE_ARM_ASSIGN_PUBLIC_IP=true
ORACLE_ARM_PV_ENCRYPTION=true

# Login
ORACLE_ARM_ENABLE_ROOT_PASSWORD=true
ORACLE_ARM_ROOT_PASSWORD=
ORACLE_ARM_SSH_AUTHORIZED_KEYS=

# Retry timing
ORACLE_ARM_INITIAL_SLEEP_SECONDS=5
ORACLE_ARM_MIN_SLEEP_SECONDS=5
ORACLE_ARM_MAX_SLEEP_SECONDS=60
ORACLE_ARM_PUBLIC_IP_WAIT_SECONDS=600
ORACLE_ARM_PUBLIC_IP_INTERVAL_SECONDS=5

# Telegram
TG_ENABLED=false
TG_BOT_TOKEN=
TG_CHAT_ID=
TG_API_HOST=api.telegram.org
```

### 🛠️ Build from Source

*Prerequisite: Go installed on your system.*

```sh
git clone https://github.com/Sheldonsix/oracle_arm_go.git
cd oracle_arm_go
go build -o oracle-arm .
```
Cross-build example (for Linux ARM64):
```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o oracle-arm .
```

### ✨ Fixed from the original Python version
- Telegram no longer sends notifications when disabled.
- Missing config now returns clear errors instead of panics/crashes.
- OCI config path/profile are environment-configurable.
- VNIC lookup uses the instance compartment, not always the tenancy.
- SSH key is optional when root password cloud-init is enabled.
- Generated root password uses cryptographic randomness.

---

## 中文说明

使用 Go 语言重写的 [Oracle ARM 抢机脚本](https://github.com/Zpipishrimp/oracle_arm)。
本工具用于自动化监控并注册 Oracle Cloud 免费层 ARM 实例。它完全兼容原有的 `main.tf` 文件配置，并支持通过环境变量覆盖。无需修改任何源代码即可自定义 Telegram 通知、OCI 配置文件路径、机器配置 (Shape)、重试间隔以及各种实例参数。

### 🚀 快速开始 (上手指南)

#### 1. 下载预编译程序
你可以直接运行以下一键安装脚本，它会自动识别你的系统架构，并拉取最新版本：

```sh
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
cd ~/oracle-arm-go
```
*(脚本默认安装到 `~/oracle-arm-go`，会自动下载程序，并在缺少 `.env` 时创建配置文件。你也可以前往 [Releases 页面](https://github.com/Sheldonsix/oracle_arm_go/releases) 手动下载。)*

自定义安装目录：

```sh
export INSTALL_DIR="$HOME/apps/oracle-arm-go"
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
```

更新程序：

```sh
curl -sL https://raw.githubusercontent.com/Sheldonsix/oracle_arm_go/main/install.sh | bash
cd ~/oracle-arm-go
```

安装脚本会覆盖 `oracle-arm` 和 `.env.example`，但不会覆盖已有的 `.env`。如果旧程序正在运行，需要先停止旧进程，再重新启动，才会使用新的二进制。自定义安装目录的用户，更新前需要先 export 同一个 `INSTALL_DIR`。

#### 2. 准备 OCI API 配置
本工具使用和 OCI CLI 一样的 API 配置文件。如果这台机器上的 `~/.oci/config`
已经可用，`.env` 里保持默认即可：

```sh
OCI_CONFIG_FILE="$HOME/.oci/config"
OCI_CONFIG_PROFILE="DEFAULT"
```

如果还没有配置 OCI CLI：

```sh
bash -c "$(curl -L https://raw.githubusercontent.com/oracle/oci-cli/master/scripts/install/install.sh)"
exec -l "$SHELL"
oci -v
oci setup config
oci iam availability-domain list
```

图文步骤可以参考 [大鸟博客-Oracle甲骨文 ARM VPS（VM.Standard.A1.Flex）自动抢购脚本代码](https://www.daniao.org/14035.html)
里的 **步骤 3、复制租户和用户的 OCID** 和 **步骤 4、配置 CLI**：先在甲骨文后台复制用户 OCID、租户 OCID，再配置 OCI CLI 并上传公钥。

执行 `oci setup config` 时，需要填写用户 OCID、租户 OCID、区域和 API Key。然后复制生成的公钥：

```sh
cat ~/.oci/oci_api_key_public.pem
```

在甲骨文后台进入 **用户设置 → 资源 → API 秘钥 → 添加 API 秘钥**，把公钥内容粘贴进去。
生成的 `~/.oci/config` 大致如下：

```ini
[DEFAULT]
user=ocid1.user.oc1...
fingerprint=xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx
tenancy=ocid1.tenancy.oc1...
region=ap-tokyo-1
key_file=/home/your-user/.oci/oci_api_key.pem
```

如果你的私钥设置了密码，还需要在 `.env` 里填写 `OCI_PRIVATE_KEY_PASSWORD`。

#### 3. 修改配置
填写必要的参数：

```sh
nano .env
```

*提示：如果你使用的是旧版抓包得到的 `main.tf` Terraform 导出文件，你可以直接在命令行指定该文件，或者在环境变量中设置 `ORACLE_ARM_TF_FILE`。*

**如何获取 `main.tf` 文件？**
参考 [大鸟博客的教程](https://www.daniao.org/14121.html)（步骤 **1、生成 main.tf**）。具体方法：在甲骨文控制台正常配置实例参数，完毕后不要点击“创建”，而是点击页面下方的 **“另存为堆栈” (Save as stack)**，下载 Terraform 压缩包并解压，即可得到 `main.tf`。

#### 4. 运行程序
加载环境变量并启动脚本：

```sh
set -a; source .env; set +a
./oracle-arm
```
*(同时支持旧版运行方式: `./oracle-arm ./main.tf`)*

> **💡 进阶建议：后台持久运行**
> 抢机脚本通常需要长时间在服务器上挂机运行。推荐使用 `nohup` 或 `screen`/`tmux` 来保持程序在后台运行：
> ```sh
> set -a; source .env; set +a
> nohup ./oracle-arm > run.log 2>&1 &
> ```
> 运行后，你可以通过 `tail -f run.log` 随时查看实时运行日志。

### ⚙️ 配置指南 (环境变量)

**大多数情况下需要手动配置的项：**
- `OCI_CONFIG_FILE`: OCI CLI 配置文件路径，通常为 `$HOME/.oci/config`。
- `OCI_CONFIG_PROFILE`: OCI CLI 对应的 Profile 名称，通常为 `DEFAULT`。
- `TG_BOT_TOKEN` 和 `TG_CHAT_ID`: 仅在 `TG_ENABLED=true` (开启 Telegram 通知) 时需要填写。

**必须提供的实例参数 (可通过 `main.tf` 或 `.env` 提供)：**
```sh
ORACLE_ARM_COMPARTMENT_ID=     # 区间 ID
ORACLE_ARM_AVAILABILITY_DOMAIN= # 可用区
ORACLE_ARM_SUBNET_ID=          # 子网 ID
ORACLE_ARM_DISPLAY_NAME=       # 实例名称
ORACLE_ARM_IMAGE_ID=           # 系统镜像 ID
ORACLE_ARM_OCPUS=1             # CPU 核心数
ORACLE_ARM_MEMORY_GBS=6        # 内存大小(GB)
```

**可选设置 (OCI、默认参数、登录方式、重试逻辑、Telegram)：**
```sh
# OCI
OCI_CONFIG_FILE="$HOME/.oci/config"
OCI_CONFIG_PROFILE=DEFAULT
OCI_PRIVATE_KEY_PASSWORD=

# 实例默认参数
ORACLE_ARM_BOOT_VOLUME_GBS=50         # 引导卷大小
ORACLE_ARM_SHAPE=VM.Standard.A1.Flex  # 机器规格
ORACLE_ARM_ASSIGN_PUBLIC_IP=true      # 是否分配公网 IP
ORACLE_ARM_PV_ENCRYPTION=true

# 登录相关
ORACLE_ARM_ENABLE_ROOT_PASSWORD=true  # 是否启用 root 密码登录
ORACLE_ARM_ROOT_PASSWORD=             # 自定义 root 密码 (留空则随机生成)
ORACLE_ARM_SSH_AUTHORIZED_KEYS=       # SSH 公钥

# 重试与 IP 轮询等待时间
ORACLE_ARM_INITIAL_SLEEP_SECONDS=5    # 初始等待时间
ORACLE_ARM_MIN_SLEEP_SECONDS=5        # 最小重试等待时间
ORACLE_ARM_MAX_SLEEP_SECONDS=60       # 最大重试等待时间
ORACLE_ARM_PUBLIC_IP_WAIT_SECONDS=600 # 轮询获取公网 IP 的最大超时时间
ORACLE_ARM_PUBLIC_IP_INTERVAL_SECONDS=5

# Telegram 推送
TG_ENABLED=false
TG_BOT_TOKEN=
TG_CHAT_ID=
TG_API_HOST=api.telegram.org
```

### 🛠️ 从源码编译

*前置条件：请确保系统已安装 Go 语言环境。*

```sh
git clone https://github.com/Sheldonsix/oracle_arm_go.git
cd oracle_arm_go
go build -o oracle-arm .
```
交叉编译示例 (为 Linux ARM64 编译)：
```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o oracle-arm .
```

### ✨ 从原 Python 版本迁移的改进亮点
- 修复了 Telegram 禁用时程序仍会发送通知的问题。
- 缺少必要配置时，会返回清晰的错误提示，而不会直接崩溃 (panic)。
- OCI 配置文件路径和 Profile 现已支持通过环境变量自由配置。
- 优化了 VNIC (虚拟网卡) 的查找逻辑：现在使用实例所属区间 (Compartment) 查找，而不再硬编码为租户级别。
- 启用了 Cloud-init 的 root 密码设置后，SSH 密钥变为非必填项。
- 自动生成的 root 密码现在使用了高强度的密码学随机算法。
