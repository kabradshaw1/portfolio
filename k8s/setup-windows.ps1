# setup-windows.ps1
# Run this on the Windows PC (PC@100.79.113.84) to set up the Minikube deployment.
# Prerequisites: Docker Desktop installed and running, Ollama installed.
# Run as Administrator (minikube tunnel requires elevated privileges).

$ErrorActionPreference = "Stop"

Write-Host "==> Step 1: Install Minikube (if not installed)" -ForegroundColor Cyan
if (-not (Get-Command minikube -ErrorAction SilentlyContinue)) {
    Write-Host "    Installing Minikube via winget..."
    winget install Kubernetes.minikube
    # Refresh PATH so minikube is available
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
} else {
    Write-Host "    Minikube already installed: $(minikube version --short)"
}

Write-Host ""
Write-Host "==> Step 2: Install kubectl (if not installed)" -ForegroundColor Cyan
if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
    Write-Host "    Installing kubectl via winget..."
    winget install Kubernetes.kubectl
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
} else {
    Write-Host "    kubectl already installed: $(kubectl version --client --short 2>$null)"
}

Write-Host ""
Write-Host "==> Step 3: Start Minikube" -ForegroundColor Cyan
$status = minikube status --format='{{.Host}}' 2>$null
if ($status -eq "Running") {
    Write-Host "    Minikube already running"
} else {
    Write-Host "    Starting Minikube with Docker driver..."
    # Allocate enough resources for all services
    minikube start --driver=docker --cpus=4 --memory=8192
}

Write-Host ""
Write-Host "==> Step 4: Enable NGINX Ingress Controller" -ForegroundColor Cyan
minikube addons enable ingress
# Wait for the ingress controller to be ready
Write-Host "    Waiting for ingress controller pod..."
kubectl wait --namespace ingress-nginx `
    --for=condition=ready pod `
    --selector=app.kubernetes.io/component=controller `
    --timeout=120s

Write-Host ""
Write-Host "==> Step 5: Deploy all services" -ForegroundColor Cyan
$repoPath = Read-Host "Enter the repo path on this machine (e.g., C:\Users\PC\repos\gen_ai_engineer)"
if (-not (Test-Path "$repoPath\k8s\deploy.sh")) {
    Write-Host "ERROR: k8s/deploy.sh not found at $repoPath" -ForegroundColor Red
    Write-Host "Make sure you've pulled the latest code: git pull origin feat/debug-assistant"
    exit 1
}
Push-Location $repoPath
bash k8s/deploy.sh
Pop-Location

Write-Host ""
Write-Host "==> Step 6: Start nvidia-gpu-exporter (Docker container)" -ForegroundColor Cyan
$gpuContainer = docker ps --filter "ancestor=utkuozdemir/nvidia_gpu_exporter:1.4.1" --format "{{.ID}}" 2>$null
if ($gpuContainer) {
    Write-Host "    nvidia-gpu-exporter already running"
} else {
    Write-Host "    Starting nvidia-gpu-exporter..."
    docker run -d --restart=unless-stopped --gpus all -p 9835:9835 utkuozdemir/nvidia_gpu_exporter:1.4.1
}

Write-Host ""
Write-Host "==> Step 7: Start minikube tunnel" -ForegroundColor Cyan
Write-Host ""
Write-Host "    IMPORTANT: minikube tunnel must run continuously." -ForegroundColor Yellow
Write-Host "    It requires Administrator privileges and will prompt for elevation."
Write-Host ""
Write-Host "    Option A (foreground - run in a separate terminal):"
Write-Host "      minikube tunnel"
Write-Host ""
Write-Host "    Option B (background task on startup):"
Write-Host "      See k8s/WINDOWS-SETUP.md for instructions on creating a scheduled task."
Write-Host ""

# Start tunnel now if user wants
$startTunnel = Read-Host "Start minikube tunnel now in this terminal? (y/n)"
if ($startTunnel -eq "y") {
    Write-Host "    Starting minikube tunnel (Ctrl+C to stop)..."
    minikube tunnel
}
