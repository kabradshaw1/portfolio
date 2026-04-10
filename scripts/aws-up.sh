#!/usr/bin/env bash
set -euo pipefail

# Spin up the AWS environment for a demo.
# Prerequisites: terraform CLI, AWS credentials configured, kubectl installed.
# Usage: ./scripts/aws-up.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TF_DIR="$REPO_DIR/terraform"
BOOTSTRAP_DIR="$TF_DIR/bootstrap"

echo "==> Step 1: Bootstrap (S3 state bucket + DynamoDB lock)"
cd "$BOOTSTRAP_DIR"
terraform init
terraform apply -auto-approve
echo ""

echo "==> Step 2: Provision infrastructure"
cd "$TF_DIR"
terraform init
terraform apply -auto-approve
echo ""

echo "==> Step 3: Configure kubectl"
CLUSTER_NAME=$(terraform output -raw cluster_name)
REGION=$(terraform output -raw region 2>/dev/null || echo "us-east-1")
aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$REGION"
echo ""

echo "==> Step 4: Deploy services"
cd "$REPO_DIR"
./k8s/deploy.sh aws
echo ""

echo "==> Step 5: Get ALB hostname"
ALB=$(kubectl get ingress -n ai-services \
  -o jsonpath='{.items[0].status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "")

echo ""
echo "============================================"
echo "  AWS environment is up!"
echo "============================================"
echo ""
if [ -n "$ALB" ]; then
  echo "  ALB hostname: $ALB"
  echo ""
  echo "  Next steps:"
  echo "    1. In Cloudflare, set api.kylebradshaw.dev CNAME -> $ALB"
  echo "    2. Pause the Cloudflare Tunnel to the Windows PC"
  echo "    3. Wait ~2 min for DNS propagation"
  echo "    4. Test: curl https://api.kylebradshaw.dev/ingestion/health"
else
  echo "  ALB not yet provisioned. Check ingress status:"
  echo "    kubectl get ingress --all-namespaces"
fi
echo ""
echo "  Estimated cost: ~\$5-9/day while running"
echo "  Tear down: ./scripts/aws-down.sh"
echo ""
