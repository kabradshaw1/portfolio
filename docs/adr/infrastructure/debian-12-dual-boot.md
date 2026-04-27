# Debian 12 Dual Boot Install on Windows PC

Step-by-step guide for installing Debian 12 alongside Windows on the PC at 100.79.113.84 (RTX 3090). Assumes SSH access from the Mac and prior Debian install experience.

## Prerequisites

- USB flash drive (4GB+)
- Physical access to the Windows PC for BIOS/boot menu and installer
- Keyboard + monitor connected to the PC
- Back up anything important on the Windows side (Minikube PVCs, scripts, etc.)

## Phase 1: Prepare on Windows (Before Reboot)

### 1.1 Back Up Critical Files

On the Windows PC, back up anything not in git:

```
C:\Users\PC\Scripts\Restart-Services.ps1
C:\Users\PC\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1
```

Also note down:
- Minikube config and any custom K8s manifests not in the repo
- Cloudflared tunnel config (usually `C:\Users\PC\.cloudflared\config.yml`)
- Ollama model list (`ollama list`)
- Any environment variables or registry tweaks

### 1.2 Shrink the Windows Partition

1. Open **Disk Management** (right-click Start > Disk Management)
2. Right-click the main NTFS partition (usually `C:`) > **Shrink Volume**
3. Allocate at least **100 GB** for Debian (more if you plan to use it heavily)
4. Leave the freed space as **Unallocated** — the Debian installer will use it
5. Note which disk number and partition layout you see — you'll need this during install

### 1.3 Create the Bootable USB

Download the Debian 12 netinst ISO from https://www.debian.org/download

On the Windows PC, use **Rufus** to write the ISO to the USB drive:
1. Download Rufus from https://rufus.ie
2. Insert USB drive
3. Select the Debian ISO
4. Partition scheme: **GPT** (for UEFI — almost certainly what this PC uses)
5. File system: **FAT32**
6. Click Start, wait for completion

### 1.4 Check BIOS Settings

Reboot into BIOS (usually DEL or F2 at POST):
- **Secure Boot:** Note whether it's enabled or disabled. If enabled, you'll need to handle module signing for NVIDIA drivers later. Consider disabling it to simplify the NVIDIA setup.
- **Boot order:** Make sure USB boot is enabled
- Save and exit

## Phase 2: Install Debian 12

### 2.1 Boot the Installer

1. Insert the USB drive
2. Reboot and press **F12** (or your BIOS boot menu key) to select the USB
3. Choose **Graphical Install** or **Install** (text mode works fine too)

### 2.2 Walk Through the Installer

Most of this is straightforward — the key steps:

- **Language/locale/keyboard:** Your preference
- **Hostname:** Something like `pc-debian` (different from the Windows hostname to avoid confusion)
- **Domain:** Leave blank
- **Root password:** Set one, or leave blank to give your user sudo via the installer
- **User account:** Create your user (e.g., `kyle`)
- **Clock/timezone:** Match your location

### 2.3 Partition the Disk (Important Step)

Choose **Manual** partitioning for full control.

You'll see the existing Windows partitions plus the unallocated space you freed earlier. In the free space, create:

| Mount Point | Size | Type | Notes |
|---|---|---|---|
| `/` | 40-60 GB | ext4 | Root filesystem |
| `/home` | Remaining space minus swap | ext4 | User data |
| swap | 8-16 GB | swap | Match your RAM size or half of it |

Alternatively, for simplicity: just create a single `/` partition using all the free space, plus a swap partition. The installer will also offer a swap file option.

**Do NOT touch the existing Windows partitions (NTFS, EFI System Partition, Recovery).** If you see an existing EFI System Partition (~100-500 MB, FAT32), the Debian bootloader (GRUB) will install there alongside the Windows bootloader.

### 2.4 Software Selection

At the **tasksel** screen:
- Deselect **Debian desktop environment** and any DE (GNOME, KDE, etc.) — you'll be SSHing in, no desktop needed
- Select **SSH server**
- Select **standard system utilities**

This keeps the install minimal, which is what you want for a headless server you access over SSH.

### 2.5 Install GRUB

- Install GRUB to the EFI partition when prompted
- The installer should detect Windows and add it to the GRUB menu automatically

### 2.6 Reboot

Remove the USB drive when prompted. On reboot, you should see the GRUB menu with:
- Debian GNU/Linux
- Windows Boot Manager

Test both options to confirm dual boot works.

## Phase 3: Post-Install Debian Setup (Over SSH)

From here, work from your Mac over SSH.

### 3.1 Find the Debian IP and SSH In

If you're on Tailscale, the IP may differ from the Windows Tailscale IP since it's a different OS. On the PC (with keyboard/monitor still connected):

```bash
ip addr show
```

Note the LAN IP, then from your Mac:

```bash
ssh kyle@<debian-ip>
```

### 3.2 Set Up SSH Key Auth

From your Mac:

```bash
ssh-copy-id kyle@<debian-ip>
```

