# Fix Windows SSH Admin Key Permissions

## What happened

Claude wrote the GitHub Actions deploy key to `C:\ProgramData\ssh\administrators_authorized_keys`. On Windows OpenSSH, when that file exists for admin users, it **replaces** `~/.ssh/authorized_keys` as the auth source. The file only has the new deploy key — not your existing Mac SSH key. So SSH from your Mac is now rejected.

## How to fix

**Open PowerShell as Administrator** (not Git Bash — you need admin privileges for `C:\ProgramData\ssh\`).

Right-click Start button → "Windows Terminal (Admin)" or search "PowerShell" → "Run as administrator".

```powershell
# Check what's currently in the file (should be just the deploy key)
Get-Content C:\ProgramData\ssh\administrators_authorized_keys

# Add your original key back so both keys work
Get-Content C:\Users\PC\.ssh\authorized_keys | Add-Content C:\ProgramData\ssh\administrators_authorized_keys

# Fix permissions — Windows OpenSSH silently ignores the file if ACLs are wrong
icacls "C:\ProgramData\ssh\administrators_authorized_keys" /inheritance:r /grant "Administradores:(F)" /grant "SYSTEM:(F)"
```

## Verify from Mac

After running the fix, test both keys from the Mac:

```bash
# Test your regular key
ssh PC@100.79.113.84 "echo regular key works"

# Test the deploy key (used by GitHub Actions)
ssh -i ~/.ssh/github_actions_deploy PC@100.79.113.84 "echo deploy key works"
```

## Then

Once SSH works again, you can `git pull` on the Windows PC normally. Delete this file after — it's a one-time fix.
