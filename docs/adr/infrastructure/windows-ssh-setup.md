# Windows SSH Setup

Set up OpenSSH on the Windows machine so the Mac can run commands remotely (Ollama, Docker, etc.).

## 1. Install OpenSSH Server

Open PowerShell **as Administrator**:

```powershell
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
```

## 2. Start and Enable the Service

```powershell
Start-Service sshd
Set-Service -Name sshd -StartupType Automatic
```

## 3. Allow Through Firewall

```powershell
New-NetFirewallRule -Name sshd -DisplayName 'OpenSSH Server' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22
```

## 4. Find Your IP Address

```powershell
ipconfig
```

Look for the IPv4 address on your local network (e.g., `192.168.1.x`).

## 5. Test from Mac

```bash
ssh your-windows-username@192.168.1.x
```

If prompted for a password, use your Windows login password.

## 6. Set Up Key-Based Auth (No Password Prompts)

On the **Mac**:

```bash
# Generate a key if you don't have one
ssh-keygen -t ed25519

# Copy your public key to the Windows machine
ssh-copy-id your-windows-username@192.168.1.x
```

If `ssh-copy-id` doesn't work on Windows, do it manually:

```bash
# On Mac — print your public key
cat ~/.ssh/id_ed25519.pub
```

Then on the **Windows machine**, paste it into:
- For standard users: `C:\Users\YourUsername\.ssh\authorized_keys`
- For admin users: `C:\ProgramData\ssh\administrators_authorized_keys`

If using the admin file, fix permissions in PowerShell (admin):

```powershell
icacls "C:\ProgramData\ssh\administrators_authorized_keys" /inheritance:r /grant "SYSTEM:(R)" /grant "Administrators:(R)"
```

## 7. Verify Passwordless Login

From the **Mac**:

```bash
ssh your-windows-username@192.168.1.x "echo it works"
```

Should print "it works" with no password prompt.

## 8. Install Docker on Windows (if needed)

Download Docker Desktop from https://docs.docker.com/desktop/setup/install/windows-install/

Or via winget in PowerShell:

```powershell
winget install Docker.DockerDesktop
```

After install, restart and ensure Docker Desktop is running.

## 9. Install Ollama on Windows (if needed)

```powershell
winget install Ollama.Ollama
```

Then pull the required models:

```powershell
ollama pull mistral
ollama pull nomic-embed-text
```