### 3.3 Update and Install Essentials

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget git build-essential software-properties-common
```

### 3.4 Install Node.js (for Claude Code later)

```bash
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs
```

### 3.5 Install Tailscale (to Keep the Same SSH Workflow)

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

Authenticate when prompted. After this, you'll have a stable Tailscale IP for the Debian install, and can SSH in the same way you do with Windows.

## Phase 4: NVIDIA Driver Setup

This is the part that can be tricky. Do this over SSH from your Mac.

### 4.1 Add Non-Free Repositories

Edit the apt sources:

```bash
sudo nano /etc/apt/sources.list
```

Make sure each `deb` line includes `non-free non-free-firmware`. For example:

```
deb http://deb.debian.org/debian bookworm main contrib non-free non-free-firmware
deb http://deb.debian.org/debian bookworm-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security bookworm-security main contrib non-free non-free-firmware
```

Then update:

```bash
sudo apt update
```

### 4.2 Install Kernel Headers

Required for building the NVIDIA kernel module:

```bash
sudo apt install -y linux-headers-$(uname -r)
```

### 4.3 Blacklist Nouveau

The open-source nouveau driver conflicts with the proprietary NVIDIA driver:

```bash
sudo bash -c 'cat > /etc/modprobe.d/blacklist-nouveau.conf << EOF
blacklist nouveau
options nouveau modeset=0
EOF'

sudo update-initramfs -u
```

### 4.4 Install the NVIDIA Driver

```bash
sudo apt install -y nvidia-driver firmware-misc-nonfree
```

This pulls in the appropriate driver version for Debian 12. For an RTX 3090, the packaged driver (535.x+) supports it fully.

Reboot after install:

```bash
sudo reboot
```

### 4.5 Verify the Driver

After reboot, SSH back in and check:

```bash
nvidia-smi
```

You should see the RTX 3090 listed with driver version and CUDA version. If this works, the hard part is done.

### 4.6 If Secure Boot Is Enabled

If you left Secure Boot on, the NVIDIA kernel module needs to be signed. During the `nvidia-driver` install, Debian will prompt you to set a MOK (Machine Owner Key) password. On the next reboot:

1. The MOK Manager appears before GRUB
2. Select **Enroll MOK**
3. Enter the password you set during install
4. Continue boot

If you missed this or it didn't work, easiest fix is to disable Secure Boot in BIOS and rebuild the module:

```bash
sudo dpkg-reconfigure nvidia-driver
sudo reboot
```

### 4.7 Install CUDA Toolkit (Optional — If You Need CUDA Beyond Ollama)

```bash
sudo apt install -y nvidia-cuda-toolkit
nvcc --version
```

Ollama bundles its own CUDA runtime, so this is only needed if you plan to run other CUDA workloads.

## Phase 5: Install Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

Verify it detects the GPU:

```bash
ollama serve &
ollama run qwen2.5:14b "Hello"
```

Check that the output of `ollama serve` mentions the RTX 3090 / CUDA.

Pull the models you need:

```bash
ollama pull qwen2.5:14b
ollama pull nomic-embed-text
```

### 5.1 Make Ollama Start on Boot

Ollama's install script typically creates a systemd service. Verify:

```bash
sudo systemctl enable ollama
sudo systemctl start ollama
sudo systemctl status ollama
```

### 5.2 Bind Ollama to All Interfaces

By default Ollama only listens on localhost. To reach it from other machines or K8s pods:

```bash
sudo systemctl edit ollama
```

Add:

```
[Service]
Environment="OLLAMA_HOST=0.0.0.0"
```

Then restart:

```bash
sudo systemctl restart ollama
```

## Phase 6: Switching Between OSes

### At Boot

GRUB shows the OS selection menu for ~5 seconds, then boots the default (Debian). To boot Windows, select it from the menu.

To change the default OS or timeout, edit `/etc/default/grub` on the Debian side:

```bash
sudo nano /etc/default/grub
```

- `GRUB_DEFAULT=0` — Debian is first (default), Windows is typically entry 2 or higher
- `GRUB_TIMEOUT=10` — seconds to wait before auto-booting default

After editing:

```bash
sudo update-grub
```

### Remote Switching (Over SSH)

You can reboot into Windows from Debian remotely:

```bash
# Find the Windows entry number
grep -i windows /boot/grub/grub.cfg

# Set Windows as next boot (one-time), then reboot
sudo grub-reboot 2  # adjust number based on grep output
sudo reboot
```

To get back to Debian from Windows, you'd need to reboot and either:
- Let GRUB timeout to the Debian default, or
- Have someone select Debian from the GRUB menu, or
- Set the BIOS to always show the boot menu

### Network Considerations

Each OS will have its own Tailscale IP. Your Mac SSH config can have entries for both:

```
# ~/.ssh/config on your Mac
Host pc-windows
    HostName 100.79.113.84
    User PC

Host pc-debian
    HostName <debian-tailscale-ip>
    User kyle
```

## Troubleshooting

### GRUB Doesn't Show Windows

```bash
sudo os-prober
sudo update-grub
```

If `os-prober` is disabled by default (Debian 12 does this), enable it:

```bash
echo 'GRUB_DISABLE_OS_PROBER=false' | sudo tee -a /etc/default/grub
sudo update-grub
```

### nvidia-smi Shows "No devices found"

1. Confirm the GPU is visible: `lspci | grep -i nvidia`
2. Check if nouveau is still loaded: `lsmod | grep nouveau` (should be empty)
3. Check if nvidia module is loaded: `lsmod | grep nvidia`
4. If not loaded: `sudo modprobe nvidia` and check `dmesg | tail` for errors
5. Secure Boot issue? See section 4.6

### SSH Connection Refused After Debian Install

- Verify `sshd` is running: `sudo systemctl status ssh`
- Check firewall: `sudo iptables -L` (Debian 12 has no firewall rules by default)
- Verify the IP: `ip addr show`

### Lost Windows After Debian Install

This shouldn't happen if you didn't touch the Windows partitions. But if GRUB doesn't show Windows:

```bash
sudo apt install os-prober
sudo os-prober
sudo update-grub
```
