# Fix Windows SSH Admin Key Permissions

Run these commands on the Windows PC in **PowerShell as Administrator**:

```powershell
# Add your existing key to the admin authorized_keys file
Get-Content C:\Users\PC\.ssh\authorized_keys | Add-Content C:\ProgramData\ssh\administrators_authorized_keys

# Fix permissions — Windows OpenSSH requires exactly these
icacls "C:\ProgramData\ssh\administrators_authorized_keys" /inheritance:r /grant "Administradores:(F)" /grant "SYSTEM:(F)"
```

After running, test SSH from the Mac:
```bash
ssh -i ~/.ssh/github_actions_deploy PC@100.79.113.84 "echo connected"
```

Then delete this file — it's a one-time fix.
